package deployment

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	ensureTls "github.com/securesign/operator/internal/controller/common/utils/tls/ensure"
	v1 "k8s.io/api/apps/v1"
)

func Proxy() func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		ensure.SetProxyEnvs(dp.Spec.Template.Spec.Containers)
		return nil
	}
}

// TrustedCA mount config map with trusted CA bundle to all deployment's containers.
func TrustedCA(lor *v1alpha1.LocalObjectReference, containerNames ...string) func(dp *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return ensureTls.TrustedCA(lor, containerNames...)(&dp.Spec.Template)
	}
}

// TLS mount secret with tls cert to all deployment's containers.
func TLS(tls v1alpha1.TLS, containerNames ...string) func(dp *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		return ensureTls.TLS(tls, containerNames...)(&dp.Spec.Template)
	}
}
