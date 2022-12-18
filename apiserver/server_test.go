package apiserver_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/johnhoman/kubeflow-profile-manager/apis/v1alpha1"
	"github.com/johnhoman/kubeflow-profile-manager/apiserver"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestServer_CreateProfile(t *testing.T) {

	cases := map[string]struct {
		options  apiserver.Options
		initObjs []client.Object
		want     client.Object
		body     Body
	}{
		"CreatesAProfile": {
			body: Body{
				"metadata": map[string]any{
					"name": "starlord",
				},
				"spec": map[string]any{
					"owner": map[string]any{
						"kind": "User",
						"name": "starlord@guardians.net",
					},
				},
			},
			want: &v1alpha1.Profile{
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
		},
	}

	ctx := context.Background()

	qt.Assert(t, v1alpha1.AddToScheme(scheme.Scheme), qt.IsNil)

	for name, subtest := range cases {
		t.Run(name, func(t *testing.T) {

			k8s := fake.NewClientBuilder().
				WithObjects(subtest.initObjs...).
				WithScheme(scheme.Scheme).
				Build()

			server := apiserver.NewServer(k8s, subtest.options)

			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/v1/profiles", subtest.body)
			qt.Assert(t, err, qt.IsNil)
			server.ServeHTTP(w, req)

			got := &v1alpha1.Profile{}
			qt.Assert(t, k8s.Get(ctx, client.ObjectKeyFromObject(subtest.want), got), qt.IsNil)
			qt.Assert(t, got, qt.CmpEquals(
				cmpopts.IgnoreFields(v1alpha1.Profile{}, "ResourceVersion", "TypeMeta"),
			), subtest.want)
		})
	}

}

func TestServer_RemoveProfile(t *testing.T) {

	cases := map[string]struct {
		user     string
		profile  string
		options  apiserver.Options
		initObjs []client.Object
		code     int
	}{
		"RemovesAProfile": {
			user:    "starlord@guardians.net",
			profile: "starlord",
			initObjs: []client.Object{
				&v1alpha1.Profile{
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
			},
			code: 200,
		},
	}

	ctx := context.Background()
	qt.Assert(t, v1alpha1.AddToScheme(scheme.Scheme), qt.IsNil)

	for name, subtest := range cases {
		t.Run(name, func(t *testing.T) {

			k8s := fake.NewClientBuilder().
				WithObjects(subtest.initObjs...).
				WithScheme(scheme.Scheme).
				Build()

			server := apiserver.NewServer(k8s, subtest.options)

			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/v1/profiles/"+subtest.profile, nil)
			qt.Assert(t, err, qt.IsNil)

			req.Header.Set(
				"kubeflow-userid",
				subtest.options.UserIDPrefix+subtest.user,
			)
			server.ServeHTTP(w, req)

			// status code matches
			qt.Assert(t, w.Code, qt.Equals, subtest.code)

			if w.Code == 200 {
				// profile was removed
				err = k8s.Get(ctx, client.ObjectKey{Name: subtest.profile}, &v1alpha1.Profile{})
				qt.Assert(t, apierrors.IsNotFound(err), qt.IsTrue)
			}
		})
	}
}

type Body map[string]any

func (b Body) Read(p []byte) (n int, err error) {

	raw, err := json.Marshal(b)
	if err != nil {
		panic(err.(any))
	}
	return bytes.NewReader(raw).Read(p)
}

var _ io.Reader = Body{}
