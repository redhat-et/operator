package utils

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/securesign/operator/api/v1alpha1"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	curveType = "p256"
)

var (
	ErrPrivateKeyPassword        = errors.New("failed to find private key password")
	ErrDecodePrivateKey          = errors.New("failed to decode private key")
	ErrResolvePrivateKey         = errors.New("failed to resolve private key")
	ErrResolvePrivateKeyPassword = errors.New("failed to resolve private key password")
)

type SignerKey struct {
	privateKey *ecdsa.PrivateKey
	password   []byte
}

func (s SignerKey) PublicKey() (PKIX, error) {
	return x509.MarshalPKIXPublicKey(s.privateKey.Public())
}

func (s SignerKey) PublicKeyPEM() (PEM, error) {
	mPubKey, err := s.PublicKey()
	if err != nil {
		return nil, err
	}
	var pemPubKey bytes.Buffer
	err = pem.Encode(&pemPubKey, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: mPubKey,
	})

	if err != nil {
		return nil, err
	}
	return pemPubKey.Bytes(), nil
}

func (s SignerKey) PrivateKey() (PKIX, error) {
	return x509.MarshalECPrivateKey(s.privateKey)
}

func (s SignerKey) PrivateKeyPEM() (PEM, error) {
	mKey, err := s.PrivateKey()
	if err != nil {
		return nil, err
	}

	var block *pem.Block
	if s.PrivateKeyPassword() != nil {
		block, err = x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", mKey, s.PrivateKeyPassword(), x509.PEMCipherAES256) //nolint:staticcheck
		if err != nil {
			return nil, err
		}
	} else {
		block = &pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: mKey,
		}
	}

	var pemKey bytes.Buffer
	err = pem.Encode(&pemKey, block)
	if err != nil {
		return nil, err
	}

	return pemKey.Bytes(), nil
}

func (s SignerKey) PrivateKeyPassword() []byte {
	return s.password
}

func NewSignerConfig(options ...func(*SignerKey) error) (*SignerKey, error) {
	if len(options) == 0 {
		options = []func(*SignerKey) error{
			WithGeneratedKey(),
		}
	}

	config := &SignerKey{}
	for _, option := range options {
		err := option(config)
		if err != nil {
			return config, err
		}
	}

	return config, nil
}

func WithGeneratedKey() func(*SignerKey) error {
	return func(s *SignerKey) error {
		key, err := ecdsa.GenerateKey(supportedCurves[curveType], rand.Reader)
		if err != nil {
			return err
		}
		s.privateKey = key

		return nil
	}
}

func WithPrivateKeyFromPEM(key PEM, password []byte) func(*SignerKey) error {
	return func(s *SignerKey) error {
		s.password = password

		var err error

		block, _ := pem.Decode(key)
		if block == nil {
			return ErrDecodePrivateKey
		}

		if x509.IsEncryptedPEMBlock(block) { //nolint:staticcheck
			if len(password) == 0 {
				return ErrPrivateKeyPassword
			}

			block.Bytes, err = x509.DecryptPEMBlock(block, password) //nolint:staticcheck
			if err != nil {
				return err
			}
		}

		if s.privateKey, err = x509.ParseECPrivateKey(block.Bytes); err != nil {
			return err
		}

		return nil
	}
}

func ResolveSignerConfig(client client.Client, instance *v1alpha1.CTlog) (*SignerKey, error) {
	var (
		private, password []byte
		err               error
		config            *SignerKey
	)

	private, err = k8sutils.GetSecretData(client, instance.Namespace, instance.Status.PrivateKeyRef)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrResolvePrivateKey, err)
	}
	if instance.Status.PrivateKeyPasswordRef != nil {
		password, err = k8sutils.GetSecretData(client, instance.Namespace, instance.Status.PrivateKeyPasswordRef)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrResolvePrivateKeyPassword, err)
		}
	}
	config, err = NewSignerConfig(WithPrivateKeyFromPEM(private, password))
	if err != nil || config == nil {
		return nil, err
	}
	return config, nil
}
