package apis

import "github.com/securesign/operator/api/v1alpha1"

type TlsClient interface {
	GetTrustedCA() *v1alpha1.LocalObjectReference
}
