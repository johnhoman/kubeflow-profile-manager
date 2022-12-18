package contributor

import (
	"context"
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/johnhoman/kubeflow-profile-manager/apis/v1alpha1"
	"github.com/johnhoman/kubeflow-profile-manager/controller/manager"
	"istio.io/api/security/v1beta1"
	v1beta12 "istio.io/api/type/v1beta1"
	istiosecurity "istio.io/client-go/pkg/apis/security/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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

func TestReconciler_ReconcileServiceAccount(t *testing.T) {

	cases := map[string]struct {
		contributor *v1alpha1.Contributor
		opts        []ReconcilerOption
		initObjs    []client.Object
		want        *corev1.ServiceAccount
	}{
		"CreatesAServiceAccount": {
			contributor: &v1alpha1.Contributor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "starlord",
					Namespace: "starlord",
				},
				Spec: v1alpha1.ContributorSpec{
					Name: "starlord@guardians.net",
					Role: "Owner",
				},
			},
			opts: []ReconcilerOption{
				WithDefaultServiceAccountReconcilerFunc(),
			},
			initObjs: []client.Object{},
			want: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "starlord",
					Namespace: "starlord",
					Labels: map[string]string{
						"owner.kubeflow.org/id": "c4b21e45ce00680aa4cfea244fcf3889",
					},
					Annotations: map[string]string{
						"owner.kubeflow.org/name": "starlord@guardians.net",
					},
					OwnerReferences: []metav1.OwnerReference{{
						Name:               "starlord",
						Kind:               "Contributor",
						APIVersion:         "kubeflow.org/v1alpha1",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					}},
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
				WithObjects(subtest.contributor).
				WithObjects(subtest.initObjs...).
				Build()

			r := NewReconciler(manager.FromClient(k8s), subtest.opts...)
			res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(subtest.contributor)})
			qt.Assert(t, err, qt.IsNil)
			qt.Assert(t, res, qt.Equals, ctrl.Result{})

			got := &corev1.ServiceAccount{}
			qt.Assert(t, k8s.Get(ctx, client.ObjectKeyFromObject(subtest.want), got), qt.IsNil)
			qt.Assert(t, got, qt.CmpEquals(
				cmpopts.IgnoreFields(*got, "TypeMeta", "ResourceVersion"),
			), subtest.want)
		})
	}
}

func TestReconciler_ReconcileRoleBinding(t *testing.T) {

	cases := map[string]struct {
		contributor *v1alpha1.Contributor
		opts        []ReconcilerOption
		initObjs    []client.Object
		want        *rbacv1.RoleBinding
	}{
		"CreatesARoleBinding": {
			contributor: &v1alpha1.Contributor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "starlord",
					Namespace: "starlord",
				},
				Spec: v1alpha1.ContributorSpec{
					Name: "starlord@guardians.net",
					Role: "Owner",
				},
			},
			opts: []ReconcilerOption{
				WithDefaultRoleBindingReconcilerFunc(),
				WithContributorClusterRole("kubeflow-contributor"),
			},
			initObjs: []client.Object{},
			want: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "starlord",
					Namespace: "starlord",
					OwnerReferences: []metav1.OwnerReference{{
						Name:               "starlord",
						Kind:               "Contributor",
						APIVersion:         "kubeflow.org/v1alpha1",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					}},
					Labels: map[string]string{
						"owner.kubeflow.org/id": "c4b21e45ce00680aa4cfea244fcf3889",
					},
					Annotations: map[string]string{
						"owner":                   "starlord@guardians.net",
						"role":                    "Owner",
						"owner.kubeflow.org/name": "starlord@guardians.net",
					},
				},
				Subjects: []rbacv1.Subject{{
					Kind: "User",
					Name: "starlord@guardians.net",
				}},
				RoleRef: rbacv1.RoleRef{
					APIGroup: rbacv1.GroupName,
					Kind:     "ClusterRole",
					Name:     "kubeflow-contributor",
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
				WithObjects(subtest.contributor).
				WithObjects(subtest.initObjs...).
				Build()

			r := NewReconciler(manager.FromClient(k8s), subtest.opts...)
			res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(subtest.contributor)})
			qt.Assert(t, err, qt.IsNil)
			qt.Assert(t, res, qt.Equals, ctrl.Result{})

			got := &rbacv1.RoleBinding{}
			qt.Assert(t, k8s.Get(ctx, client.ObjectKeyFromObject(subtest.want), got), qt.IsNil)
			qt.Assert(t, got, qt.CmpEquals(
				cmpopts.IgnoreFields(*got, "TypeMeta", "ResourceVersion"),
			), subtest.want)
		})
	}
}

func TestReconciler_ReconcileIstioAuthorizationPolicy(t *testing.T) {
	cases := map[string]struct {
		contributor *v1alpha1.Contributor
		opts        []ReconcilerOption
		initObjs    []client.Object
		want        *istiosecurity.AuthorizationPolicy
	}{
		"CreatesAnAuthorizationPolicy": {
			contributor: &v1alpha1.Contributor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "starlord",
					Namespace: "starlord",
				},
				Spec: v1alpha1.ContributorSpec{
					Role: "Owner",
					Name: "starlord@guardians.net",
				},
			},
			opts: []ReconcilerOption{WithIstioEnabled()},
			want: &istiosecurity.AuthorizationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "starlord-private",
					Namespace: "starlord",
					OwnerReferences: []metav1.OwnerReference{{
						Name:               "starlord",
						Kind:               "Contributor",
						APIVersion:         "kubeflow.org/v1alpha1",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					}},
				},
				Spec: v1beta1.AuthorizationPolicy{
					Action: v1beta1.AuthorizationPolicy_ALLOW,
					Rules: []*v1beta1.Rule{{
						From: []*v1beta1.Rule_From{{
							Source: &v1beta1.Source{
								Principals: []string{
									"cluster.local/ns/istio-system/sa/istio-ingressgateway-service-account",
									"cluster.local/ns/starlord/sa/starlord",
								},
							},
						}},
						When: []*v1beta1.Condition{{
							Key:    fmt.Sprintf("request.headers[kubeflow-userid]"),
							Values: []string{"starlord@guardians.net"},
						}},
					}},
					Selector: &v1beta12.WorkloadSelector{
						MatchLabels: map[string]string{
							"owner.kubeflow.org/id": "c4b21e45ce00680aa4cfea244fcf3889",
						},
					},
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
				WithObjects(subtest.contributor).
				WithObjects(subtest.initObjs...).
				Build()

			opts := append(subtest.opts, WithLogger(logging.NewLogrLogger(zl)))
			r := NewReconciler(manager.FromClient(k8s), opts...)
			res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(subtest.contributor)})

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
