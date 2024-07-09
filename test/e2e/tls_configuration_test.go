//go:build integration

package e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/securesign/operator/internal/controller/common/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/tas"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Securesign TLS Configuration", Ordered, func() {
	cli, _ := CreateClient()
	ctx := context.TODO()

	// var targetImageName string
	var namespace *v1.Namespace
	var securesign *v1alpha1.Securesign

	var caKey *rsa.PrivateKey
	var caCert *x509.Certificate
	var caCertPEM string
	var err error

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			if val, present := os.LookupEnv("CI"); present && val == "true" {
				support.DumpNamespace(ctx, cli, namespace.Name)
			}
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
					Create: utils.Pointer(true),
				}},
			},
		}
	})

	Describe("User-Specified TLS Certificate Configuration", func() {
		BeforeAll(func() {
			caKey, caCert, _, caCertPEM, err = generateCA()
			if err != nil {
				fmt.Printf("Failed to generate CA: %v\n", err)
				return
			}
			ca_configmap := map[string]string{
				"ca.crt": caCertPEM,
			}
			createConfigMap(ctx, cli, namespace.Name, "ca-configmap", ca_configmap)

			var trillianKey string
			var trillianCert string
			trillianKey, trillianCert, err = generateServerCert(caCert, caKey, "trillian-log-server:8090")
			if err != nil {
				fmt.Printf("Failed to generate trillian server certificate: %v\n", err)
				return
			}
			trillian_data := map[string]string{
				"key":  trillianKey,
				"cert": trillianCert,
			}
			createSecret(ctx, cli, namespace.Name, "trillian-server-secret", trillian_data)

			Expect(cli.Create(ctx, securesign)).To(Succeed())
		})

		It("All components are running", func() {
			tas.VerifySecuresign(ctx, cli, namespace.Name, securesign.Name)
			tas.VerifyTrillian(ctx, cli, namespace.Name, securesign.Name, true)
			tas.VerifyCTLog(ctx, cli, namespace.Name, securesign.Name)
			tas.VerifyTuf(ctx, cli, namespace.Name, securesign.Name)
			tas.VerifyRekor(ctx, cli, namespace.Name, securesign.Name)
		})
	})
})

func createSecret(ctx context.Context, cli client.Client, namespace, name string, data map[string]string) {
	secretData := make(map[string][]byte)
	for key, value := range data {
		secretData[key] = []byte(value)
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Data: secretData,
	}

	Expect(cli.Create(ctx, secret)).To(Succeed())
}

func createConfigMap(ctx context.Context, cli client.Client, namespace, name string, data map[string]string) {
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
	Expect(cli.Create(ctx, configMap)).To(Succeed())
}

func generateCA() (*rsa.PrivateKey, *x509.Certificate, string, string, error) {
	caKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, "", "", err
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "My CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 2, 0),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, "", "", err
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return nil, nil, "", "", err
	}

	caKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(caKey)})
	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})

	return caKey, caCert, string(caKeyPEM), string(caCertPEM), nil
}

func generateServerCert(caCert *x509.Certificate, caKey *rsa.PrivateKey, serverName string) (string, string, error) {
	serverKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", err
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName:         "local",
			Organization:       []string{"RedHat"},
			OrganizationalUnit: []string{"RHTAS"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(0, 2, 0),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	serverTemplate.DNSNames = []string{"*", serverName}
	serverTemplate.IPAddresses = []net.IP{net.ParseIP("0.0.0.0")}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return "", "", err
	}

	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})
	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})

	return string(serverKeyPEM), string(serverCertPEM), nil
}
