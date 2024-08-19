//go:build upgrade

package e2e

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	semver "github.com/blang/semver/v4"
	"github.com/onsi/ginkgo/v2/dsl/core"
	"github.com/onsi/gomega/matchers"
	v12 "github.com/operator-framework/api/pkg/operators/v1"
	tasv1alpha "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/constants"
	ctl "github.com/securesign/operator/internal/controller/ctlog/actions"
	fulcio "github.com/securesign/operator/internal/controller/fulcio/actions"
	rekor "github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/securesign/actions"
	trillian "github.com/securesign/operator/internal/controller/trillian/actions"
	tuf "github.com/securesign/operator/internal/controller/tuf/actions"
	"github.com/securesign/operator/test/e2e/support/tas"
	clients "github.com/securesign/operator/test/e2e/support/tas/cli"
	v13 "k8s.io/api/apps/v1"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
)

const testCatalog = "test-catalog"

var _ = Describe("Operator upgrade", Ordered, func() {
	gomega.SetDefaultEventuallyTimeout(5 * time.Minute)
	cli, _ := CreateClient()
	ctx := context.TODO()

	var (
		namespace                              *v1.Namespace
		baseCatalogImage, targetedCatalogImage string
		base, updated                          semver.Version
		securesignDeployment                   *tasv1alpha.Securesign
		rfulcio                                *tasv1alpha.Fulcio
		rrekor                                 *tasv1alpha.Rekor
		rtuf                                   *tasv1alpha.Tuf
		oidcToken                              string
		prevImageName, newImageName            string
		openshift                              bool
	)

	AfterEach(func() {
		if CurrentSpecReport().Failed() && support.IsCIEnvironment() {
			support.DumpNamespace(ctx, cli, namespace.Name)
			csvs := &v1alpha1.ClusterServiceVersionList{}
			gomega.Expect(cli.List(ctx, csvs, runtimeCli.InNamespace(namespace.Name))).To(gomega.Succeed())
			core.GinkgoWriter.Println("\n\nClusterServiceVersions:")
			for _, p := range csvs.Items {
				core.GinkgoWriter.Printf("%s %s %s\n", p.Name, p.Spec.Version, p.Status.Phase)
			}

			catalogs := &v1alpha1.CatalogSourceList{}
			gomega.Expect(cli.List(ctx, catalogs, runtimeCli.InNamespace(namespace.Name))).To(gomega.Succeed())
			core.GinkgoWriter.Println("\n\nCatalogSources:")
			for _, p := range catalogs.Items {
				core.GinkgoWriter.Printf("%s %s %s\n", p.Name, p.Spec.Image, p.Status.GRPCConnectionState.LastObservedState)
			}
		}
	})

	BeforeAll(func() {

		baseCatalogImage = os.Getenv("TEST_BASE_CATALOG")
		targetedCatalogImage = os.Getenv("TEST_TARGET_CATALOG")
		openshift, _ = strconv.ParseBool(os.Getenv("OPENSHIFT"))

		namespace = support.CreateTestNamespace(ctx, cli)
		DeferCleanup(func() {
			_ = cli.Delete(ctx, namespace)
		})
	})

	BeforeAll(func() {
		prevImageName = support.PrepareImage(ctx)
		newImageName = support.PrepareImage(ctx)
	})

	It("Install catalogSource", func() {
		gomega.Expect(baseCatalogImage).To(gomega.Not(gomega.BeEmpty()))
		gomega.Expect(targetedCatalogImage).To(gomega.Not(gomega.BeEmpty()))

		gomega.Expect(support.CreateOrUpdateCatalogSource(ctx, cli, namespace.Name, testCatalog, baseCatalogImage)).To(gomega.Succeed())

		gomega.Eventually(func(g gomega.Gomega) *v1alpha1.CatalogSource {
			c := &v1alpha1.CatalogSource{}
			g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: testCatalog}, c)).To(gomega.Succeed())
			return c
		}).Should(gomega.And(gomega.Not(gomega.BeNil()), gomega.WithTransform(func(c *v1alpha1.CatalogSource) string {
			if c.Status.GRPCConnectionState == nil {
				return ""
			}
			return c.Status.GRPCConnectionState.LastObservedState
		}, gomega.Equal("READY"))))
	})

	It("Install TAS", func() {
		og := &v12.OperatorGroup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "e2e-test",
			},
			Spec: v12.OperatorGroupSpec{
				TargetNamespaces: []string{},
			},
		}
		gomega.Expect(cli.Create(ctx, og)).To(gomega.Succeed())
		subscription := &v1alpha1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test",
				Namespace: namespace.Name,
			},
			Spec: &v1alpha1.SubscriptionSpec{
				CatalogSource:          testCatalog,
				CatalogSourceNamespace: namespace.Name,
				Package:                "rhtas-operator",
				Channel:                "stable",
				Config: &v1alpha1.SubscriptionConfig{
					Env: []v1.EnvVar{
						{
							Name:  "OPENSHIFT",
							Value: strconv.FormatBool(openshift),
						},
					},
				},
			},
			Status: v1alpha1.SubscriptionStatus{},
		}

		gomega.Expect(cli.Create(ctx, subscription)).To(gomega.Succeed())

		gomega.Eventually(func(g gomega.Gomega) {
			csvs := &v1alpha1.ClusterServiceVersionList{}
			g.Expect(cli.List(ctx, csvs, runtimeCli.InNamespace(namespace.Name))).To(gomega.Succeed())
		}).Should(gomega.Succeed())

		gomega.Eventually(findClusterServiceVersion(ctx, cli, func(_ v1alpha1.ClusterServiceVersion) bool {
			return true
		}, namespace.Name)).Should(gomega.Not(gomega.BeNil()))

		base = findClusterServiceVersion(ctx, cli, func(_ v1alpha1.ClusterServiceVersion) bool {
			return true
		}, namespace.Name)().Spec.Version.Version

		gomega.Eventually(func(g gomega.Gomega) []v13.Deployment {
			list := &v13.DeploymentList{}
			g.Expect(cli.List(ctx, list, runtimeCli.InNamespace(namespace.Name), runtimeCli.MatchingLabels{"app.kubernetes.io/part-of": "rhtas-operator"})).To(gomega.Succeed())
			return list.Items
		}).Should(gomega.And(gomega.HaveLen(1), gomega.WithTransform(func(items []v13.Deployment) int32 {
			return items[0].Status.AvailableReplicas
		}, gomega.BeNumerically(">=", 1))))
	})

	It("Install securesign", func() {
		securesignDeployment = &tasv1alpha.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "test",
				Annotations: map[string]string{
					"rhtas.redhat.com/metrics": "false",
				},
			},
			Spec: tasv1alpha.SecuresignSpec{
				Rekor: tasv1alpha.RekorSpec{
					ExternalAccess: tasv1alpha.ExternalAccess{
						Enabled: true,
					},
					RekorSearchUI: tasv1alpha.RekorSearchUI{
						Enabled: utils.Pointer(true),
					},
				},
				Fulcio: tasv1alpha.FulcioSpec{
					ExternalAccess: tasv1alpha.ExternalAccess{
						Enabled: true,
					},
					Config: tasv1alpha.FulcioConfig{
						OIDCIssuers: []tasv1alpha.OIDCIssuer{
							{
								ClientID:  support.OidcClientID(),
								IssuerURL: support.OidcIssuerUrl(),
								Issuer:    support.OidcIssuerUrl(),
								Type:      "email",
							},
						}},
					Certificate: tasv1alpha.FulcioCert{
						OrganizationName:  "MyOrg",
						OrganizationEmail: "my@email.org",
						CommonName:        "fulcio",
					},
				},
				Ctlog: tasv1alpha.CTlogSpec{},
				Tuf: tasv1alpha.TufSpec{
					ExternalAccess: tasv1alpha.ExternalAccess{
						Enabled: true,
					},
				},
				Trillian: tasv1alpha.TrillianSpec{Db: tasv1alpha.TrillianDB{
					Create: utils.Pointer(true),
				}},
			},
		}

		gomega.Expect(cli.Create(ctx, securesignDeployment)).To(gomega.Succeed())

		tas.VerifySecuresign(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyFulcio(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyRekor(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyTrillian(ctx, cli, namespace.Name, securesignDeployment.Name, true)
		tas.VerifyCTLog(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyTuf(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyRekorSearchUI(ctx, cli, namespace.Name, securesignDeployment.Name)
	})

	It("Sign image with cosign cli", func() {
		rfulcio = tas.GetFulcio(ctx, cli, namespace.Name, securesignDeployment.Name)()
		gomega.Expect(rfulcio).ToNot(gomega.BeNil())

		rrekor = tas.GetRekor(ctx, cli, namespace.Name, securesignDeployment.Name)()
		gomega.Expect(rrekor).ToNot(gomega.BeNil())

		rtuf = tas.GetTuf(ctx, cli, namespace.Name, securesignDeployment.Name)()
		gomega.Expect(rtuf).ToNot(gomega.BeNil())

		var err error
		oidcToken, err = support.OidcToken(ctx)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(oidcToken).ToNot(gomega.BeEmpty())

		// sleep for a while to be sure everything has settled down
		time.Sleep(time.Duration(10) * time.Second)

		gomega.Expect(clients.Execute("cosign", "initialize", "--mirror="+rtuf.Status.Url, "--root="+rtuf.Status.Url+"/root.json")).To(gomega.Succeed())

		gomega.Expect(clients.Execute(
			"cosign", "sign", "-y",
			"--fulcio-url="+rfulcio.Status.Url,
			"--rekor-url="+rrekor.Status.Url,
			"--oidc-issuer="+support.OidcIssuerUrl(),
			"--oidc-client-id="+support.OidcClientID(),
			"--identity-token="+oidcToken,
			prevImageName,
		)).To(gomega.Succeed())
	})

	It("Upgrade operator", func() {
		gomega.Expect(support.CreateOrUpdateCatalogSource(ctx, cli, namespace.Name, testCatalog, targetedCatalogImage)).To(gomega.Succeed())

		gomega.Eventually(func(g gomega.Gomega) *v1alpha1.CatalogSource {
			c := &v1alpha1.CatalogSource{}
			g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: testCatalog}, c)).To(gomega.Succeed())
			return c
		}).Should(gomega.And(gomega.Not(gomega.BeNil()), gomega.WithTransform(func(c *v1alpha1.CatalogSource) string {
			if c.Status.GRPCConnectionState == nil {
				return ""
			}
			return c.Status.GRPCConnectionState.LastObservedState
		}, gomega.Equal("READY"))))

		gomega.Eventually(findClusterServiceVersion(ctx, cli, func(csv v1alpha1.ClusterServiceVersion) bool {
			return csv.Spec.Version.Version.String() == base.String()
		}, namespace.Name)).WithTimeout(5 * time.Minute).Should(gomega.WithTransform(func(csv *v1alpha1.ClusterServiceVersion) v1alpha1.ClusterServiceVersionPhase {
			return csv.Status.Phase
		}, gomega.Equal(v1alpha1.CSVPhaseReplacing)))

		gomega.Eventually(findClusterServiceVersion(ctx, cli, func(csv v1alpha1.ClusterServiceVersion) bool {
			return csv.Spec.Version.Version.String() != base.String()
		}, namespace.Name)).WithTimeout(5 * time.Minute).Should(gomega.And(gomega.Not(gomega.BeNil()), gomega.WithTransform(func(csv *v1alpha1.ClusterServiceVersion) v1alpha1.ClusterServiceVersionPhase {
			return csv.Status.Phase
		}, gomega.Equal(v1alpha1.CSVPhaseSucceeded))))

		updated = findClusterServiceVersion(ctx, cli, func(csv v1alpha1.ClusterServiceVersion) bool {
			return csv.Spec.Version.Version.String() != base.String()
		}, namespace.Name)().Spec.Version.Version
	})

	It("Verify deployment was upgraded", func() {
		gomega.Expect(updated.GT(base)).To(gomega.BeTrue())

		for k, v := range map[string]string{
			fulcio.DeploymentName:            constants.FulcioServerImage,
			ctl.DeploymentName:               constants.CTLogImage,
			tuf.DeploymentName:               constants.TufImage,
			rekor.ServerDeploymentName:       constants.RekorServerImage,
			rekor.SearchUiDeploymentName:     constants.RekorSearchUiImage,
			trillian.LogsignerDeploymentName: constants.TrillianLogSignerImage,
			trillian.LogserverDeploymentName: constants.TrillianServerImage,
		} {
			gomega.Eventually(func(g gomega.Gomega) string {
				d := &v13.Deployment{}
				g.Expect(cli.Get(ctx, types.NamespacedName{
					Namespace: namespace.Name,
					Name:      k,
				}, d)).To(gomega.Succeed())

				return d.Spec.Template.Spec.Containers[0].Image
			}).Should(gomega.Equal(v), fmt.Sprintf("Expected %s deployment image to be equal to %s", k, v))
		}

		tas.VerifySecuresign(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyFulcio(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyRekor(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyTrillian(ctx, cli, namespace.Name, securesignDeployment.Name, true)
		tas.VerifyCTLog(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyTuf(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyRekorSearchUI(ctx, cli, namespace.Name, securesignDeployment.Name)
	})

	It("Verify image signature after upgrade", func() {
		gomega.Expect(clients.Execute(
			"cosign", "verify",
			"--rekor-url="+rrekor.Status.Url,
			"--certificate-identity-regexp", ".*@redhat",
			"--certificate-oidc-issuer-regexp", ".*keycloak.*",
			prevImageName,
		)).To(gomega.Succeed())
	})

	It("Sign and Verify new image after upgrade", func() {
		gomega.Expect(clients.Execute(
			"cosign", "sign", "-y",
			"--fulcio-url="+rfulcio.Status.Url,
			"--rekor-url="+rrekor.Status.Url,
			"--oidc-issuer="+support.OidcIssuerUrl(),
			"--oidc-client-id="+support.OidcClientID(),
			"--identity-token="+oidcToken,
			newImageName,
		)).To(gomega.Succeed())

		gomega.Expect(clients.Execute(
			"cosign", "verify",
			"--rekor-url="+rrekor.Status.Url,
			"--certificate-identity-regexp", ".*@redhat",
			"--certificate-oidc-issuer-regexp", ".*keycloak.*",
			newImageName,
		)).To(gomega.Succeed())
	})

	It("Install Timestamp Authority after upgrade", func() {
		securesign := tas.GetSecuresign(ctx, cli, namespace.Name, securesignDeployment.Name)()
		gomega.Expect(securesign).ToNot(gomega.BeNil())
		gomega.Expect(meta.FindStatusCondition(securesign.GetConditions(), actions.TSACondition).Reason).To(gomega.Equal(constants.NotDefined))

		securesign.Spec.TimestampAuthority = tasv1alpha.TimestampAuthoritySpec{
			ExternalAccess: tasv1alpha.ExternalAccess{
				Enabled: true,
			},
			Signer: tasv1alpha.TimestampAuthoritySigner{
				CertificateChain: tasv1alpha.CertificateChain{
					RootCA: tasv1alpha.TsaCertificateAuthority{
						OrganizationName:  "MyOrg",
						OrganizationEmail: "my@email.org",
						CommonName:        "tsa.hostname",
					},
					IntermediateCA: []tasv1alpha.TsaCertificateAuthority{
						{
							OrganizationName:  "MyOrg",
							OrganizationEmail: "my@email.org",
							CommonName:        "tsa.hostname",
						},
					},
					LeafCA: tasv1alpha.TsaCertificateAuthority{
						OrganizationName:  "MyOrg",
						OrganizationEmail: "my@email.org",
						CommonName:        "tsa.hostname",
					},
				},
			},
			NTPMonitoring: tasv1alpha.NTPMonitoring{
				Enabled: true,
				Config: &tasv1alpha.NtpMonitoringConfig{
					RequestAttempts: 3,
					RequestTimeout:  5,
					NumServers:      4,
					ServerThreshold: 3,
					MaxTimeDelta:    6,
					Period:          60,
					Servers:         []string{"time.apple.com", "time.google.com", "time-a-b.nist.gov", "time-b-b.nist.gov", "gbg1.ntp.se"},
				},
			},
		}
		gomega.Expect(cli.Update(ctx, securesign)).Should(gomega.Succeed())
	})

	It("tsa should reach a ready state", func() {
		gomega.Eventually(func() string {
			securesign := tas.GetSecuresign(ctx, cli, namespace.Name, securesignDeployment.Name)()
			gomega.Expect(securesign).ToNot(gomega.BeNil())
			return meta.FindStatusCondition(securesign.Status.Conditions, actions.TSACondition).Reason
		}).Should(gomega.Equal(constants.Ready))
	})

	It("operator should generate TSA secret", func() {
		securesign := tas.GetSecuresign(ctx, cli, namespace.Name, securesignDeployment.Name)()
		gomega.Expect(securesign).ToNot(gomega.BeNil())
		tsa := tas.GetTSA(ctx, cli, namespace.Name, securesign.Name)()

		gomega.Eventually(func() *v1.Secret {
			scr := &v1.Secret{}
			gomega.Expect(cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: tsa.Status.Signer.File.PrivateKeyRef.Name}, scr)).To(gomega.Succeed())
			return scr
		}).Should(
			gomega.WithTransform(func(secret *v1.Secret) map[string][]byte { return secret.Data },
				gomega.And(
					&matchers.HaveKeyMatcher{Key: "rootPrivateKey"},
					&matchers.HaveKeyMatcher{Key: "rootPrivateKeyPassword"},
					&matchers.HaveKeyMatcher{Key: "interPrivateKey-0"},
					&matchers.HaveKeyMatcher{Key: "interPrivateKeyPassword-0"},
					&matchers.HaveKeyMatcher{Key: "leafPrivateKey"},
					&matchers.HaveKeyMatcher{Key: "leafPrivateKeyPassword"},
					&matchers.HaveKeyMatcher{Key: "certificateChain"},
				)))
	})

	It("tsa is running with operator generated certs and keys", func() {
		securesign := tas.GetSecuresign(ctx, cli, namespace.Name, securesignDeployment.Name)()
		gomega.Expect(securesign).ToNot(gomega.BeNil())
		tsa := tas.GetTSA(ctx, cli, namespace.Name, securesign.Name)()
		server := tas.GetTSAServerPod(ctx, cli, namespace.Name)()

		gomega.Expect(server).NotTo(gomega.BeNil())
		gomega.Expect(server.Spec.Volumes).To(
			gomega.ContainElement(
				gomega.WithTransform(func(volume v1.Volume) string {
					if volume.VolumeSource.Secret != nil {
						return volume.VolumeSource.Secret.SecretName
					}
					return ""
				}, gomega.Equal(tsa.Status.Signer.CertificateChain.CertificateChainRef.Name))),
		)
		gomega.Expect(server.Spec.Volumes).To(
			gomega.ContainElement(
				gomega.WithTransform(func(volume v1.Volume) string {
					if volume.VolumeSource.Secret != nil {
						return volume.VolumeSource.Secret.SecretName
					}
					return ""
				}, gomega.Equal(tsa.Status.Signer.File.PrivateKeyRef.Name))),
		)
	})

	It("ntp monitoring config created", func() {
		securesign := tas.GetSecuresign(ctx, cli, namespace.Name, securesignDeployment.Name)()
		gomega.Expect(securesign).ToNot(gomega.BeNil())
		tsa := tas.GetTSA(ctx, cli, namespace.Name, securesign.Name)()

		server := tas.GetTSAServerPod(ctx, cli, namespace.Name)()
		gomega.Expect(server).NotTo(gomega.BeNil())
		gomega.Expect(server.Spec.Volumes).To(
			gomega.ContainElement(
				gomega.WithTransform(func(volume v1.Volume) string {
					if volume.VolumeSource.ConfigMap != nil {
						return volume.VolumeSource.ConfigMap.Name
					}
					return ""
				}, gomega.Equal(tsa.Status.NTPMonitoring.Config.NtpConfigRef.Name))),
		)
	})

	It("Update tuf root with tsa certificate chain", func() {
		securesign := tas.GetSecuresign(ctx, cli, namespace.Name, securesignDeployment.Name)()
		gomega.Expect(securesign).ToNot(gomega.BeNil())

		securesign.Spec.Tuf.Keys = append(securesign.Spec.Tuf.Keys, tasv1alpha.TufKey{Name: "tsa.certchain.pem"})
		gomega.Expect(cli.Update(ctx, securesign)).To(gomega.Succeed())
		gomega.Eventually(func() string {
			tuf := tas.GetTuf(ctx, cli, namespace.Name, securesign.Name)()
			return meta.FindStatusCondition(tuf.Status.Conditions, constants.Ready).Reason
		}).Should(gomega.Equal(constants.Ready))
	})

	It("Sign and Verify image after tsa install", func() {
		securesign := tas.GetSecuresign(ctx, cli, namespace.Name, securesignDeployment.Name)()
		gomega.Expect(securesign).ToNot(gomega.BeNil())
		tsa := tas.GetTSA(ctx, cli, namespace.Name, securesign.Name)()

		gomega.Expect(tsa).ToNot(gomega.BeNil())
		err := tas.GetTSACertificateChain(ctx, cli, tsa.Namespace, tsa.Name, tsa.Status.Url)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		gomega.Expect(clients.Execute("cosign", "initialize", "--mirror="+rtuf.Status.Url, "--root="+rtuf.Status.Url+"/root.json")).To(gomega.Succeed())

		gomega.Expect(clients.Execute(
			"cosign", "verify",
			"--rekor-url="+rrekor.Status.Url,
			"--certificate-identity-regexp", ".*@redhat",
			"--certificate-oidc-issuer-regexp", ".*keycloak.*",
			prevImageName,
		)).To(gomega.Succeed())

		gomega.Expect(clients.Execute(
			"cosign", "sign", "-y",
			"--fulcio-url="+rfulcio.Status.Url,
			"--rekor-url="+rrekor.Status.Url,
			"--timestamp-server-url="+tsa.Status.Url+"/api/v1/timestamp",
			"--oidc-issuer="+support.OidcIssuerUrl(),
			"--oidc-client-id="+support.OidcClientID(),
			"--identity-token="+oidcToken,
			newImageName,
		)).To(gomega.Succeed())

		gomega.Expect(clients.Execute(
			"cosign", "verify",
			"--rekor-url="+rrekor.Status.Url,
			"--timestamp-certificate-chain=ts_chain.pem",
			"--certificate-identity-regexp", ".*@redhat",
			"--certificate-oidc-issuer-regexp", ".*keycloak.*",
			newImageName,
		)).To(gomega.Succeed())
	})
})

func findClusterServiceVersion(ctx context.Context, cli runtimeCli.Client, conditions func(version v1alpha1.ClusterServiceVersion) bool, ns string) func() *v1alpha1.ClusterServiceVersion {
	return func() *v1alpha1.ClusterServiceVersion {
		lst := v1alpha1.ClusterServiceVersionList{}
		if err := cli.List(ctx, &lst, runtimeCli.InNamespace(ns)); err != nil {
			panic(err)
		}
		for _, s := range lst.Items {
			if strings.Contains(s.Name, "rhtas-operator") && conditions(s) {
				return &s
			}
		}
		return nil
	}
}
