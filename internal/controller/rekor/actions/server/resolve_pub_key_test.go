package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"testing"

	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	testAction "github.com/securesign/operator/internal/testing/action"
	httpmock "github.com/securesign/operator/internal/testing/http"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var testPublicKey = []byte("-----BEGIN PUBLIC KEY-----\nMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEy5wMSNagtqLsSF+zf8gBVHm2VThGP69D\ngWyhhIm/BkemPBoD/BNq+/yvD2IjsV4unLp5Lcpv4UAGAPJHL/wm+tHD1nS4QKo/\nsXJ8Ezy1K+bM5DUEilcu4hGgQ7+RCG/H\n-----END PUBLIC KEY-----")
var testPublicKey2 = []byte("-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEZFt6NEqMxaeU76lnlYzFUNjFQGHq\nNF46BPCTlP/FgfMZjN608cDXf3LM5hTbvNyCEabE+4MbOcEMXhDQUlYFvA==\n-----END PUBLIC KEY-----")

func TestResolvePubKey_CanHandle(t *testing.T) {
	tests := []struct {
		name      string
		status    metav1.ConditionStatus
		canHandle bool
		ref       *v1alpha1.SecretKeySelector
	}{
		{
			name:      "ref set",
			status:    metav1.ConditionFalse,
			canHandle: false,
			ref:       &v1alpha1.SecretKeySelector{},
		},
		{
			name:      "no server condition",
			canHandle: false,
		},
		{
			name:      "ServerAvailable == True",
			status:    metav1.ConditionTrue,
			canHandle: true,
		},
		{
			name:      "ServerAvailable == False",
			status:    metav1.ConditionFalse,
			canHandle: false,
		},
		{
			name:      "ServerAvailable == Unknown",
			status:    metav1.ConditionUnknown,
			canHandle: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewResolvePubKeyAction()
			instance := v1alpha1.Rekor{
				Status: v1alpha1.RekorStatus{
					PublicKeyRef: tt.ref,
				},
			}
			if tt.status != "" {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   actions.ServerCondition,
					Status: tt.status,
				})
			}

			if got := a.CanHandle(context.TODO(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestResolvePubKey_Handle(t *testing.T) {
	g := NewWithT(t)
	type env struct {
		objects []client.Object
	}
	type want struct {
		result    *action.Result
		publicKey []byte
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "create new public key secret",
			env: env{
				objects: []client.Object{},
			},
			want: want{
				result:    testAction.StatusUpdate(),
				publicKey: testPublicKey,
			},
		},
		{
			name: "remove label from old secret",
			env: env{
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "old-secret",
							Namespace: "default",
							Labels: map[string]string{
								RekorPubLabel: "public",
							},
						},
						Data: map[string][]byte{
							"public": testPublicKey2,
						},
					},
				},
			},
			want: want{
				result:    testAction.StatusUpdate(),
				publicKey: testPublicKey,
			},
		},
		{
			name: "use existing secret",
			env: env{
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
							Labels: map[string]string{
								RekorPubLabel: "public",
							},
						},
						Data: map[string][]byte{
							"public": testPublicKey,
						},
					},
				},
			},
			want: want{
				result:    testAction.StatusUpdate(),
				publicKey: testPublicKey,
			},
		},
		{
			name: "remove label from old secret and use existing secret",
			env: env{
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "old-secret-1",
							Namespace: "default",
							Labels: map[string]string{
								RekorPubLabel: "public",
							},
						},
						Data: map[string][]byte{
							"public": testPublicKey2,
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
							Labels: map[string]string{
								RekorPubLabel: "public",
							},
						},
						Data: map[string][]byte{
							"public": testPublicKey,
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "old-secret-2",
							Namespace: "default",
							Labels: map[string]string{
								RekorPubLabel: "public",
							},
						},
						Data: map[string][]byte{
							"public": testPublicKey2,
						},
					},
				},
			},
			want: want{
				result:    testAction.StatusUpdate(),
				publicKey: testPublicKey,
			},
		},
		{
			name: "unable to resolve public key",
			want: want{
				result:    testAction.Error(fmt.Errorf("ResolvePubKey: unable to resolve public key: unexpected http response ")),
				publicKey: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			instance := &v1alpha1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rekor",
					Namespace: "default",
				},
				Status: v1alpha1.RekorStatus{
					TreeID:       ptr.To(int64(123456789)),
					PublicKeyRef: nil,
					Conditions: []metav1.Condition{
						{
							Type:   actions.ServerCondition,
							Reason: constants.Initialize,
							Status: metav1.ConditionFalse,
						},
					},
				},
			}

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				WithObjects(tt.env.objects...).Build()
			httpmock.SetMockTransport(http.DefaultClient, map[string]httpmock.RoundTripFunc{
				"http://rekor-server.default.svc/api/v1/log/publicKey": func(req *http.Request) *http.Response {
					if tt.want.publicKey == nil {
						return &http.Response{
							StatusCode: http.StatusBadRequest,
							Header:     make(http.Header),
						}
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader(tt.want.publicKey)),
						Header:     make(http.Header),
					}
				},
			})
			defer httpmock.RestoreDefaultTransport(http.DefaultClient)

			a := testAction.PrepareAction(c, NewResolvePubKeyAction())

			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.result)
			}

			if tt.want.publicKey == nil {
				secrets := v1.SecretList{}
				g.Expect(kubernetes.FindByLabelSelector(ctx, c, &secrets, instance.Namespace, RekorPubLabel)).To(Succeed())
				g.Expect(secrets.Items).Should(BeEmpty())
			} else {
				secrets := v1.SecretList{}
				g.Expect(kubernetes.FindByLabelSelector(ctx, c, &secrets, instance.Namespace, RekorPubLabel)).To(Succeed())
				g.Expect(secrets.Items).Should(HaveLen(1))
				g.Expect(secrets.Items[0].Name).ShouldNot(Equal("old-secret"))
				g.Expect(secrets.Items[0].Data).Should(HaveKeyWithValue("public", testPublicKey))
			}
		})
	}
}
