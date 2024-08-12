//go:build integration

package update

import (
	"context"
	"time"

	"github.com/securesign/operator/test/e2e/support/tas"

	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/tuf"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	ctlogAction "github.com/securesign/operator/internal/controller/ctlog/actions"
	tufAction "github.com/securesign/operator/internal/controller/tuf/actions"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CTlog update", Ordered, func() {
	SetDefaultEventuallyTimeout(time.Duration(5) * time.Minute)
	cli, _ := support.CreateClient()
	ctx := context.TODO()

	var targetImageName string
	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	AfterEach(func() {
		if CurrentSpecReport().Failed() && support.IsCIEnvironment() {
			support.DumpNamespace(ctx, cli, namespace.Name)
		}
	})

	BeforeAll(func() {
		namespace = support.CreateTestNamespace(ctx, cli)
		DeferCleanup(func() {
			_ = cli.Delete(ctx, namespace)
		})
		s = securesignResource(namespace)
	})

	BeforeAll(func() {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with autogenerated certificates", func() {
		BeforeAll(func() {
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All other components are running", func() {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})
	})

	Describe("Change private key", func() {
		var tufGeneration, ctlogGeneration int64

		It("stored current deployment observed generations ", func() {
			tufGeneration = getDeploymentGeneration(ctx, cli,
				types.NamespacedName{Namespace: namespace.Name, Name: tufAction.DeploymentName},
			)
			Expect(tufGeneration).Should(BeNumerically(">", 0))
			ctlogGeneration = getDeploymentGeneration(ctx, cli,
				types.NamespacedName{Namespace: namespace.Name, Name: ctlogAction.DeploymentName},
			)
			Expect(ctlogGeneration).Should(BeNumerically(">", 0))
		})

		It("modified ctlog.privateKeyRef and ctlog.publicKeyRef", func() {
			Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(s), s)).To(Succeed())
			s.Spec.Ctlog.PrivateKeyRef = &v1alpha1.SecretKeySelector{
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: "my-ctlog-secret",
				},
				Key: "private",
			}
			s.Spec.Ctlog.PublicKeyRef = &v1alpha1.SecretKeySelector{
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: "my-ctlog-secret",
				},
				Key: "public",
			}
			Expect(cli.Update(ctx, s)).To(Succeed())
		})

		It("has status Creating: waiting on my-ctlog-secret", func() {
			Eventually(func(g Gomega) string {
				ctl := ctlog.Get(ctx, cli, namespace.Name, s.Name)()
				g.Expect(ctl).NotTo(BeNil())
				return meta.FindStatusCondition(ctl.Status.Conditions, constants.Ready).Reason
			}).Should(Equal(constants.Creating))
		})

		It("created my-ctlog-secret", func() {
			Expect(cli.Create(ctx, ctlog.CreateSecret(namespace.Name, "my-ctlog-secret"))).Should(Succeed())
		})

		It("has status Ready", func() {
			Eventually(func(g Gomega) string {
				ctl := ctlog.Get(ctx, cli, namespace.Name, s.Name)()
				g.Expect(ctl).NotTo(BeNil())
				return meta.FindStatusCondition(ctl.Status.Conditions, constants.Ready).Reason
			}).Should(Equal(constants.Ready))
		})

		It("updated CTlog deployment", func() {
			Eventually(func() int64 {
				return getDeploymentGeneration(ctx, cli, types.NamespacedName{Namespace: namespace.Name, Name: ctlogAction.DeploymentName})
			}).Should(BeNumerically(">", ctlogGeneration))
		})

		It("update TUF deployment", func() {
			Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(s), s)).To(Succeed())
			s.Spec.Tuf.Keys = []v1alpha1.TufKey{
				{
					Name: "rekor.pub",
				},
				{
					Name: "fulcio_v1.crt.pem",
				},
				{
					Name: "tsa.certchain.pem",
				},
				{
					Name: "ctfe.pub",
					SecretRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "my-ctlog-secret",
						},
						Key: "public",
					},
				},
			}
			Expect(cli.Update(ctx, s)).To(Succeed())
			Eventually(func(g Gomega) []v1alpha1.TufKey {
				t := tuf.Get(ctx, cli, namespace.Name, s.Name)()
				return t.Status.Keys
			}).Should(And(HaveLen(4), WithTransform(func(keys []v1alpha1.TufKey) string {
				return keys[3].SecretRef.Name
			}, Equal("my-ctlog-secret"))))
			tuf.RefreshTufRepository(ctx, cli, namespace.Name, s.Name)
		})

		It("verify CTlog and TUF", func() {
			ctlog.Verify(ctx, cli, namespace.Name, s.Name)
			tuf.Verify(ctx, cli, namespace.Name, s.Name)
		})

		It("verify new configuration", func() {
			var ctl *v1alpha1.CTlog
			var ctlPod *v1.Pod
			Eventually(func(g Gomega) {
				ctl = ctlog.Get(ctx, cli, namespace.Name, s.Name)()
				g.Expect(ctl).NotTo(BeNil())
				ctlPod = ctlog.GetServerPod(ctx, cli, namespace.Name)()
				g.Expect(ctlPod).NotTo(BeNil())
			}).Should(Succeed())

			Expect(ctlPod.Spec.Volumes).To(ContainElements(And(
				WithTransform(func(v v1.Volume) string { return v.Name }, Equal("keys")),
				WithTransform(func(v v1.Volume) string { return v.VolumeSource.Secret.SecretName }, Equal(ctl.Status.ServerConfigRef.Name)))))

			existing := &v1.Secret{}
			expected := &v1.Secret{}
			Expect(cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: ctl.Status.ServerConfigRef.Name}, existing)).To(Succeed())
			Expect(cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: "my-ctlog-secret"}, expected)).To(Succeed())

			Expect(existing.Data["public"]).To(Equal(expected.Data["public"]))
		})

		It("verify by cosign", func() {
			tas.VerifyByCosign(ctx, cli, s, targetImageName)
		})
	})
})
