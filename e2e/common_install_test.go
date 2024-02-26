//go:build integration

package e2e_test

import (
	"context"
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

	targetImageName := "ttl.sh/" + uuid.New().String() + ":15m"
	var namespace *v1.Namespace
	var securesign *v1alpha1.Securesign

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			support.DumpNamespace(ctx, cli, namespace.Name)
		}
	})

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
						OrganizationName:  "MyOrg",
						OrganizationEmail: "my@email.org",
						CommonName:        "fulcio",
					},
				},
				Ctlog: v1alpha1.CTlogSpec{},
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

		It("Fulcio is running", func() {
			tas.VerifyFulcio(ctx, cli, namespace.Name, securesign.Name)
		})

		It("operator should generate fulcio secret", func() {
			Eventually(func() *v1.Secret {
				fulcio := tas.GetFulcio(ctx, cli, namespace.Name, securesign.Name)()
				scr := &v1.Secret{}
				Expect(cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: fulcio.Status.Certificate.PrivateKeyRef.Name}, scr)).To(Succeed())
				return scr
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
				rekor := tas.GetRekor(ctx, cli, namespace.Name, securesign.Name)()
				scr := &v1.Secret{}
				Expect(cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: rekor.Status.Signer.KeyRef.Name}, scr)).To(Succeed())
				return scr
				return secret
			}).Should(
				WithTransform(func(secret *v1.Secret) map[string][]byte { return secret.Data },
					And(
						&matchers.HaveKeyMatcher{Key: "private"},
					)))
		})

		It("fulcio is running with mounted certs", func() {
			server := tas.GetFulcioServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())

			sp := []v1.SecretProjection{}
			for _, volume := range server.Spec.Volumes {
				if volume.Name == "fulcio-cert" {
					for _, source := range volume.VolumeSource.Projected.Sources {
						sp = append(sp, *source.Secret)
					}
				}
			}

			Expect(sp).To(
				ContainElement(
					WithTransform(func(sp v1.SecretProjection) string {
						return sp.Name
					}, Equal(tas.GetFulcio(ctx, cli, namespace.Name, securesign.Name)().Status.Certificate.CARef.Name)),
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
					}, Equal(tas.GetRekor(ctx, cli, namespace.Name, securesign.Name)().Status.Signer.KeyRef.Name))),
			)

		})

		It("All other components are running", func() {
			tas.VerifySecuresign(ctx, cli, namespace.Name, securesign.Name)
			tas.VerifyTrillian(ctx, cli, namespace.Name, securesign.Name, true)
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
