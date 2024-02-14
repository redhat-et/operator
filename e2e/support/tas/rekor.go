package tas

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func VerifyRekor(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(GetRekor(ctx, cli, namespace, name)).Should(
		WithTransform(func(f *v1alpha1.Rekor) bool {
			return meta.IsStatusConditionTrue(f.Status.Conditions, constants.Ready)
		}, BeTrue()))

	list := &v1.PodList{}

	// server
	cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: actions.ServerComponentName})
	Expect(list.Items).To(And(Not(BeEmpty()), HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning)))))

	// redis
	cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: actions.RedisComponentName})
	Expect(list.Items).To(And(Not(BeEmpty()), HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning)))))
}

func GetRekorServerPod(ctx context.Context, cli client.Client, ns string) func() *v1.Pod {
	return func() *v1.Pod {
		list := &v1.PodList{}
		cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{kubernetes.ComponentLabel: actions.ServerComponentName, kubernetes.NameLabel: "rekor-server"})
		if len(list.Items) != 1 {
			return nil
		}
		return &list.Items[0]
	}
}

func GetRekor(ctx context.Context, cli client.Client, ns string, name string) func() *v1alpha1.Rekor {
	return func() *v1alpha1.Rekor {
		instance := &v1alpha1.Rekor{}
		cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, instance)
		return instance
	}
}

func VerifyRekorSearchUI(ctx context.Context, cli client.Client, namespace string, name string) {
	list := &v1.PodList{}
	cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: actions.UIComponentName})
	Expect(list.Items).To(And(Not(BeEmpty()), HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning)))))
}
