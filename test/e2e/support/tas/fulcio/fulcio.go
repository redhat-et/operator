package fulcio

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/fulcio/actions"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(Get(ctx, cli, namespace, name)).Should(
		WithTransform(func(f *v1alpha1.Fulcio) bool {
			return meta.IsStatusConditionTrue(f.Status.Conditions, constants.Ready)
		}, BeTrue()))

	Eventually(func(g Gomega) (bool, error) {
		return kubernetes.DeploymentIsRunning(ctx, cli, namespace, map[string]string{
			kubernetes.ComponentLabel: actions.ComponentName,
		})
	}).Should(BeTrue())
}

func GetServerPod(ctx context.Context, cli client.Client, ns string) func() *v1.Pod {
	return func() *v1.Pod {
		list := &v1.PodList{}
		_ = cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{kubernetes.ComponentLabel: actions.ComponentName, kubernetes.NameLabel: "fulcio-server"})
		if len(list.Items) != 1 {
			return nil
		}
		return &list.Items[0]
	}
}

func Get(ctx context.Context, cli client.Client, ns string, name string) func() *v1alpha1.Fulcio {
	return func() *v1alpha1.Fulcio {
		instance := &v1alpha1.Fulcio{}
		_ = cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, instance)
		return instance
	}
}

func CreateSecret(ns string, name string) *v1.Secret {
	public, private, root, err := support.CreateCertificates(true)
	if err != nil {
		return nil
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"password": []byte(support.CertPassword),
			"private":  private,
			"public":   public,
			"cert":     root,
		},
	}
}
