package trillianUtils

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
)

func UseTLSDb(instance *rhtasv1alpha1.Trillian) bool {

	if instance == nil {
		return false
	}

	// when DB is managed by operator
	if utils.IsEnabled(instance.Spec.Db.Create) && instance.Status.Db.TLS.CertRef != nil {
		return true
	}

	// external DB
	if !utils.IsEnabled(instance.Spec.Db.Create) && instance.GetTrustedCA() != nil {
		return true
	}

	return false
}
