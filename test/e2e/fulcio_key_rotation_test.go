//go:build integration

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	tufAction "github.com/securesign/operator/internal/controller/tuf/actions"
	"github.com/securesign/operator/test/e2e/support"
	kubernetes2 "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/tas"
	clients "github.com/securesign/operator/test/e2e/support/tas/cli"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var _ = Describe("Fulcio cert rotation test", Ordered, func() {
	cli, _ := support.CreateClient()
	ctx := context.TODO()
	var (
		targetImageName string
		namespace       *v1.Namespace
		s               *v1alpha1.Securesign
		oldCert         []byte
		newCert         *v1.Secret
		err             error
	)

	AfterEach(func() {
		if CurrentSpecReport().Failed() && support.IsCIEnvironment() {
			support.DumpNamespace(ctx, cli, namespace.Name)
		}
	})

	BeforeAll(func() {
		if _, err := exec.LookPath("tuftool"); err != nil {
			Skip("tuftool command not found")
		}
		namespace = support.CreateTestNamespace(ctx, cli)
		DeferCleanup(func() {
			_ = cli.Delete(ctx, namespace)
		})

		s = &v1alpha1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "test",
				Annotations: map[string]string{
					"rhtas.redhat.com/metrics": "false",
				},
			},
			Spec: v1alpha1.SecuresignSpec{
				Rekor: v1alpha1.RekorSpec{
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
					RekorSearchUI: v1alpha1.RekorSearchUI{
						Enabled: utils.Pointer(true),
					},
				},
				Fulcio: v1alpha1.FulcioSpec{
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
					Config: v1alpha1.FulcioConfig{
						OIDCIssuers: []v1alpha1.OIDCIssuer{
							{
								ClientID:  support.OidcClientID(),
								IssuerURL: support.OidcIssuerUrl(),
								Issuer:    support.OidcIssuerUrl(),
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
					Create: ptr.To(true),
				}},
				TimestampAuthority: &v1alpha1.TimestampAuthoritySpec{
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
					Signer: v1alpha1.TimestampAuthoritySigner{
						CertificateChain: v1alpha1.CertificateChain{
							RootCA: &v1alpha1.TsaCertificateAuthority{
								OrganizationName:  "MyOrg",
								OrganizationEmail: "my@email.org",
								CommonName:        "tsa.hostname",
							},
							IntermediateCA: []*v1alpha1.TsaCertificateAuthority{
								{
									OrganizationName:  "MyOrg",
									OrganizationEmail: "my@email.org",
									CommonName:        "tsa.hostname",
								},
							},
							LeafCA: &v1alpha1.TsaCertificateAuthority{
								OrganizationName:  "MyOrg",
								OrganizationEmail: "my@email.org",
								CommonName:        "tsa.hostname",
							},
						},
					},
					NTPMonitoring: v1alpha1.NTPMonitoring{
						Enabled: true,
						Config: &v1alpha1.NtpMonitoringConfig{
							RequestAttempts: 3,
							RequestTimeout:  5,
							NumServers:      4,
							ServerThreshold: 3,
							MaxTimeDelta:    6,
							Period:          60,
							Servers:         []string{"time.apple.com", "time.google.com", "time-a-b.nist.gov", "time-b-b.nist.gov", "gbg1.ntp.se"},
						},
					},
				},
			},
		}
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

		It("Use cosign cli", func() {
			tas.VerifyByCosign(ctx, cli, s, targetImageName)
		})
	})

	Describe("Fulcio cert rotation", func() {

		It("Download fulcio cert", func() {
			f := fulcio.Get(ctx, cli, namespace.Name, s.Name)()
			Expect(f).ToNot(BeNil())
			oldCert, err = kubernetes.GetSecretData(cli, namespace.Name, f.Status.Certificate.CARef)
			Expect(err).ToNot(HaveOccurred())
			Expect(oldCert).ToNot(BeEmpty())
		})

		It("Update fulcio cert", func() {
			secretName := "new-fulcio-cert"
			newCert = fulcio.CreateSecret(namespace.Name, secretName)
			Expect(cli.Create(ctx, newCert)).To(Succeed())

			Eventually(func(g Gomega) error {
				f := securesign.Get(ctx, cli, namespace.Name, s.Name)()
				g.Expect(f).ToNot(BeNil())
				f.Spec.Fulcio.Certificate.PrivateKeyRef = &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: secretName,
					},
					Key: "private",
				}

				f.Spec.Fulcio.Certificate.PrivateKeyPasswordRef = &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: secretName,
					},
					Key: "password",
				}

				f.Spec.Fulcio.Certificate.CARef = &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: secretName,
					},
					Key: "cert",
				}

				f.Spec.Ctlog.RootCertificates = []v1alpha1.SecretKeySelector{
					{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: secretName,
						},
						Key: "cert",
					},
				}

				return cli.Update(ctx, f)
			}).Should(Succeed())

			// wait a moment for redeploy
			time.Sleep(10 * time.Second)
			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		It("Update TUF repository", func() {
			certs, err := os.MkdirTemp(os.TempDir(), "certs")
			Expect(err).ToNot(HaveOccurred())

			Expect(os.WriteFile(certs+"/new-fulcio.cert.pem", newCert.Data["cert"], 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/fulcio_v1.crt.pem", oldCert, 0644)).To(Succeed())

			tufRepoWorkdir, err := os.MkdirTemp(os.TempDir(), "tuf-repo")
			Expect(err).ToNot(HaveOccurred())

			tufKeys := &v1.Secret{}
			Expect(os.Mkdir(filepath.Join(tufRepoWorkdir, "keys"), 0777)).To(Succeed())
			Expect(cli.Get(ctx, runtimeCli.ObjectKey{Name: "tuf-root-keys", Namespace: namespace.Name}, tufKeys)).To(Succeed())
			for k, v := range tufKeys.Data {
				Expect(os.WriteFile(filepath.Join(tufRepoWorkdir, "keys", k), v, 0644)).To(Succeed())
			}

			Expect(os.Mkdir(filepath.Join(tufRepoWorkdir, "tuf-repo"), 0777)).To(Succeed())
			tufPodList := &v1.PodList{}
			Expect(cli.List(ctx, tufPodList, runtimeCli.InNamespace(namespace.Name), runtimeCli.MatchingLabels{constants.LabelAppComponent: tufAction.ComponentName})).To(Succeed())
			Expect(tufPodList.Items).To(HaveLen(1))

			Expect(kubernetes2.CopyFromPod(ctx, tufPodList.Items[0], "/var/www/html", filepath.Join(tufRepoWorkdir, "tuf-repo"))).To(Succeed())

			Expect(clients.ExecuteInDir(certs, "tuftool", tufToolParams("fulcio_v1.crt.pem", tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tuftool", tufToolParams("new-fulcio.cert.pem", tufRepoWorkdir, false)...)).To(Succeed())

			Expect(kubernetes2.CopyToPod(ctx, config.GetConfigOrDie(), tufPodList.Items[0], filepath.Join(tufRepoWorkdir, "tuf-repo"), "/var/www/html")).To(Succeed())
		})

		It("All other components are running", func() {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		It("Use cosign cli", func() {
			tas.VerifyByCosign(ctx, cli, s, targetImageName)
			newImage := support.PrepareImage(ctx)
			tas.VerifyByCosign(ctx, cli, s, newImage)
		})
	})

})

func tufToolParams(targetName string, workdir string, expire bool) []string {
	args := []string{
		"rhtas",
		"--root", workdir + "/tuf-repo/root.json",
		"--key", workdir + "/keys/snapshot.pem",
		"--key", workdir + "/keys/targets.pem",
		"--key", workdir + "/keys/timestamp.pem",
		"--set-fulcio-target", targetName,
		"--fulcio-uri", "https://fulcio.localhost",
		"--outdir", workdir + "/tuf-repo",
		"--metadata-url", "file://" + workdir + "/tuf-repo",
	}

	if expire {
		args = append(args, "--fulcio-status", "Expired")
	}
	return args
}
