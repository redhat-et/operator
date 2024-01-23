//go:build integration

package e2e_test

import (
	"context"
	"fmt"
	"github.com/securesign/operator/controllers/rekor"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/matchers"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/e2e/support"
	"github.com/securesign/operator/e2e/support/tas"
	clients "github.com/securesign/operator/e2e/support/tas/cli"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Securesign install with certificate generation", Ordered, func() {
	cli, _ := CreateClient()
	ctx := context.TODO()

	targetImageName := "ttl.sh/" + uuid.New().String() + ":5m"
	var namespace *v1.Namespace
	var securesign *v1alpha1.Securesign

	BeforeAll(func() {
		namespace = support.CreateTestNamespace(ctx, cli)
		DeferCleanup(func() {
			cli.Delete(ctx, namespace)
		})

		securesign = &v1alpha1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "test",
			},
			Spec: v1alpha1.SecuresignSpec{
				Rekor: v1alpha1.RekorSpec{
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
					RekorSearchUI: v1alpha1.RekorSearchUI{
						Enabled: true,
					},
				},
				Fulcio: v1alpha1.FulcioSpec{
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
					Config: v1alpha1.FulcioConfig{
						OIDCIssuers: map[string]v1alpha1.OIDCIssuer{
							support.OidcIssuerUrl(): {
								ClientID:  support.OidcClientID(),
								IssuerURL: support.OidcIssuerUrl(),
								Type:      "email",
							},
						}},
					Certificate: v1alpha1.FulcioCert{
						Create:            true,
						SecretName:        "fulcio-secret",
						OrganizationName:  "MyOrg",
						OrganizationEmail: "my@email.org",
					},
				},
				Ctlog: v1alpha1.CTlogSpec{
					Certificate: v1alpha1.CtlogCert{
						Create: true,
					},
				},
				Tuf: v1alpha1.TufSpec{
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
				},
				Trillian: v1alpha1.TrillianSpec{Db: v1alpha1.TrillianDB{
					Create: true,
				}},
			},
		}
	})

	BeforeAll(func() {
		support.PrepareImage(ctx, targetImageName)
	})

	Describe("Install with autogenerated certificates", func() {
		BeforeAll(func() {
			Expect(cli.Create(ctx, securesign)).To(Succeed())
		})

		It("operator should generate fulcio secret", func() {
			Eventually(func() *v1.Secret {
				fulcioSecret := &v1.Secret{}
				cli.Get(ctx, types.NamespacedName{
					Namespace: namespace.Name,
					Name:      securesign.Spec.Fulcio.Certificate.SecretName,
				}, fulcioSecret)
				return fulcioSecret
			}).Should(
				WithTransform(func(secret *v1.Secret) map[string][]byte { return secret.Data },
					And(
						&matchers.HaveKeyMatcher{Key: "cert"},
						&matchers.HaveKeyMatcher{Key: "private"},
						&matchers.HaveKeyMatcher{Key: "public"},
						&matchers.HaveKeyMatcher{Key: "password"},
					)))
		})

		It("operator should generate rekor secret", func() {
			Eventually(func() *v1.Secret {
				secret := &v1.Secret{}
				cli.Get(ctx, types.NamespacedName{
					Namespace: namespace.Name,
					Name:      fmt.Sprintf(rekor.SecretNameFormat, securesign.Name),
				}, secret)
				return secret
			}).Should(
				WithTransform(func(secret *v1.Secret) map[string][]byte { return secret.Data },
					And(
						&matchers.HaveKeyMatcher{Key: "private"},
					)))
		})

		It("fulcio is running with mounted certs", func() {
			tas.VerifyFulcio(ctx, cli, namespace.Name, securesign.Name)
			server := tas.GetFulcioServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())
			Expect(server.Spec.Volumes).To(
				ContainElement(
					WithTransform(func(volume v1.Volume) string {
						if volume.VolumeSource.Secret != nil {
							return volume.VolumeSource.Secret.SecretName
						}
						return ""
					}, Equal(securesign.Spec.Fulcio.Certificate.SecretName)),
				))

		})

		It("rekor is running with mounted certs", func() {
			tas.VerifyRekor(ctx, cli, namespace.Name, securesign.Name)
			server := tas.GetRekorServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())
			Expect(server.Spec.Volumes).To(
				ContainElement(
					WithTransform(func(volume v1.Volume) string {
						if volume.VolumeSource.Secret != nil {
							return volume.VolumeSource.Secret.SecretName
						}
						return ""
					}, Equal(fmt.Sprintf(rekor.SecretNameFormat, securesign.Name))),
				))

		})

		It("All other components are running", func() {
			tas.VerifyTrillian(ctx, cli, namespace.Name, securesign.Name)
			tas.VerifyCTLog(ctx, cli, namespace.Name, securesign.Name)
			tas.VerifyTuf(ctx, cli, namespace.Name, securesign.Name)
			tas.VerifyRekorSearchUI(ctx, cli, namespace.Name, securesign.Name)
		})

		It("Verify Rekor Search UI is accessible", func() {
			rekor := tas.GetRekor(ctx, cli, namespace.Name, securesign.Name)()
			Expect(rekor).ToNot(BeNil())
			Expect(rekor.Status.RekorSearchUIUrl).NotTo(BeEmpty())

			httpClient := http.Client{
				Timeout: time.Second * 10,
			}
			Eventually(func() bool {
				resp, err := httpClient.Get(rekor.Status.RekorSearchUIUrl)
				if err != nil {
					return false
				}
				defer resp.Body.Close()
				return resp.StatusCode == http.StatusOK
			}, "30s", "1s").Should(BeTrue(), "Rekor UI should be accessible and return a status code of 200")
		})

		It("Use cosign cli", func() {
			fulcio := tas.GetFulcio(ctx, cli, namespace.Name, securesign.Name)()
			Expect(fulcio).ToNot(BeNil())

			rekor := tas.GetRekor(ctx, cli, namespace.Name, securesign.Name)()
			Expect(rekor).ToNot(BeNil())

			tuf := tas.GetTuf(ctx, cli, namespace.Name, securesign.Name)()
			Expect(tuf).ToNot(BeNil())

			oidcToken, err := support.OidcToken(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(oidcToken).ToNot(BeEmpty())

			// sleep for a while to be sure everything has settled down
			time.Sleep(time.Duration(10) * time.Second)

			Expect(clients.Execute("cosign", "initialize", "--mirror="+tuf.Status.Url, "--root="+tuf.Status.Url+"/root.json")).To(Succeed())

			Expect(clients.Execute(
				"cosign", "sign", "-y",
				"--fulcio-url="+fulcio.Status.Url,
				"--rekor-url="+rekor.Status.Url,
				"--oidc-issuer="+support.OidcIssuerUrl(),
				"--identity-token="+oidcToken,
				targetImageName,
			)).To(Succeed())

			Expect(clients.Execute(
				"cosign", "verify",
				"--rekor-url="+rekor.Status.Url,
				"--certificate-identity-regexp", ".*@redhat",
				"--certificate-oidc-issuer-regexp", ".*keycloak.*",
				targetImageName,
			)).To(Succeed())
		})
	})
})
