package utils

import (
	"crypto/x509"
	_ "embed"
	"testing"

	"github.com/onsi/gomega"
)

var (
	//go:embed testdata/private_key.pem
	privateKey PEM

	//go:embed testdata/public_key.pem
	publicKey PEM

	//go:embed testdata/private_key_pass.pem
	privatePassKey PEM
)

func TestSignerConfig(t *testing.T) {
	type args struct {
		options []func(*SignerKey) error
	}
	tests := []struct {
		name   string
		args   args
		verify func(gomega.Gomega, *SignerKey, error)
	}{
		{
			name: "empty",
			args: args{
				options: []func(*SignerKey) error{},
			},
			verify: func(g gomega.Gomega, config *SignerKey, err error) {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(config.privateKey).NotTo(gomega.BeNil())
				g.Expect(config.password).To(gomega.BeNil())

				pr, err := config.PrivateKeyPEM()
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(pr).ToNot(gomega.BeNil())

				pub, err := config.PublicKeyPEM()
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(pub).ToNot(gomega.BeNil())
			},
		},
		{
			name: "generated signer key",
			args: args{
				options: []func(*SignerKey) error{
					WithGeneratedKey(),
				},
			},
			verify: func(g gomega.Gomega, config *SignerKey, err error) {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(config.privateKey).NotTo(gomega.BeNil())
				g.Expect(config.password).To(gomega.BeNil())

				pr, err := config.PrivateKeyPEM()
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(pr).ToNot(gomega.BeNil())

				pub, err := config.PublicKeyPEM()
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(pub).ToNot(gomega.BeNil())
			},
		},
		{
			name: "from EC PRIVATE KEY PEM",
			args: args{
				options: []func(*SignerKey) error{
					WithPrivateKeyFromPEM(privateKey, nil),
				},
			},
			verify: func(g gomega.Gomega, config *SignerKey, err error) {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(config.privateKey).NotTo(gomega.BeNil())
				g.Expect(config.password).To(gomega.BeNil())

				g.Expect(config.PrivateKeyPEM()).To(gomega.Equal(privateKey))
				g.Expect(config.PublicKeyPEM()).To(gomega.Equal(publicKey))
			},
		},
		{
			name: "from encrypted EC PRIVATE KEY PEM",
			args: args{
				options: []func(*SignerKey) error{
					WithPrivateKeyFromPEM(privatePassKey, []byte("changeit")),
				},
			},
			verify: func(g gomega.Gomega, config *SignerKey, err error) {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(config.privateKey).NotTo(gomega.BeNil())
				g.Expect(config.password).NotTo(gomega.BeNil())

				pr, err := config.PrivateKeyPEM()
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(pr).ToNot(gomega.BeNil())
				g.Expect(string(pr)).To(gomega.ContainSubstring("DEK-Info: AES-256-CBC"))

				pub, err := config.PublicKeyPEM()
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(pub).To(gomega.Equal(publicKey))
			},
		},
		{
			name: "incorrect password",
			args: args{
				options: []func(*SignerKey) error{
					WithPrivateKeyFromPEM(privatePassKey, []byte("incorrect")),
				},
			},
			verify: func(g gomega.Gomega, config *SignerKey, err error) {
				g.Expect(err).To(gomega.HaveOccurred())
				g.Expect(err).To(gomega.MatchError(x509.IncorrectPasswordError))
			},
		},
		{
			name: "incorrect key",
			args: args{
				options: []func(*SignerKey) error{
					WithPrivateKeyFromPEM([]byte("invalid data"), nil),
				},
			},
			verify: func(g gomega.Gomega, config *SignerKey, err error) {
				g.Expect(err).To(gomega.HaveOccurred())
				g.Expect(err).To(gomega.MatchError(ErrDecodePrivateKey))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			got, err := NewSignerConfig(tt.args.options...)
			tt.verify(g, got, err)
		})
	}
}
