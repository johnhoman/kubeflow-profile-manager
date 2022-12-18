package access

import (
	"crypto/md5"
	"encoding/base32"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/johnhoman/kubeflow-profile-manager/apis/v1alpha1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ManagerOption func(m *manager)

func WithUserIDHeader(header string) ManagerOption {
	return func(m *manager) {
		m.header = header
	}
}

func WithUserIDPrefix(prefix string) ManagerOption {
	return func(m *manager) {
		m.prefix = prefix
	}
}

func WithAdmin(admins ...string) ManagerOption {
	return func(m *manager) {
		if m.admins == nil {
			m.admins = sets.NewString()
		}
		m.admins.Insert(admins...)
	}
}

func NewManager(cli client.Client, opts ...ManagerOption) *manager {
	m := &manager{
		client: cli,
		admins: sets.NewString(),
		header: "kubeflow-userid",
	}
	for _, f := range opts {
		f(m)
	}
	return m
}

type manager struct {
	// client is a Kubernetes client
	client client.Client
	// header is the user id header name from the request that identifies the user
	header string
	// prefix is the user id header name prefix from the request that identifies the user
	prefix string
	// admins are cluster admins
	admins sets.String
}

// CreateProfile creates a new profile for a user
func (m *manager) CreateProfile(c *gin.Context) {

	p := &v1alpha1.Profile{}
	if err := c.ShouldBindJSON(p); err != nil {
		_ = c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if err := m.client.Create(c, p); err != nil {
		code := http.StatusInternalServerError
		if apierrors.IsAlreadyExists(err) {
			code = http.StatusConflict
		}
		_ = c.AbortWithError(code, err)
		return
	}
	c.Writer.WriteHeader(http.StatusOK)
}

func (m *manager) RemoveProfile(c *gin.Context) {

	email := strings.TrimPrefix(c.GetHeader(m.header), m.prefix)
	name := c.Param("profile")

	p := &v1alpha1.Profile{}
	if err := m.client.Get(c, client.ObjectKey{Name: name}, p); err != nil {
		if apierrors.IsNotFound(err) {
			_ = c.AbortWithError(http.StatusNotFound, err)
			return
		}
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	contributorList := &v1alpha1.ContributorList{}
	if err := m.client.List(c, contributorList, client.InNamespace(p.Name)); err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	admins := m.admins.Clone()
	admins.Insert(p.Spec.Owner.Name)
	for _, item := range contributorList.Items {
		if item.Spec.Role == v1alpha1.ContributorRoleOwner {
			admins.Insert(item.Name)
		}
	}

	if !admins.Has(email) {
		c.Writer.WriteHeader(http.StatusForbidden)
		return
	}

	if err := m.client.Delete(c, p); client.IgnoreNotFound(err) != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.Writer.WriteHeader(http.StatusOK)
	return
}

// AddContributor adds a contributor to a user profile
func (m *manager) AddContributor(c *gin.Context) {
	// TODO: auth

	binding := &Binding{}
	if err := c.ShouldBindJSON(binding); err != nil {
		_ = c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if binding.User.Kind != rbacv1.UserKind {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"message": "only users can be added as contributors",
		})
		return
	}

	contributor := &v1alpha1.Contributor{}
	contributor.Name = strings.Split(binding.User.Name, "@")[0]
	contributor.Labels = map[string]string{
		"owner.kubeflow.org/id":         md5Sum(binding.User.Name),
		"contributor.kubeflow.org/role": binding.RoleRef.Name,
	}
	contributor.Namespace = binding.ReferredNamespace
	contributor.Spec = v1alpha1.ContributorSpec{
		Name: binding.User.Name,
		Role: v1alpha1.ContributorRoleContributor,
	}
	if err := m.client.Create(c, contributor); err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Added Contributor"})
}

func (m *manager) ReadNamespaces(c *gin.Context) {
	namespace := c.Query("namespace")
	user := c.Query("user")
	role := c.Query("role")

	opts := make([]client.ListOption, 0)
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}
	if user != "" || role != "" {
		selector := client.MatchingLabels{}
		if user != "" {
			selector["owner.kubeflow.org/id"] = md5Sum(user)
		}
		if role != "" {
			selector["contributor.kubeflow.org/role"] = role
		}
		opts = append(opts, selector)
	}

	contributorList := &v1alpha1.ContributorList{}
	if err := m.client.List(c, contributorList, opts...); err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	bindings := make([]Binding, 0)
	for _, contributor := range contributorList.Items {
		roleRefName := ""
		switch contributor.Spec.Role {
		case v1alpha1.ContributorRoleOwner:
			roleRefName = "admin"
		case v1alpha1.ContributorRoleContributor:
			roleRefName = "edit"
		default:
			continue
		}
		binding := Binding{
			ReferredNamespace: contributor.Namespace,
			RoleRef: &rbacv1.RoleRef{
				Name: roleRefName,
				Kind: "ClusterRole",
			},
			User: &rbacv1.Subject{
				Kind: rbacv1.UserKind,
				Name: contributor.Spec.Name,
			},
		}
		bindings = append(bindings, binding)
	}

	c.JSON(http.StatusOK, gin.H{"bindings": bindings})
}

func (m *manager) RemoveContributor(c *gin.Context) {

	email := strings.TrimPrefix(c.GetHeader(m.header), m.prefix)

	binding := &Binding{}
	if err := c.ShouldBindJSON(binding); err != nil {
		_ = c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	profile := &v1alpha1.Profile{}
	if err := m.client.Get(c, client.ObjectKey{Name: binding.ReferredNamespace}, profile); err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	contributorList := &v1alpha1.ContributorList{}
	if err := m.client.List(c, contributorList, client.InNamespace(binding.ReferredNamespace)); err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	admins := m.admins.Clone()
	if profile.Spec.Owner.Kind == rbacv1.UserKind {
		admins.Insert(profile.Spec.Owner.Name)
	}

	for _, item := range contributorList.Items {
		if item.Spec.Role == v1alpha1.ContributorRoleOwner {
			admins.Insert(item.Spec.Name)
		}
	}

	if !admins.Has(email) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	if err := m.client.DeleteAllOf(c,
		&v1alpha1.Contributor{},
		client.MatchingLabels{"owner.kubeflow.org/id": md5Sum(binding.User.Name)},
		client.InNamespace(binding.ReferredNamespace),
	); err != nil {
		_ = c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Removed Contributor"})
	return
}

func (m *manager) ListAdmins(c *gin.Context) {
	user := c.Query("user")
	if user == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "missing required param 'user'",
		})
		return
	}
	c.String(http.StatusOK, strconv.FormatBool(m.admins.Has(user)))
}

var _ Manager = &manager{}

func b32Encode(name string) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(name))
}

func md5Sum(name string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(name)))
}
