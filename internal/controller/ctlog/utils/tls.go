package utils

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
)

func UseTLS(instance *rhtasv1alpha1.CTlog) bool {

	if instance == nil {
		return false
	}

	// TLS enabled on Ctlog
	if instance.Spec.TLS.CertRef != nil || kubernetes.IsOpenShift() {
		return true
	}

	return false
}
