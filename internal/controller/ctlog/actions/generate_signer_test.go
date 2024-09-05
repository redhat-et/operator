package actions

import (
	"context"

	. "github.com/onsi/gomega"

	"reflect"
	"testing"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGenerateSigner_CanHandle(t *testing.T) {
	tests := []struct {
		name                    string
		status                  []metav1.Condition
		canHandle               bool
		privateKeyRef           *rhtasv1alpha1.SecretKeySelector
		statusPrivateKeyRef     *rhtasv1alpha1.SecretKeySelector
		privateKeyPassRef       *rhtasv1alpha1.SecretKeySelector
		statusPrivateKeyPassRef *rhtasv1alpha1.SecretKeySelector
	}{
		{
			name: "spec.privateKeyRef is not nil and status.privateKeyRef is nil",
			status: []metav1.Condition{
				{
					Type:   SignerCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:           true,
			privateKeyRef:       &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
			statusPrivateKeyRef: nil,
		},
		{
			name: "spec.privateKeyRef is nil and status.privateKeyRef is not nil",
			status: []metav1.Condition{
				{
					Type:   SignerCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:           false,
			privateKeyRef:       nil,
			statusPrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
		},
		{
			name: "spec.privateKeyRef is nil and status.privateKeyRef is nil",
			status: []metav1.Condition{
				{
					Type:   SignerCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:           true,
			privateKeyRef:       nil,
			statusPrivateKeyRef: nil,
		},
		{
			name: "spec.privateKeyRef != status.privateKeyRef",
			status: []metav1.Condition{
				{
					Type:   SignerCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:           true,
			privateKeyRef:       &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "new_secret"}, Key: "private"},
			statusPrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "old_secret"}, Key: "private"},
		},
		{
			name: "spec.privateKeyRef == status.privateKeyRef",
			status: []metav1.Condition{
				{
					Type:   SignerCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:           false,
			privateKeyRef:       &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
			statusPrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
		},
		{
			name: "spec.privateKeyPasswordRef == status.privateKeyPasswordRef",
			status: []metav1.Condition{
				{
					Type:   SignerCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:               false,
			privateKeyRef:           &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
			statusPrivateKeyRef:     &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
			privateKeyPassRef:       &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "pass"},
			statusPrivateKeyPassRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "pass"},
		},
		{
			name: "spec.privateKeyPasswordRef != status.privateKeyPasswordRef",
			status: []metav1.Condition{
				{
					Type:   SignerCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:               true,
			privateKeyRef:           &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
			statusPrivateKeyRef:     &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
			privateKeyPassRef:       &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "new_secret"}, Key: "pass"},
			statusPrivateKeyPassRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "old_secret"}, Key: "pass"},
		},
		{
			name:                "no phase condition",
			status:              []metav1.Condition{},
			canHandle:           true,
			statusPrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
		},
		{
			name: "ConditionFalse",
			status: []metav1.Condition{
				{
					Type:   SignerCondition,
					Status: metav1.ConditionFalse,
					Reason: constants.Pending,
				},
			},
			statusPrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
			canHandle:           true,
		},
		{
			name: "ConditionTrue",
			status: []metav1.Condition{
				{
					Type:   SignerCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			statusPrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
			canHandle:           false,
		},
		{
			name: "ConditionUnknown",
			status: []metav1.Condition{
				{
					Type:   SignerCondition,
					Status: metav1.ConditionUnknown,
					Reason: constants.Ready,
				},
			},
			statusPrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
			canHandle:           true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewGenerateSignerAction())
			instance := rhtasv1alpha1.CTlog{
				Spec: rhtasv1alpha1.CTlogSpec{
					PrivateKeyRef:         tt.privateKeyRef,
					PrivateKeyPasswordRef: tt.privateKeyPassRef,
				},
				Status: rhtasv1alpha1.CTlogStatus{
					PrivateKeyRef:         tt.statusPrivateKeyRef,
					PrivateKeyPasswordRef: tt.statusPrivateKeyPassRef,
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

func TestGenerateSigner_Handle(t *testing.T) {
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
			name: "use spec.privateKeyRef",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
				},
				status: rhtasv1alpha1.CTlogStatus{
					PrivateKeyRef: nil,
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"private": privateKey,
					}, map[string]string{}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.PrivateKeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.PrivateKeyRef.Name).Should(Equal("secret"))
					g.Expect(instance.Status.PrivateKeyRef.Key).Should(Equal("private"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, SignerCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, ServerConfigCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, PublicKeyCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "generate private key",
			env: env{
				spec:    rhtasv1alpha1.CTlogSpec{},
				status:  rhtasv1alpha1.CTlogStatus{},
				objects: []client.Object{},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.PrivateKeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.PrivateKeyRef.Name).Should(ContainSubstring("ctlog-ctlog-keys-"))

					g.Expect(instance.Status.PrivateKeyPasswordRef).Should(BeNil())

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, SignerCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, ServerConfigCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, PublicKeyCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "replace status.privateKeyRef from spec",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "new_secret"}, Key: "private"},
				},
				status: rhtasv1alpha1.CTlogStatus{
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "old_secret"}, Key: "private"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("new_secret", "default", map[string][]byte{
						"private": privateKey,
					}, map[string]string{}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.PrivateKeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.PrivateKeyRef.Name).Should(Equal("new_secret"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, SignerCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, ServerConfigCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, PublicKeyCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "spec with encrypted private key",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					PrivateKeyRef:         &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PrivateKeyPasswordRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "password"},
				},
				status: rhtasv1alpha1.CTlogStatus{},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"private":  privatePassKey,
						"password": []byte("changeit"),
					}, map[string]string{}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.PrivateKeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.PrivateKeyRef.Name).Should(Equal("secret"))
					g.Expect(instance.Status.PrivateKeyRef.Key).Should(Equal("private"))

					g.Expect(instance.Status.PrivateKeyPasswordRef).ShouldNot(BeNil())
					g.Expect(instance.Status.PrivateKeyPasswordRef.Name).Should(Equal("secret"))
					g.Expect(instance.Status.PrivateKeyPasswordRef.Key).Should(Equal("password"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, SignerCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, ServerConfigCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, PublicKeyCondition)).Should(BeTrue())
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
				Type:   PublicKeyCondition,
				Status: metav1.ConditionFalse,
				Reason: constants.Pending,
			})

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				WithObjects(tt.env.objects...).
				Build()

			a := testAction.PrepareAction(c, NewGenerateSignerAction())

			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.result)
			}
			if tt.want.verify != nil {
				tt.want.verify(g, instance)
			}
		})
	}
}
