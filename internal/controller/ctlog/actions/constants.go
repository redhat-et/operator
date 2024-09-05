package actions

import "github.com/securesign/operator/internal/controller/constants"

const (
	DeploymentName     = "ctlog"
	ComponentName      = "ctlog"
	RBACName           = "ctlog"
	MonitoringRoleName = "prometheus-k8s-ctlog"

	SignerCondition       = "SignerAvailable"
	CertCondition         = "FulcioCertAvailable"
	ServerConfigCondition = "ServerConfigAvailable"
	PublicKeyCondition    = "PublicKeyAvailable"
	TreeCondition         = "TreeAvailable"

	ServerPortName   = "http"
	ServerPort       = 80
	ServerTargetPort = 6962
	MetricsPortName  = "metrics"
	MetricsPort      = 6963

	CTLPubLabel = constants.LabelNamespace + "/ctfe.pub"
)
