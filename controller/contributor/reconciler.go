package contributor

import (
	"context"
	"crypto/md5"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/pkg/errors"
	"istio.io/api/security/v1beta1"
	v1beta12 "istio.io/api/type/v1beta1"
	istiosecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/johnhoman/kubeflow-profile-manager/apis/v1alpha1"
	"github.com/johnhoman/kubeflow-profile-manager/controller/features"
	"github.com/johnhoman/kubeflow-profile-manager/controller/manager"
)

const (
	errReconcileServiceAccount      = "failed to reconcile service account"
	errReconcileRoleBinding         = "failed to reconcile role binding"
	errReconcileAuthorizationPolicy = "failed to reconcile authorization policy"

	errFmtSetControllerRef = "failed to set controller reference on %s"
)

func Setup(mgr ctrl.Manager, o controller.Options, opts ...ReconcilerOption) error {

	name := "kubeflow.org/contributor-manager"

	opts = append(opts,
		WithDefaultServiceAccountReconcilerFunc(),
		WithDefaultRoleBindingReconcilerFunc(),
		WithLogger(o.Logger.WithValues("controller", name)),
	)
	builder := ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		// +kubebuilder:rbac:groups=kubeflow.org,resources=contributors,verbs=create;update;delete;patch;get;list;watch
		For(&v1alpha1.Contributor{}).
		// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=create;update;delete;patch;get;list;watch
		Owns(&corev1.ServiceAccount{}).
		// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs="*"
		Owns(&rbacv1.RoleBinding{})

	if o.Features.Enabled(features.Istio) {
		// +kubebuilder:rbac:groups=security.istio.io,resources=authorizationpolicies,verbs=create;update;delete;get;list;patch;watch
		builder.Owns(&istiosecurity.AuthorizationPolicy{})
		opts = append(opts, WithIstioEnabled())
	}

	if o.Features.Enabled(features.Pipelines) {
		opts = append(opts, WithPipelinesEnabled())
	}

	return builder.Complete(NewReconciler(mgr, opts...))
}

type ReconcilerOption func(r *Reconciler)

func WithUserIDPrefix(prefix string) ReconcilerOption {
	return func(r *Reconciler) {
		r.userIDPrefix = prefix
	}
}

func WithUserIDHeader(header string) ReconcilerOption {
	return func(r *Reconciler) {
		r.userIDHeader = header
	}
}

func WithIstioEnabled() ReconcilerOption {
	return func(r *Reconciler) {
		r.istio = r.ReconcileIstioAuthorizationPolicy
	}
}

func WithPipelinesEnabled() ReconcilerOption {
	return func(r *Reconciler) {}
}

func WithContributorClusterRole(name string) ReconcilerOption {
	return func(r *Reconciler) {
		r.contributorRole = corev1.LocalObjectReference{Name: name}
	}
}

func WithDefaultServiceAccountReconcilerFunc() ReconcilerOption {
	return func(r *Reconciler) {
		r.serviceAccount = r.ReconcileServiceAccount
	}
}

func WithDefaultRoleBindingReconcilerFunc() ReconcilerOption {
	return func(r *Reconciler) {
		r.roleBinding = r.ReconcileRoleBinding
	}
}

func WithLogger(logger logging.Logger) ReconcilerOption {
	return func(r *Reconciler) {
		r.logger = logger
	}
}

type ReconcileFunc func(ctx context.Context, contributor *v1alpha1.Contributor) (controllerutil.OperationResult, error)

func NopReconcileFunc(context.Context, *v1alpha1.Contributor) (controllerutil.OperationResult, error) {
	return controllerutil.OperationResultNone, nil
}

func NewReconciler(mgr manager.Manager, opts ...ReconcilerOption) *Reconciler {
	r := &Reconciler{
		client:       mgr.GetClient(),
		logger:       logging.NewNopLogger(),
		userIDHeader: "kubeflow-userid",

		contributorRole: corev1.LocalObjectReference{Name: "kubeflow-edit"},
		// reconcile features
		istio:          NopReconcileFunc,
		roleBinding:    NopReconcileFunc,
		serviceAccount: NopReconcileFunc,
	}
	for _, f := range opts {
		f(r)
	}
	return r
}

type Reconciler struct {
	client client.Client
	logger logging.Logger

	contributorRole corev1.LocalObjectReference

	// user id
	userIDPrefix string
	userIDHeader string

	// Features
	istio          ReconcileFunc
	roleBinding    ReconcileFunc
	serviceAccount ReconcileFunc
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	contributor := &v1alpha1.Contributor{}
	if err := r.client.Get(ctx, req.NamespacedName, contributor); err != nil {
		return ctrl.Result{}, errors.Wrap(client.IgnoreNotFound(err), "failed to read profile")
	}

	funcs := []ReconcileFunc{
		r.serviceAccount,
		r.roleBinding,
		r.istio,
	}

	for _, f := range funcs {
		res, err := f(ctx, contributor)
		if err != nil {
			return ctrl.Result{}, err
		}
		switch res {
		case controllerutil.OperationResultCreated:
		case controllerutil.OperationResultUpdated:
		}
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileServiceAccount(ctx context.Context, contributor *v1alpha1.Contributor) (controllerutil.OperationResult, error) {

	serviceAccount := &corev1.ServiceAccount{}
	serviceAccount.SetName(contributor.Name)
	serviceAccount.SetNamespace(contributor.Namespace)

	res, err := controllerutil.CreateOrPatch(ctx, r.client, serviceAccount, func() error {
		if err := controllerutil.SetControllerReference(contributor, serviceAccount, r.client.Scheme()); err != nil {
			return errors.Wrapf(err, errFmtSetControllerRef, "ServiceAccount")
		}
		addLabel(serviceAccount, "owner.kubeflow.org/id", md5Sum(contributor.Spec.Name))
		addAnnotation(serviceAccount, "owner.kubeflow.org/name", contributor.Spec.Name)
		return nil
	})
	return res, errors.Wrap(err, errReconcileServiceAccount)
}

func (r *Reconciler) ReconcileRoleBinding(ctx context.Context, contributor *v1alpha1.Contributor) (controllerutil.OperationResult, error) {
	// TODO: maybe move this out of here and just have one reconciler that watches all
	//       contributors and rewrites the subjects based on that

	binding := &rbacv1.RoleBinding{}
	binding.SetName(contributor.Name)
	binding.SetNamespace(contributor.Namespace)

	res, err := controllerutil.CreateOrPatch(ctx, r.client, binding, func() error {
		if err := controllerutil.SetControllerReference(contributor, binding, r.client.Scheme()); err != nil {
			return errors.Wrapf(err, errFmtSetControllerRef, "RoleBinding")
		}
		addLabel(binding, "owner.kubeflow.org/id", md5Sum(contributor.Spec.Name))
		addAnnotation(binding, "owner.kubeflow.org/name", contributor.Spec.Name)
		// TODO(johnhoman): These might not be necessary but are used by kfam
		//   to list contributors in a namespace
		addAnnotation(binding, "owner", contributor.Spec.Name)
		addAnnotation(binding, "role", contributor.Spec.Role)
		binding.RoleRef = rbacv1.RoleRef{
			Kind:     "ClusterRole",
			APIGroup: rbacv1.GroupName,
			Name:     r.contributorRole.Name,
		}
		binding.Subjects = []rbacv1.Subject{{
			Kind: rbacv1.UserKind,
			Name: contributor.Spec.Name,
		}}
		return nil
	})
	return res, errors.Wrap(err, errReconcileRoleBinding)
}

func (r *Reconciler) ReconcileIstioAuthorizationPolicy(ctx context.Context, contributor *v1alpha1.Contributor) (controllerutil.OperationResult, error) {

	// TODO: AuthorizationPolicy for all public Notebooks e.g.
	//   selector:
	//     matchLabels:
	//       kubeflow.org/visibility: private
	public := &istiosecurity.AuthorizationPolicy{}
	public.Name = fmt.Sprintf("%s-public", contributor.Name)
	public.Namespace = contributor.Namespace
	res, err := controllerutil.CreateOrPatch(ctx, r.client, public, func() error {
		if err := controllerutil.SetControllerReference(contributor, public, r.client.Scheme()); err != nil {
			return errors.Wrapf(err, errFmtSetControllerRef, "AuthorizationPolicy")
		}
		public.Spec = v1beta1.AuthorizationPolicy{
			Action: v1beta1.AuthorizationPolicy_ALLOW,
			Rules: []*v1beta1.Rule{{
				When: []*v1beta1.Condition{{
					Key:    fmt.Sprintf("request.headers[%v]", r.userIDHeader),
					Values: []string{r.userIDPrefix + contributor.Spec.Name},
				}},
				From: []*v1beta1.Rule_From{{
					Source: &v1beta1.Source{
						Principals: []string{principalIstioIngressGateway},
					},
				}},
			}},
			Selector: &v1beta12.WorkloadSelector{
				MatchLabels: map[string]string{
					"kubeflow.org/visibility": "public",
				},
			},
		}
		return nil
	})
	if err != nil {
		r.logger.Debug("failed to reconcile contributor public istio AuthorizationPolicy",
			"error", err.Error())
		return res, err
	}

	policy := &istiosecurity.AuthorizationPolicy{}
	policy.Name = fmt.Sprintf("%s-private", contributor.Name)
	policy.Namespace = contributor.Namespace

	res, err = controllerutil.CreateOrPatch(ctx, r.client, policy, func() error {
		if err := controllerutil.SetControllerReference(contributor, policy, r.client.Scheme()); err != nil {
			return errors.Wrapf(err, errFmtSetControllerRef, "AuthorizationPolicy")
		}
		policy.Spec = v1beta1.AuthorizationPolicy{
			Action: v1beta1.AuthorizationPolicy_ALLOW,
			Rules: []*v1beta1.Rule{{
				When: []*v1beta1.Condition{{
					// Namespace Owner can access all workloads in the
					// namespace
					Key:    fmt.Sprintf("request.headers[%v]", r.userIDHeader),
					Values: []string{r.userIDPrefix + contributor.Spec.Name},
				}},
				From: []*v1beta1.Rule_From{{
					Source: &v1beta1.Source{
						Principals: []string{
							principalIstioIngressGateway,
							fmt.Sprintf("cluster.local/ns/%s/sa/%s", contributor.Namespace, contributor.Name),
						},
					},
				}},
			}},
			Selector: &v1beta12.WorkloadSelector{
				MatchLabels: map[string]string{
					"owner.kubeflow.org/id": md5Sum(contributor.Spec.Name),
				},
			},
		}
		return nil
	})
	r.logger.Debug("finished reconciling authorization policy")
	return res, errors.Wrap(err, errReconcileAuthorizationPolicy)
}

var _ reconcile.Reconciler = &Reconciler{}

func md5Sum(name string) string { return fmt.Sprintf("%x", md5.Sum([]byte(name))) }

func addLabel(o client.Object, key, value string) {
	labels := o.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = value
	o.SetLabels(labels)
}

func addAnnotation(o client.Object, key, value string) {
	annotations := o.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[key] = value
	o.SetAnnotations(annotations)
}

const (
	principalIstioIngressGateway = "cluster.local/ns/istio-system/sa/istio-ingressgateway-service-account"
)
