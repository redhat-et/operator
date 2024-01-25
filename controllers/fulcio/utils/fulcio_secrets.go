package utils

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

type PEM []byte

type FulcioCertConfig struct {
	PrivateKey         PEM
	PublicKey          PEM
	RootCert           PEM
	PrivateKeyPassword []byte
}

func (c FulcioCertConfig) ToMap() map[string][]byte {
	result := make(map[string][]byte)

	if len(c.PrivateKey) > 0 {
		result["private"] = c.PrivateKey
	}
	if len(c.PublicKey) > 0 {
		result["public"] = c.PublicKey
	}
	if len(c.PrivateKeyPassword) > 0 {
		result["password"] = c.PrivateKeyPassword
	}
	if len(c.RootCert) > 0 {
		result["cert"] = c.RootCert
	}

	return result
}

func CreateCAKey(key *ecdsa.PrivateKey, password []byte) (PEM, error) {
	mKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	block, err := x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", mKey, password, x509.PEMCipherAES256)
	if err != nil {
		return nil, err
	}

	var pemData bytes.Buffer
	if err := pem.Encode(&pemData, block); err != nil {
		return nil, err
	}

	return pemData.Bytes(), nil
}

func CreateCAPub(key crypto.PublicKey) (PEM, error) {
	mPubKey, err := x509.MarshalPKIXPublicKey(key)
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

func CreateFulcioCA(config *FulcioCertConfig, instance *rhtasv1alpha1.Fulcio) (PEM, error) {
	var err error

	if instance.Spec.Certificate.CommonName == "" || instance.Spec.Certificate.OrganizationEmail == "" || instance.Spec.Certificate.OrganizationName == "" {
		return nil, fmt.Errorf("could not create certificate: missing OrganizationName, OrganizationEmail or CommonName from config")
	}

	block, _ := pem.Decode(config.PrivateKey)
	keyBytes := block.Bytes
	if x509.IsEncryptedPEMBlock(block) {
		keyBytes, err = x509.DecryptPEMBlock(block, config.PrivateKeyPassword)
		if err != nil {
			return nil, err
		}
	}

	key, err := x509.ParseECPrivateKey(keyBytes)
	if err != nil {
		return nil, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * 10 * time.Hour)

	issuer := pkix.Name{
		CommonName:   instance.Spec.Certificate.CommonName,
		Organization: []string{instance.Spec.Certificate.OrganizationName},
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(0),
		Subject:               issuer,
		EmailAddresses:        []string{instance.Spec.Certificate.OrganizationEmail},
		SignatureAlgorithm:    x509.ECDSAWithSHA256,
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		Issuer:                issuer,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
	}

	fulcioRoot, err := x509.CreateCertificate(rand.Reader, &template, &template, key.Public(), key)
	if err != nil {
		return nil, err
	}

	var pemFulcioRoot bytes.Buffer
	err = pem.Encode(&pemFulcioRoot, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: fulcioRoot,
	})
	if err != nil {
		return nil, err
	}

	return pemFulcioRoot.Bytes(), nil
}
