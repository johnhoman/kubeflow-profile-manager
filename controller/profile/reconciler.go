package profile

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/http"

	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/feature"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/pkg/errors"
	"istio.io/api/security/v1beta1"
	istiosecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/johnhoman/kubeflow-profile-manager/apis/v1alpha1"
	"github.com/johnhoman/kubeflow-profile-manager/controller/features"
	"github.com/johnhoman/kubeflow-profile-manager/controller/manager"
)

const (
	errReconcileNamespace           = "failed to reconcile namespace"
	errReconcileAuthorizationPolicy = "failed to reconcile Istio AuthorizationPolicy"
	errReconcileResourceQuota       = "failed to reconcile resource quota"
	errReconcileOwnerContributor    = "failed to reconcile owner contributor"

	errFmtSetControllerRef = "failed to set controller reference on %s"

	// Stop result is returned from a reconciler when profile reconciliation should stop
	// and finish gracefully (e.g. without error or requeue)
	Stop = controllerutil.OperationResult("Stop")
)

// +kubebuilder:rbac:groups=kubeflow.org,resources=profiles,verbs=create;update;delete;patch;get;list;watch
// +kubebuilder:rbac:groups=kubeflow.org,resources=profiles/status,verbs=patch
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=create;update;delete;patch;get;list;watch
// +kubebuilder:rbac:groups=core,resources=resourcequotas,verbs=get;list;watch;patch;create;update;delete
// +kubebuilder:rbac:groups=kubeflow.org,resources=contributors,verbs=get;list;watch

func Setup(mgr ctrl.Manager, o controller.Options, opts ...ReconcilerOption) error {

	name := "kubeflow.org/profile-manager"

	opts = append(opts,
		WithLogger(o.Logger.WithValues("controller", "profile-manager")),
		WithDefaultNamespaceReconcileFunc(),
		WithDefaultContributorReconcilerFunc(),
		WithNamespaceAdoptionDisabled(),
		WithResourceQuotaEnabled(),
	)

	builder := ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&v1alpha1.Profile{}).
		Owns(&corev1.Namespace{}).
		Owns(&corev1.ResourceQuota{}).
		Watches(
			&source.Kind{Type: &v1alpha1.Contributor{}},
			handler.EnqueueRequestsFromMapFunc(func(o client.Object) []ctrl.Request {
				return []ctrl.Request{{
					NamespacedName: client.ObjectKey{Name: o.GetNamespace()},
				}}
			}),
		)

	if o.Features.Enabled(features.Istio) {
		opts = append(opts, WithIstioEnabled(), WithNamespaceLabel("istio-injection", "true"))
		// +kubebuilder:rbac:groups=security.istio.io,resources=authorizationpolicies,verbs=create;update;delete;patch;get;list;watch
		builder.Owns(&istiosecurity.AuthorizationPolicy{})
	}

	if o.Features.Enabled(features.NamespaceAdoption) {
		opts = append(opts, WithNamespaceAdoptionEnabled())
	}

	if o.Features.Enabled(features.Pipelines) {
		opts = append(opts, WithPipelinesEnabled())
	}

	return builder.Complete(NewReconciler(mgr, opts...))
}

type ReconcilerOption func(r *Reconciler)

func WithNamespaceAdoptionEnabled() ReconcilerOption {
	return func(r *Reconciler) {
		r.namespaceAdoptionEnabled = true
	}
}

func WithNamespaceAdoptionDisabled() ReconcilerOption {
	return func(r *Reconciler) {
		r.namespaceAdoptionEnabled = false
	}
}

func WithIstioEnabled() ReconcilerOption {
	return func(r *Reconciler) {
		r.istio = r.ReconcileIstioAuthorizationPolicy
	}
}

func WithDefaultNamespaceReconcileFunc() ReconcilerOption {
	return func(r *Reconciler) {
		r.namespace = r.ReconcileNamespace
	}
}

func WithNamespaceLabels(labels map[string]string) ReconcilerOption {
	return func(r *Reconciler) {
		if r.namespaceLabels == nil {
			r.namespaceLabels = make(map[string]string)
		}
		for key, value := range labels {
			r.namespaceLabels[key] = value
		}
	}
}

func WithNamespaceLabel(key, value string) ReconcilerOption {
	return WithNamespaceLabels(map[string]string{key: value})
}

func WithDefaultResourceQuotaSpec(spec corev1.ResourceQuotaSpec) ReconcilerOption {
	return func(r *Reconciler) {
		r.defaultResourceQuotaSpec = &spec
	}
}

func WithResourceQuotaEnabled() ReconcilerOption {
	return func(r *Reconciler) {
		r.resourceQuota = r.ReconcileResourceQuota
	}
}

func WithPipelinesEnabled() ReconcilerOption {
	return WithNamespaceLabel("pipelines.kubeflow.org/enabled", "true")
}

func WithDefaultContributorReconcilerFunc() ReconcilerOption {
	return func(r *Reconciler) {
		r.contributor = r.ReconcileContributor
	}
}

func WithLogger(logger logging.Logger) ReconcilerOption {
	return func(r *Reconciler) {
		r.logger = logger
	}
}

type ReconcileFunc func(ctx context.Context, profile *v1alpha1.Profile) (controllerutil.OperationResult, error)

func NopReconcileFunc(context.Context, *v1alpha1.Profile) (controllerutil.OperationResult, error) {
	return controllerutil.OperationResultNone, nil
}

// NewReconciler returns a new profile reconciler with all resource reconciliation
// set to no op. Use the package ReconcilerOption functions to change the default behaviour
// Call NewReconciler minimally with NewReconciler(mgr, WithDefaultNamespaceReconcileFunc()).
func NewReconciler(mgr manager.Manager, opts ...ReconcilerOption) *Reconciler {
	r := &Reconciler{
		client: mgr.GetClient(),
		logger: logging.NewNopLogger(),

		// reconcile features
		namespace:     NopReconcileFunc,
		istio:         NopReconcileFunc,
		resourceQuota: NopReconcileFunc,
		contributor:   NopReconcileFunc,
	}
	for _, f := range opts {
		f(r)
	}
	return r
}

type Reconciler struct {
	client client.Client
	logger logging.Logger

	features *feature.Flags

	namespaceAdoptionEnabled bool
	namespaceLabels          map[string]string

	defaultResourceQuotaSpec *corev1.ResourceQuotaSpec

	// Features
	namespace     ReconcileFunc
	resourceQuota ReconcileFunc
	istio         ReconcileFunc
	contributor   ReconcileFunc
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	profile := &v1alpha1.Profile{}
	if err := r.client.Get(ctx, req.NamespacedName, profile); err != nil {
		return ctrl.Result{}, errors.Wrap(client.IgnoreNotFound(err), "failed to read profile")
	}

	contributorList := &v1alpha1.ContributorList{}
	if err := r.client.List(ctx, contributorList, client.InNamespace(profile.Name)); err != nil {
		return ctrl.Result{}, err
	}
	contributors := sets.NewString()
	for _, item := range contributorList.Items {
		contributors.Insert(item.Name)
	}
	patch := client.MergeFrom(profile.DeepCopy())
	profile.Status.Contributors = make([]corev1.LocalObjectReference, contributors.Len())
	for k, name := range contributors.List() {
		profile.Status.Contributors[k].Name = name
	}
	if err := r.client.Status().Patch(ctx, profile, patch); err != nil {
		return ctrl.Result{}, err
	}

	funcs := []ReconcileFunc{
		r.namespace,
		r.contributor,
		r.resourceQuota,
		r.istio,
	}

	for _, f := range funcs {
		res, err := f(ctx, profile)
		if err != nil {
			return ctrl.Result{}, err
		}
		switch res {
		case controllerutil.OperationResultCreated:
		case controllerutil.OperationResultUpdated:
		case Stop:
			r.logger.Debug("stop signal received from reconcile func")
			return ctrl.Result{}, nil
		}
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNamespace(ctx context.Context, profile *v1alpha1.Profile) (controllerutil.OperationResult, error) {

	namespace := &corev1.Namespace{}
	namespace.Name = profile.Name

	updateFn := func() error {
		if err := controllerutil.SetControllerReference(profile, namespace, r.client.Scheme()); err != nil {
			return errors.Wrapf(err, errFmtSetControllerRef, "namespace")
		}
		for key, value := range r.namespaceLabels {
			if !metav1.HasLabel(namespace.ObjectMeta, key) {
				addLabel(namespace, key, value)
			}
		}
		addAnnotation(namespace, "owner", profile.Spec.Owner.Name)
		return nil
	}

	if err := r.client.Get(ctx, client.ObjectKeyFromObject(profile), namespace); err != nil {
		if apierrors.IsNotFound(err) {
			res, err := controllerutil.CreateOrPatch(ctx, r.client, namespace, updateFn)
			return res, errors.Wrap(err, errReconcileNamespace)
		}
		return controllerutil.OperationResultNone, errors.Wrap(err, errReconcileNamespace)
	}

	annotations := namespace.Annotations
	if !r.namespaceAdoptionEnabled {
		if owner, ok := annotations["owner"]; !ok || owner != profile.Spec.Owner.Name {
			r.logger.Debug("refusing to update namespace not owned by profile")
			return Stop, nil
		}
	}
	res, err := controllerutil.CreateOrPatch(ctx, r.client, namespace, updateFn)
	return res, errors.Wrap(err, errReconcileNamespace)
}

func (r *Reconciler) ReconcileIstioAuthorizationPolicy(ctx context.Context, profile *v1alpha1.Profile) (controllerutil.OperationResult, error) {
	policy := &istiosecurity.AuthorizationPolicy{}
	policy.Name = "control-plane-access"
	policy.Namespace = profile.Name
	res, err := controllerutil.CreateOrPatch(ctx, r.client, policy, func() error {
		if err := controllerutil.SetControllerReference(profile, policy, r.client.Scheme()); err != nil {
			return errors.Wrapf(err, errFmtSetControllerRef, "AuthorizationPolicy")
		}
		addLabel(policy, "app.kubernetes.io/part-of", "kubeflow-profile")
		policy.Spec = v1beta1.AuthorizationPolicy{
			Action: v1beta1.AuthorizationPolicy_ALLOW,
			Rules: []*v1beta1.Rule{{
				To: []*v1beta1.Rule_To{{
					Operation: &v1beta1.Operation{
						// Workloads pathes should be accessible for KNative's
						// `activator` and `controller` probes
						// See: https://knative.dev/docs/serving/istio-authorization/#allowing-access-from-system-pods-by-paths
						Paths: []string{"/healthz", "/metrics", "/wait-for-drain"},
					},
				}},
			}, {
				// allow the notebook-controller in the kubeflow namespace to
				// access the api/kernels endpoint of the notebook servers.
				From: []*v1beta1.Rule_From{{
					Source: &v1beta1.Source{
						Principals: []string{principalNotebookController},
					},
				}},
				To: []*v1beta1.Rule_To{{
					Operation: &v1beta1.Operation{
						Methods: []string{http.MethodGet},
						Paths:   []string{"*/api/kernels"},
					},
				}},
			}},
		}
		return nil
	})
	r.logger.Debug("finished reconciling authorization policy")
	return res, errors.Wrap(err, errReconcileAuthorizationPolicy)
}

// ReconcileResourceQuota creates a resource quota in the namespace currently being reconciled. If
// a profile specifies a resource quota spec, that will be the default spec. If the profile quota
// is not specified but a default exists, the default resource quota spec will be used
func (r *Reconciler) ReconcileResourceQuota(ctx context.Context, profile *v1alpha1.Profile) (controllerutil.OperationResult, error) {

	quota := &corev1.ResourceQuota{}
	quota.Name = "kf-resource-quota"
	quota.Namespace = profile.Name

	res, err := controllerutil.CreateOrUpdate(ctx, r.client, quota, func() error {
		if err := controllerutil.SetControllerReference(profile, quota, r.client.Scheme()); err != nil {
			return errors.Wrapf(err, errFmtSetControllerRef, "ResourceQuota")
		}
		addLabel(quota, "app.kubernetes.io/part-of", "kubeflow-profile")
		spec := corev1.ResourceQuotaSpec{}
		switch {
		case profile.Spec.ResourceQuotaSpec != nil:
			spec = *profile.Spec.ResourceQuotaSpec
		case r.defaultResourceQuotaSpec != nil:
			spec = *r.defaultResourceQuotaSpec
		}
		quota.Spec = spec
		return nil
	})
	return res, errors.Wrap(err, errReconcileResourceQuota)
}

func (r *Reconciler) ReconcileContributor(ctx context.Context, profile *v1alpha1.Profile) (controllerutil.OperationResult, error) {

	r.logger.Debug("reconciling owner")
	if profile.Spec.Owner.Kind != rbacv1.UserKind {
		r.logger.Debug("skipping owner contributor because profile owner is not kind User")
		return NopReconcileFunc(ctx, profile)
	}

	contrib := &v1alpha1.Contributor{}
	contrib.Name = profile.Name
	contrib.Namespace = profile.Name
	res, err := controllerutil.CreateOrPatch(ctx, r.client, contrib, func() error {
		if err := controllerutil.SetControllerReference(profile, contrib, r.client.Scheme()); err != nil {
			return errors.Wrapf(err, errFmtSetControllerRef, "Contributor")
		}
		addLabel(contrib, "owner.kubeflow.org/id", md5Sum(profile.Spec.Owner.Name))
		addLabel(contrib, "contributor.kubeflow.org/role", "admin")
		contrib.Spec.Name = profile.Spec.Owner.Name
		contrib.Spec.Role = v1alpha1.ContributorRoleOwner
		return nil
	})
	r.logger.Debug("finished reconciling contributor", "result", res)
	return res, errors.Wrap(err, errReconcileOwnerContributor)
}

var _ reconcile.Reconciler = &Reconciler{}

func md5Sum(name string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(name)))
}

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
	principalNotebookController = "cluster.local/ns/kubeflow/sa/notebook-controller-service-account"
)
