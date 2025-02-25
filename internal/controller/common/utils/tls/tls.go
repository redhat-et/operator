package tls

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"golang.org/x/exp/maps"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type objectWithTlsClient interface {
	client.Object
	apis.TlsClient
}

func CAPath(ctx context.Context, cli client.Client, instance objectWithTlsClient) (string, error) {
	lor := instance.GetTrustedCA()
	switch {
	case lor != nil:
		cfgTrust, err := kubernetes.GetConfigMap(ctx, cli, instance.GetNamespace(), lor.Name)
		if err != nil {
			return "", err
		}
		if len(cfgTrust.Data) != 1 {
			err = fmt.Errorf("%s ConfigMap can contain only 1 record", lor.Name)
			return "", err
		}
		return CATrustMountPath + maps.Keys(cfgTrust.Data)[0], nil
	case kubernetes.IsOpenShift():
		return "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", nil
	default:
		return "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil
	}
}

func UseTlsClient(instance objectWithTlsClient) bool {
	return kubernetes.IsOpenShift() || instance.GetTrustedCA() != nil
}
