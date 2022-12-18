package profile

import (
	"context"
	"net/http"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/johnhoman/kubeflow-profile-manager/apis/v1alpha1"
	"github.com/johnhoman/kubeflow-profile-manager/controller/manager"
	"istio.io/api/security/v1beta1"
	istiosecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestReconciler_ReconcileNamespace(t *testing.T) {

	cases := map[string]struct {
		profile  *v1alpha1.Profile
		opts     []ReconcilerOption
		initObjs []client.Object
		want     *corev1.Namespace
	}{
		"CreatesANamespace": {
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
				},
				Spec: v1alpha1.ProfileSpec{
					Owner: rbacv1.Subject{
						Kind: "User",
						Name: "starlord@guardians.net",
					},
				},
			},
			opts: []ReconcilerOption{
				WithDefaultNamespaceReconcileFunc(),
			},
			want: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
					Annotations: map[string]string{
						"owner": "starlord@guardians.net",
					},
					OwnerReferences: []metav1.OwnerReference{{
						Name:               "starlord",
						Kind:               "Profile",
						APIVersion:         "kubeflow.org/v1alpha1",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					}},
				},
			},
		},
		"CreatesANamespaceWithLabels": {
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
				},
				Spec: v1alpha1.ProfileSpec{
					Owner: rbacv1.Subject{
						Kind: "User",
						Name: "starlord@guardians.net",
					},
				},
			},
			opts: []ReconcilerOption{
				WithDefaultNamespaceReconcileFunc(),
				WithNamespaceLabels(map[string]string{"istio-injected": "true"}),
			},
			want: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "starlord",
					Labels: map[string]string{"istio-injected": "true"},
					Annotations: map[string]string{
						"owner": "starlord@guardians.net",
					},
					OwnerReferences: []metav1.OwnerReference{{
						Name:               "starlord",
						Kind:               "Profile",
						APIVersion:         "kubeflow.org/v1alpha1",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					}},
				},
			},
		},
		"UpdatesAnExistingNamespaceWithLabels": {
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
				},
				Spec: v1alpha1.ProfileSpec{
					Owner: rbacv1.Subject{
						Kind: "User",
						Name: "starlord@guardians.net",
					},
				},
			},
			initObjs: []client.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "starlord",
						Labels: map[string]string{"istio-injected": "true"},
						Annotations: map[string]string{
							"owner": "starlord@guardians.net",
						},
					},
				},
			},
			opts: []ReconcilerOption{
				WithDefaultNamespaceReconcileFunc(),
				WithNamespaceLabel("app.kubernetes.io/part-of", "kubeflow-profile"),
			},
			want: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
					Labels: map[string]string{
						"istio-injected":            "true",
						"app.kubernetes.io/part-of": "kubeflow-profile",
					},
					Annotations: map[string]string{
						"owner": "starlord@guardians.net",
					},
					OwnerReferences: []metav1.OwnerReference{{
						Name:               "starlord",
						Kind:               "Profile",
						APIVersion:         "kubeflow.org/v1alpha1",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					}},
				},
			},
		},
		"AdoptsAnExistingNamespace": {
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
				},
				Spec: v1alpha1.ProfileSpec{
					Owner: rbacv1.Subject{
						Kind: "User",
						Name: "starlord@guardians.net",
					},
				},
			},
			initObjs: []client.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "starlord",
						OwnerReferences: []metav1.OwnerReference{{
							Name:               "starlord",
							Kind:               "Profile",
							APIVersion:         "kubeflow.org/v1alpha1",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						}},
					},
				},
			},
			opts: []ReconcilerOption{
				WithDefaultNamespaceReconcileFunc(),
				WithNamespaceAdoptionEnabled(),
				WithNamespaceLabel("app.kubernetes.io/part-of", "kubeflow-profile"),
			},
			want: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
					Labels: map[string]string{
						"app.kubernetes.io/part-of": "kubeflow-profile",
					},
					Annotations: map[string]string{
						"owner": "starlord@guardians.net",
					},
					OwnerReferences: []metav1.OwnerReference{{
						Name:               "starlord",
						Kind:               "Profile",
						APIVersion:         "kubeflow.org/v1alpha1",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					}},
				},
			},
		},
		"IgnoresAnExistingNamespaceNotOwnedByAProfile": {
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
				},
				Spec: v1alpha1.ProfileSpec{
					Owner: rbacv1.Subject{
						Kind: "User",
						Name: "starlord@guardians.net",
					},
				},
			},
			initObjs: []client.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "starlord",
					},
				},
			},
			opts: []ReconcilerOption{
				WithDefaultNamespaceReconcileFunc(),
				WithNamespaceLabel("app.kubernetes.io/part-of", "kubeflow-profile"),
			},
			want: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
				},
			},
		},
	}
	qt.Assert(t, v1alpha1.AddToScheme(scheme.Scheme), qt.IsNil)

	ctx := context.Background()
	for name, subtest := range cases {
		t.Run(name, func(t *testing.T) {

			k8s := fake.NewClientBuilder().
				WithScheme(scheme.Scheme).
				WithObjects(subtest.profile).
				WithObjects(subtest.initObjs...).
				Build()

			r := NewReconciler(manager.FromClient(k8s), subtest.opts...)
			res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(subtest.profile)})
			qt.Assert(t, err, qt.IsNil)
			qt.Assert(t, res, qt.Equals, ctrl.Result{})

			got := &corev1.Namespace{}
			qt.Assert(t, k8s.Get(ctx, client.ObjectKeyFromObject(subtest.want), got), qt.IsNil)
			qt.Assert(t, got, qt.CmpEquals(
				cmpopts.IgnoreFields(corev1.Namespace{}, "TypeMeta", "ResourceVersion"),
			), subtest.want)
		})
	}
}

func TestReconciler_ReconcileIstioAuthorizationPolicy(t *testing.T) {
	cases := map[string]struct {
		profile  *v1alpha1.Profile
		opts     []ReconcilerOption
		initObjs []client.Object
		want     *istiosecurity.AuthorizationPolicy
	}{
		"CreatesAnAuthorizationPolicy": {
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
				},
				Spec: v1alpha1.ProfileSpec{
					Owner: rbacv1.Subject{
						Kind: "User",
						Name: "starlord@guardians.net",
					},
				},
			},
			opts: []ReconcilerOption{WithIstioEnabled()},
			want: &istiosecurity.AuthorizationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "control-plane-access",
					Namespace: "starlord",
					Labels: map[string]string{
						"app.kubernetes.io/part-of": "kubeflow-profile",
					},
					OwnerReferences: []metav1.OwnerReference{{
						Name:               "starlord",
						Kind:               "Profile",
						APIVersion:         "kubeflow.org/v1alpha1",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					}},
				},
				Spec: v1beta1.AuthorizationPolicy{
					Action: v1beta1.AuthorizationPolicy_ALLOW,
					Rules: []*v1beta1.Rule{{
						To: []*v1beta1.Rule_To{{
							Operation: &v1beta1.Operation{
								Paths: []string{"/healthz", "/metrics", "/wait-for-drain"},
							},
						}},
					}, {
						From: []*v1beta1.Rule_From{{
							Source: &v1beta1.Source{
								Principals: []string{
									"cluster.local/ns/kubeflow/sa/notebook-controller-service-account",
								},
							},
						}},
						To: []*v1beta1.Rule_To{{
							Operation: &v1beta1.Operation{
								Methods: []string{http.MethodGet},
								Paths:   []string{"*/api/kernels"},
							},
						}},
					}},
				},
			},
		},
	}
	qt.Assert(t, v1alpha1.AddToScheme(scheme.Scheme), qt.IsNil)
	qt.Assert(t, istiosecurity.AddToScheme(scheme.Scheme), qt.IsNil)

	ctx := context.Background()
	zl := zap.New(zap.UseDevMode(true))
	for name, subtest := range cases {
		t.Run(name, func(t *testing.T) {

			k8s := fake.NewClientBuilder().
				WithScheme(scheme.Scheme).
				WithObjects(subtest.profile).
				WithObjects(subtest.initObjs...).
				Build()

			opts := append(subtest.opts, WithLogger(logging.NewLogrLogger(zl)))
			r := NewReconciler(manager.FromClient(k8s), opts...)
			res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(subtest.profile)})

			qt.Assert(t, err, qt.IsNil)
			qt.Assert(t, res, qt.Equals, ctrl.Result{})

			got := &istiosecurity.AuthorizationPolicy{}
			qt.Assert(t, k8s.Get(ctx, client.ObjectKeyFromObject(subtest.want), got), qt.IsNil)

			out, _ := json.Marshal(got)
			have := make(map[string]any)
			qt.Assert(t, json.Unmarshal(out, &have), qt.IsNil)
			want := make(map[string]any)
			out, _ = json.Marshal(subtest.want)
			qt.Assert(t, json.Unmarshal(out, &want), qt.IsNil)
			qt.Assert(t, have, qt.CmpEquals(
				cmpopts.IgnoreMapEntries(func(T, R any) bool {
					return sets.NewString("resourceVersion", "apiVersion", "kind").Has(T.(string))
				}),
			), want)
		})
	}
}

func TestReconciler_ReconcileResourceQuota(t *testing.T) {
	cases := map[string]struct {
		profile  *v1alpha1.Profile
		opts     []ReconcilerOption
		initObjs []client.Object
		want     *corev1.ResourceQuota
	}{
		"CreatesAResourceQuantityFromProfileSpec": {
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
				},
				Spec: v1alpha1.ProfileSpec{
					Owner: rbacv1.Subject{
						Kind: "User",
						Name: "starlord@guardians.net",
					},
					ResourceQuotaSpec: &corev1.ResourceQuotaSpec{
						Hard: corev1.ResourceList{
							"configmaps": resource.MustParse("10"),
						},
					},
				},
			},
			opts: []ReconcilerOption{WithResourceQuotaEnabled()},
			want: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kf-resource-quota",
					Namespace: "starlord",
					Labels: map[string]string{
						"app.kubernetes.io/part-of": "kubeflow-profile",
					},
					OwnerReferences: []metav1.OwnerReference{{
						Name:               "starlord",
						Kind:               "Profile",
						APIVersion:         "kubeflow.org/v1alpha1",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					}},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{
						"configmaps": resource.MustParse("10"),
					},
				},
			},
		},
		"CreatesAResourceQuantityFromDefault": {
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
				},
				Spec: v1alpha1.ProfileSpec{
					Owner: rbacv1.Subject{
						Kind: "User",
						Name: "starlord@guardians.net",
					},
				},
			},
			opts: []ReconcilerOption{
				WithResourceQuotaEnabled(),
				WithDefaultResourceQuotaSpec(corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{
						"configmaps": resource.MustParse("10"),
					},
				}),
			},
			want: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kf-resource-quota",
					Namespace: "starlord",
					Labels: map[string]string{
						"app.kubernetes.io/part-of": "kubeflow-profile",
					},
					OwnerReferences: []metav1.OwnerReference{{
						Name:               "starlord",
						Kind:               "Profile",
						APIVersion:         "kubeflow.org/v1alpha1",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					}},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{
						"configmaps": resource.MustParse("10"),
					},
				},
			},
		},
		"RemovesExistingResourceQuotaLimits": {
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
				},
				Spec: v1alpha1.ProfileSpec{
					Owner: rbacv1.Subject{
						Kind: "User",
						Name: "starlord@guardians.net",
					},
				},
			},
			initObjs: []client.Object{
				&corev1.ResourceQuota{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kf-resource-quota",
						Namespace: "starlord",
						Labels: map[string]string{
							"app.kubernetes.io/part-of": "kubeflow-profile",
						},
						OwnerReferences: []metav1.OwnerReference{{
							Name:               "starlord",
							Kind:               "Profile",
							APIVersion:         "kubeflow.org/v1alpha1",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						}},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: corev1.ResourceList{
							"configmaps": resource.MustParse("10"),
						},
					},
				},
			},
			opts: []ReconcilerOption{
				WithResourceQuotaEnabled(),
			},
			want: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kf-resource-quota",
					Namespace: "starlord",
					Labels: map[string]string{
						"app.kubernetes.io/part-of": "kubeflow-profile",
					},
					OwnerReferences: []metav1.OwnerReference{{
						Name:               "starlord",
						Kind:               "Profile",
						APIVersion:         "kubeflow.org/v1alpha1",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					}},
				},
				Spec: corev1.ResourceQuotaSpec{},
			},
		},
	}
	qt.Assert(t, v1alpha1.AddToScheme(scheme.Scheme), qt.IsNil)
	qt.Assert(t, istiosecurity.AddToScheme(scheme.Scheme), qt.IsNil)

	ctx := context.Background()
	zl := zap.New(zap.UseDevMode(true))
	for name, subtest := range cases {
		t.Run(name, func(t *testing.T) {

			k8s := fake.NewClientBuilder().
				WithScheme(scheme.Scheme).
				WithObjects(subtest.profile).
				WithObjects(subtest.initObjs...).
				Build()

			opts := append(subtest.opts, WithLogger(logging.NewLogrLogger(zl)))
			r := NewReconciler(manager.FromClient(k8s), opts...)
			res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(subtest.profile)})

			qt.Assert(t, err, qt.IsNil)
			qt.Assert(t, res, qt.Equals, ctrl.Result{})

			got := &corev1.ResourceQuota{}
			qt.Assert(t, k8s.Get(ctx, client.ObjectKeyFromObject(subtest.want), got), qt.IsNil)
			qt.Assert(t, got, qt.CmpEquals(
				cmpopts.IgnoreFields(corev1.ResourceQuota{}, "TypeMeta", "ResourceVersion"),
			), subtest.want)
		})
	}
}

func TestReconciler_ReconcileOwner(t *testing.T) {
	cases := map[string]struct {
		profile  *v1alpha1.Profile
		opts     []ReconcilerOption
		initObjs []client.Object
		want     *v1alpha1.Contributor
	}{
		"CreatesAContributorForTheProfileOwner": {
			profile: &v1alpha1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "starlord",
				},
				Spec: v1alpha1.ProfileSpec{
					Owner: rbacv1.Subject{
						Kind: "User",
						Name: "starlord@guardians.net",
					},
					ResourceQuotaSpec: &corev1.ResourceQuotaSpec{
						Hard: corev1.ResourceList{
							"configmaps": resource.MustParse("10"),
						},
					},
				},
			},
			opts: []ReconcilerOption{WithDefaultContributorReconcilerFunc()},
			want: &v1alpha1.Contributor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "starlord",
					Namespace: "starlord",
					OwnerReferences: []metav1.OwnerReference{{
						Name:               "starlord",
						Kind:               "Profile",
						APIVersion:         "kubeflow.org/v1alpha1",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					}},
					Labels: map[string]string{
						"owner.kubeflow.org/id":         "c4b21e45ce00680aa4cfea244fcf3889",
						"contributor.kubeflow.org/role": "admin",
					},
				},
				Spec: v1alpha1.ContributorSpec{
					Name: "starlord@guardians.net",
					Role: "Owner",
				},
			},
		},
	}
	qt.Assert(t, v1alpha1.AddToScheme(scheme.Scheme), qt.IsNil)
	qt.Assert(t, istiosecurity.AddToScheme(scheme.Scheme), qt.IsNil)

	ctx := context.Background()
	zl := zap.New(zap.UseDevMode(true))
	for name, subtest := range cases {
		t.Run(name, func(t *testing.T) {

			k8s := fake.NewClientBuilder().
				WithScheme(scheme.Scheme).
				WithObjects(subtest.profile).
				WithObjects(subtest.initObjs...).
				Build()

			opts := append(subtest.opts, WithLogger(logging.NewLogrLogger(zl)))
			r := NewReconciler(manager.FromClient(k8s), opts...)
			res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(subtest.profile)})

			qt.Assert(t, err, qt.IsNil)
			qt.Assert(t, res, qt.Equals, ctrl.Result{})

			got := &v1alpha1.Contributor{}
			qt.Assert(t, k8s.Get(ctx, client.ObjectKeyFromObject(subtest.want), got), qt.IsNil)
			qt.Assert(t, got, qt.CmpEquals(
				cmpopts.IgnoreFields(v1alpha1.Contributor{}, "TypeMeta", "ResourceVersion"),
			), subtest.want)
		})
	}
}
