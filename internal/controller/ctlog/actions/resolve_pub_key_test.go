package actions

import (
	"context"
	_ "embed"
	"reflect"
	"testing"

	"github.com/onsi/gomega/gstruct"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResolvePubKey_CanHandle(t *testing.T) {
	tests := []struct {
		name               string
		status             []metav1.Condition
		canHandle          bool
		publicKeyRef       *rhtasv1alpha1.SecretKeySelector
		statusPublicKeyRef *rhtasv1alpha1.SecretKeySelector
	}{
		{
			name: "spec.publicKeyRef is not nil and status.publicKeyRef is nil",
			status: []metav1.Condition{
				{
					Type:   PublicKeyCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:          true,
			publicKeyRef:       &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
			statusPublicKeyRef: nil,
		},
		{
			name: "spec.publicKeyRef is nil and status.publicKeyRef is not nil",
			status: []metav1.Condition{
				{
					Type:   PublicKeyCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:          false,
			publicKeyRef:       nil,
			statusPublicKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
		},
		{
			name: "spec.publicKeyRef is nil and status.publicKeyRef is nil",
			status: []metav1.Condition{
				{
					Type:   PublicKeyCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:          true,
			publicKeyRef:       nil,
			statusPublicKeyRef: nil,
		},
		{
			name: "spec.publicKeyRef != status.publicKeyRef",
			status: []metav1.Condition{
				{
					Type:   PublicKeyCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:          true,
			publicKeyRef:       &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "new_secret"}, Key: "public"},
			statusPublicKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "old_secret"}, Key: "public"},
		},
		{
			name: "spec.publicKeyRef == status.publicKeyRef",
			status: []metav1.Condition{
				{
					Type:   PublicKeyCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			canHandle:          false,
			publicKeyRef:       &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
			statusPublicKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
		},
		{
			name:               "no phase condition",
			status:             []metav1.Condition{},
			canHandle:          true,
			statusPublicKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
		},
		{
			name: "ConditionFalse",
			status: []metav1.Condition{
				{
					Type:    PublicKeyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  constants.Pending,
					Message: "treeID changed",
				},
			},
			statusPublicKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
			canHandle:          true,
		},
		{
			name: "ConditionTrue",
			status: []metav1.Condition{
				{
					Type:   PublicKeyCondition,
					Status: metav1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			statusPublicKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
			canHandle:          false,
		},
		{
			name: "ConditionUnknown",
			status: []metav1.Condition{
				{
					Type:   PublicKeyCondition,
					Status: metav1.ConditionUnknown,
					Reason: constants.Ready,
				},
			},
			statusPublicKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
			canHandle:          true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewResolvePubKeyAction())
			instance := rhtasv1alpha1.CTlog{
				Spec: rhtasv1alpha1.CTlogSpec{
					PublicKeyRef: tt.publicKeyRef,
				},
				Status: rhtasv1alpha1.CTlogStatus{
					PublicKeyRef: tt.statusPublicKeyRef,
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

func TestResolvePubKey_Handle(t *testing.T) {
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
			name: "use spec.publicKeyRef",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					PublicKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "pub-secret"}, Key: "public"},
				},
				status: rhtasv1alpha1.CTlogStatus{
					PublicKeyRef:  nil,
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "key-secret"}, Key: "private"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("key-secret", "default", map[string][]byte{
						"private": privateKey,
					}, map[string]string{}),
					kubernetes.CreateSecret("pub-secret", "default", map[string][]byte{
						"public": publicKey,
					}, map[string]string{}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.PublicKeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.PublicKeyRef.Name).Should(Equal("pub-secret"))
					g.Expect(instance.Status.PublicKeyRef.Key).Should(Equal("public"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, PublicKeyCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "use spec.publicKeyRef with ctfe.pub label",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					PublicKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "pub-secret"}, Key: "public"},
				},
				status: rhtasv1alpha1.CTlogStatus{
					PublicKeyRef:  nil,
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "key-secret"}, Key: "private"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("key-secret", "default", map[string][]byte{
						"private": privateKey,
					}, map[string]string{}),
					kubernetes.CreateSecret("pub-secret", "default", map[string][]byte{
						"public": publicKey,
					}, map[string]string{
						CTLPubLabel: "public",
					}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.PublicKeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.PublicKeyRef.Name).Should(Equal("pub-secret"))
					g.Expect(instance.Status.PublicKeyRef.Key).Should(Equal("public"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, PublicKeyCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "generate secret from private key",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{},
				status: rhtasv1alpha1.CTlogStatus{
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PublicKeyRef:  nil,
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
					g.Expect(instance.Status.PublicKeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.PublicKeyRef.Name).Should(ContainSubstring("ctlog-ctlog-pub-"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, PublicKeyCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "replace publicKeyRef from spec",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					PublicKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "new_secret"}, Key: "public"},
				},
				status: rhtasv1alpha1.CTlogStatus{
					PublicKeyRef:  &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "old_secret"}, Key: "public"},
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "key-secret"}, Key: "private"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("key-secret", "default", map[string][]byte{
						"private": privateKey,
					}, map[string]string{}),
					kubernetes.CreateSecret("new_secret", "default", map[string][]byte{
						"public": publicKey,
					}, map[string]string{}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.PublicKeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.PublicKeyRef.Name).Should(Equal("new_secret"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, PublicKeyCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "Waiting for Private Key",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{},
				status: rhtasv1alpha1.CTlogStatus{
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "not-existing"}, Key: "private"},
				},
				objects: []client.Object{},
			},
			want: want{
				result: testAction.Requeue(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.PublicKeyRef).Should(BeNil())
					g.Expect(instance.Status.Conditions).To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Message": ContainSubstring("Waiting for secret not-existing"),
					})))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, PublicKeyCondition)).Should(BeFalse())
				},
			},
		},
		{
			name: "Waiting for private key password",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{},
				status: rhtasv1alpha1.CTlogStatus{
					PrivateKeyRef:         &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PrivateKeyPasswordRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "not-existing"}, Key: "password"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"private": privateKey,
					}, map[string]string{}),
				},
			},
			want: want{
				result: testAction.Requeue(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.PublicKeyRef).Should(BeNil())
					g.Expect(instance.Status.Conditions).To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Message": ContainSubstring("Waiting for secret not-existing"),
					})))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, PublicKeyCondition)).Should(BeFalse())
				},
			},
		},
		{
			name: "remove label from old secret",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{},
				status: rhtasv1alpha1.CTlogStatus{
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PublicKeyRef:  &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "old-secret"}, Key: "public"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"private": privateKey,
					}, map[string]string{}),
					kubernetes.CreateSecret("old-secret", "default", map[string][]byte{
						"public": []byte("old public key data"),
					}, map[string]string{
						CTLPubLabel: "public",
					}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.PublicKeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.PublicKeyRef.Name).Should(ContainSubstring("ctlog-ctlog-pub-"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, PublicKeyCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "use existing secret",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{},
				status: rhtasv1alpha1.CTlogStatus{
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PublicKeyRef:  &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "old-secret"}, Key: "public"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"private": privateKey,
					}, map[string]string{}),
					kubernetes.CreateSecret("existing-secret", "default", map[string][]byte{
						"public": publicKey,
					}, map[string]string{
						CTLPubLabel: "public",
					}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog) {
					g.Expect(instance.Status.PublicKeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.PublicKeyRef.Name).Should(ContainSubstring("existing-secret"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, PublicKeyCondition)).Should(BeTrue())
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

			a := testAction.PrepareAction(c, NewResolvePubKeyAction())

			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.result)
			}
			if tt.want.verify != nil {
				tt.want.verify(g, instance)
			}
		})
	}
}
