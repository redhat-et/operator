package actions

import (
	"context"
	"testing"

	constants2 "github.com/securesign/operator/internal/controller/ctlog/constants"
	testAction "github.com/securesign/operator/internal/testing/action"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/fulcio/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func Test_HandleFulcioCert_Autodiscover(t *testing.T) {
	g := NewWithT(t)

	instance := &v1alpha1.CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto",
			Namespace: "default",
		},
		Spec: v1alpha1.CTlogSpec{},
		Status: v1alpha1.CTlogStatus{
			Conditions: []metav1.Condition{
				{
					Type:   constants.Ready,
					Reason: constants.Creating,
					Status: metav1.ConditionFalse,
				},
			},
		},
	}

	c := testAction.FakeClientBuilder().WithObjects(
		kubernetes.CreateSecret("secret", "default",
			map[string][]byte{"key": nil}, map[string]string{actions.FulcioCALabel: "key"}),
		instance,
	).Build()

	i := &v1alpha1.CTlog{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}, i); err != nil {
		t.Error(err)
	}

	a := testAction.PrepareAction(c, NewHandleFulcioCertAction())
	g.Expect(a.CanHandle(context.TODO(), i)).To(BeTrue())

	_ = a.Handle(context.TODO(), i)

	g.Expect(i.Status.RootCertificates).Should(HaveLen(1))
	g.Expect(i.Status.RootCertificates[0].Key).Should(Equal("key"))
	g.Expect(i.Status.RootCertificates[0].Name).Should(Equal("secret"))

	g.Expect(meta.IsStatusConditionTrue(i.Status.Conditions, constants2.CertCondition)).To(BeTrue())
}

func Test_HandleFulcioCert_Empty(t *testing.T) {
	g := NewWithT(t)

	instance := &v1alpha1.CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty",
			Namespace: "default",
		},
		Spec: v1alpha1.CTlogSpec{},
		Status: v1alpha1.CTlogStatus{
			Conditions: []metav1.Condition{
				{
					Type:   constants.Ready,
					Reason: constants.Creating,
					Status: metav1.ConditionFalse,
				},
			},
		},
	}

	c := testAction.FakeClientBuilder().WithObjects(
		instance,
	).Build()

	i := &v1alpha1.CTlog{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}, i); err != nil {
		t.Error(err)
	}

	a := testAction.PrepareAction(c, NewHandleFulcioCertAction())
	g.Expect(a.CanHandle(context.TODO(), i)).To(BeTrue())

	result := a.Handle(context.TODO(), i)
	var dummyAction = action.BaseAction{}
	g.Expect(result).Should(Equal(dummyAction.Requeue()))
}

func Test_HandleFulcioCert_Configured(t *testing.T) {
	g := NewWithT(t)

	instance := &v1alpha1.CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "configured",
			Namespace: "default",
		},
		Spec: v1alpha1.CTlogSpec{
			RootCertificates: []v1alpha1.SecretKeySelector{
				{
					Key:                  "key",
					LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
				},
				{
					Key:                  "key",
					LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret-2"},
				},
			},
		},
		Status: v1alpha1.CTlogStatus{
			Conditions: []metav1.Condition{
				{
					Type:   constants.Ready,
					Reason: constants.Creating,
					Status: metav1.ConditionFalse,
				},
			},
		},
	}

	c := testAction.FakeClientBuilder().WithObjects(
		kubernetes.CreateSecret("secret", "default",
			map[string][]byte{"key": nil}, map[string]string{}),
		instance,
	).Build()

	i := &v1alpha1.CTlog{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}, i); err != nil {
		t.Error(err)
	}

	a := testAction.PrepareAction(c, NewHandleFulcioCertAction())
	g.Expect(a.CanHandle(context.TODO(), i)).To(BeTrue())

	_ = a.Handle(context.TODO(), i)
	g.Expect(i.Status.RootCertificates).Should(HaveLen(2))
	g.Expect(i.Status.RootCertificates[0].Key).Should(Equal("key"))
	g.Expect(i.Status.RootCertificates[0].Name).Should(Equal("secret"))
	g.Expect(i.Status.RootCertificates[1].Key).Should(Equal("key"))
	g.Expect(i.Status.RootCertificates[1].Name).Should(Equal("secret-2"))

	g.Expect(meta.IsStatusConditionTrue(i.Status.Conditions, constants2.CertCondition)).To(BeTrue())
}

func Test_HandleFulcioCert_Configured_Priority(t *testing.T) {
	g := NewWithT(t)

	instance := &v1alpha1.CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "configured-priority",
			Namespace: "default",
		},
		Spec: v1alpha1.CTlogSpec{
			RootCertificates: []v1alpha1.SecretKeySelector{
				{
					Key:                  "key",
					LocalObjectReference: v1alpha1.LocalObjectReference{Name: "my-secret"},
				},
			},
		},
		Status: v1alpha1.CTlogStatus{
			Conditions: []metav1.Condition{
				{
					Type:   constants.Ready,
					Reason: constants.Creating,
					Status: metav1.ConditionFalse,
				},
			},
		},
	}

	c := testAction.FakeClientBuilder().WithObjects(
		kubernetes.CreateSecret("my-secret", "default",
			map[string][]byte{"key": nil}, map[string]string{}),
		kubernetes.CreateSecret("incorrect-secret", "default",
			map[string][]byte{"key": nil}, map[string]string{actions.FulcioCALabel: "key"}),
		instance,
	).Build()

	i := &v1alpha1.CTlog{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}, i); err != nil {
		t.Error(err)
	}

	a := testAction.PrepareAction(c, NewHandleFulcioCertAction())
	g.Expect(a.CanHandle(context.TODO(), i)).To(BeTrue())

	_ = a.Handle(context.TODO(), i)
	g.Expect(i.Status.RootCertificates).Should(HaveLen(1))
	g.Expect(i.Status.RootCertificates[0].Key).Should(Equal("key"))
	g.Expect(i.Status.RootCertificates[0].Name).Should(Equal("my-secret"))

	g.Expect(meta.IsStatusConditionTrue(i.Status.Conditions, constants2.CertCondition)).To(BeTrue())
}

func Test_HandleFulcioCert_Delete_ServerConfig(t *testing.T) {
	g := NewWithT(t)

	instance := &v1alpha1.CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delete-config",
			Namespace: "default",
		},
		Spec: v1alpha1.CTlogSpec{
			RootCertificates: []v1alpha1.SecretKeySelector{
				{
					Key:                  "key",
					LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
				},
			},
		},
		Status: v1alpha1.CTlogStatus{
			ServerConfigRef: &v1alpha1.LocalObjectReference{Name: "ctlog-config"},
			Conditions: []metav1.Condition{
				{
					Type:   constants.Ready,
					Reason: constants.Creating,
					Status: metav1.ConditionFalse,
				},
			},
		},
	}

	c := testAction.FakeClientBuilder().WithObjects(
		kubernetes.CreateImmutableSecret("ctlog-config", instance.Namespace, map[string][]byte{}, map[string]string{}),
		instance,
	).Build()

	i := &v1alpha1.CTlog{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}, i); err != nil {
		t.Error(err)
	}

	a := testAction.PrepareAction(c, NewHandleFulcioCertAction())
	g.Expect(a.CanHandle(context.TODO(), i)).To(BeTrue())

	_ = a.Handle(context.TODO(), i)
	g.Expect(meta.IsStatusConditionTrue(i.Status.Conditions, constants2.CertCondition)).To(BeTrue())

	g.Expect(i.Status.ServerConfigRef).To(BeNil())
	g.Expect(c.Get(context.TODO(), types.NamespacedName{Name: "ctlog-config", Namespace: instance.GetNamespace()}, &v1.Secret{})).To(HaveOccurred())
}
