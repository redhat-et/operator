package actions

import (
	"context"
	_ "embed"
	"reflect"
	"testing"

	"github.com/securesign/operator/internal/controller/ctlog/utils"

	"github.com/onsi/gomega/gstruct"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	//go:embed testdata/private_key.pem
	privateKey utils.PEM
	//go:embed testdata/private_key_pass.pem
	privatePassKey utils.PEM
	//go:embed testdata/public_key.pem
	publicKey utils.PEM
	//go:embed testdata/cert.pem
	cert utils.PEM
)

func TestServerConfig_CanHandle(t *testing.T) {
	tests := []struct {
		name                  string
		status                []metav1.Condition
		canHandle             bool
		serverConfigRef       *rhtasv1alpha1.LocalObjectReference
		statusServerConfigRef *rhtasv1alpha1.LocalObjectReference
	}{
		{
			name: "spec.serverConfigRef is not nil and status.serverConfigRef is nil",
			status: []metav1.Condition{
				{
					Type:   ServerConfigCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:             true,
			serverConfigRef:       &rhtasv1alpha1.LocalObjectReference{Name: "config"},
			statusServerConfigRef: nil,
		},
		{
			name: "spec.serverConfigRef is nil and status.serverConfigRef is not nil",
			status: []metav1.Condition{
				{
					Type:   ServerConfigCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:             false,
			serverConfigRef:       nil,
			statusServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
		},
		{
			name: "spec.serverConfigRef is nil and status.serverConfigRef is nil",
			status: []metav1.Condition{
				{
					Type:   ServerConfigCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:             true,
			serverConfigRef:       nil,
			statusServerConfigRef: nil,
		},
		{
			name: "spec.serverConfigRef != status.serverConfigRef",
			status: []metav1.Condition{
				{
					Type:   ServerConfigCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:             true,
			serverConfigRef:       &rhtasv1alpha1.LocalObjectReference{Name: "new_config"},
			statusServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "old_config"},
		},
		{
			name: "spec.serverConfigRef == status.serverConfigRef",
			status: []metav1.Condition{
				{
					Type:   ServerConfigCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:             false,
			serverConfigRef:       &rhtasv1alpha1.LocalObjectReference{Name: "config"},
			statusServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
		},
		{
			name:      "no phase condition",
			status:    []metav1.Condition{},
			canHandle: true,
		},
		{
			name: "ServerConfigCondition == false",
			status: []metav1.Condition{
				{
					Type:    ServerConfigCondition,
					Status:  metav1.ConditionFalse,
					Reason:  constants.Pending,
					Message: "treeID changed",
				},
			},
			canHandle: true,
		},
		{
			name: "ServerConfigCondition == true",
			status: []metav1.Condition{
				{
					Type:   ServerConfigCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewServerConfigAction())
			instance := rhtasv1alpha1.CTlog{
				Spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: tt.serverConfigRef,
				},
				Status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: tt.statusServerConfigRef,
				},
			}
			for _, status := range tt.status {
				meta.SetStatusCondition(&instance.Status.Conditions, status)
			}

			if got := a.CanHandle(context.TODO(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestServerConfig_Handle(t *testing.T) {
	g := NewWithT(t)
	type env struct {
		spec    rhtasv1alpha1.CTlogSpec
		status  rhtasv1alpha1.CTlogStatus
		objects []client.Object
	}
	type want struct {
		result *action.Result
		verify func(Gomega, *rhtasv1alpha1.CTlog)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "use spec.serverConfigRef",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
				},
				status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: nil,
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(instance.Status.ServerConfigRef.Name).Should(Equal("config"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, ServerConfigCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "create a new config",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: nil,
					Trillian:        rhtasv1alpha1.TrillianService{Port: ptr.To(int32(80))},
				},
				status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: nil,
					TreeID:          ptr.To(int64(123456)),
					RootCertificates: []rhtasv1alpha1.SecretKeySelector{
						{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "cert"},
					},
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PublicKeyRef:  &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"cert":    cert,
						"private": privateKey,
						"public":  publicKey,
					}, map[string]string{}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(instance.Status.ServerConfigRef.Name).Should(ContainSubstring("ctlog-config-"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, ServerConfigCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "replace config from spec",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "new_config"},
				},
				status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "old_config"},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(instance.Status.ServerConfigRef.Name).Should(Equal("new_config"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, ServerConfigCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "Waiting for Fulcio root certificate",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: nil,
					Trillian:        rhtasv1alpha1.TrillianService{Port: ptr.To(int32(80))},
				},
				status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: nil,
					TreeID:          ptr.To(int64(123456)),
					RootCertificates: []rhtasv1alpha1.SecretKeySelector{
						{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "not-existing"}, Key: "cert"},
					},
					PrivateKeyRef:         &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PublicKeyRef:          &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
					PrivateKeyPasswordRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "password"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"cert":    cert,
						"private": privateKey,
						"public":  publicKey,
					}, map[string]string{}),
				},
			},
			want: want{
				result: testAction.Requeue(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.ServerConfigRef).Should(BeNil())
					g.Expect(instance.Status.Conditions).To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Message": ContainSubstring("Waiting for Fulcio root certificate: not-existing/cert"),
					})))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, ServerConfigCondition)).Should(BeFalse())
				},
			},
		},
		{
			name: "Waiting for Ctlog private key secret",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: nil,
					Trillian:        rhtasv1alpha1.TrillianService{Port: ptr.To(int32(80))},
				},
				status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: nil,
					TreeID:          ptr.To(int64(123456)),
					RootCertificates: []rhtasv1alpha1.SecretKeySelector{
						{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "cert"},
					},
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "not-existing"}, Key: "private"},
					PublicKeyRef:  &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"cert":    cert,
						"private": privateKey,
						"public":  publicKey,
					}, map[string]string{}),
				},
			},
			want: want{
				result: testAction.Requeue(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.ServerConfigRef).Should(BeNil())
					g.Expect(instance.Status.Conditions).To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Message": ContainSubstring("Waiting for Ctlog private key secret"),
					})))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, ServerConfigCondition)).Should(BeFalse())
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			instance := &rhtasv1alpha1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ctlog",
					Namespace: "default",
				},
				Spec:   tt.env.spec,
				Status: tt.env.status,
			}

			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   constants.Ready,
				Reason: constants.Pending,
			})

			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   ServerConfigCondition,
				Status: metav1.ConditionFalse,
				Reason: constants.Pending,
			})

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				WithObjects(tt.env.objects...).
				Build()

			a := testAction.PrepareAction(c, NewServerConfigAction())

			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.result)
			}
			if tt.want.verify != nil {
				tt.want.verify(g, instance)
			}
		})
	}
}
