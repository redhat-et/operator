package ensure

import (
	"github.com/operator-framework/operator-lib/proxy"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
)

// SetProxyEnvs set the standard environment variables for proxys "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"
func SetProxyEnvs(containers []v1.Container) {
	proxyEnvs := proxy.ReadProxyVarsFromEnv()
	for i := range containers {
		for _, e := range proxyEnvs {
			env := kubernetes.FindEnvByNameOrCreate(&containers[i], e.Name)
			env.Value = e.Value

		}
	}
}
