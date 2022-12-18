package serviceaccount

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/johnhoman/kubeflow-profile-manager/controller/manager"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type OperationResult = controllerutil.OperationResult

type ReconcileFunc func(ctx context.Context, serviceAccount *corev1.ServiceAccount) (OperationResult, error)

func NopReconcileFunc(context.Context, *corev1.ServiceAccount) (OperationResult, error) {
	return controllerutil.OperationResultNone, nil
}

type ReconcilerOption func(r *Reconciler)

func WithLogger(logger logging.Logger) ReconcilerOption {
	return func(r *Reconciler) {
		r.logger = logger
	}
}

func NewReconciler(mgr manager.Manager, opts ...ReconcilerOption) *Reconciler {
	r := &Reconciler{
		client: mgr.GetClient(),
		logger: logging.NewNopLogger(),
	}

	for _, f := range opts {
		f(r)
	}
	return r
}

type Reconciler struct {
	client client.Client
	logger logging.Logger

	// Create the required webhook configuration for
	// mounting a service account
}

// Reconcile reconciles state for each user service account
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	serviceAccount := &corev1.ServiceAccount{}
	if err := r.client.Get(ctx, req.NamespacedName, serviceAccount); err != nil {
		r.logger.Debug("failed to read ServiceAccount")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}
