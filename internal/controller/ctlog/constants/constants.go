package constants

import "github.com/securesign/operator/internal/controller/constants"

const (
	DeploymentName     = "ctlog"
	ComponentName      = "ctlog"
	RBACName           = "ctlog"
	MonitoringRoleName = "prometheus-k8s-ctlog"

	CertCondition    = "FulcioCertAvailable"
	ServerPortName   = "ctlog-server"
	HttpsServerPort  = 443
	ServerPort       = 80
	ServerTargetPort = 6962
	MetricsPortName  = "metrics"
	MetricsPort      = 6963
	ServerCondition  = "ServerAvailable"

	CTLPubLabel = constants.LabelNamespace + "/ctfe.pub"
)
