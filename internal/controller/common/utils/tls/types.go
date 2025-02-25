package tls

const (
	CaTrustVolumeName = "ca-trust"
	CATrustMountPath  = "/var/run/configs/tas/ca-trust"

	TLSVolumeName  = "tls-cert"
	TLSVolumeMount = "/var/run/secrets/tas"
	TLSKeyPath     = TLSVolumeMount + "/tls.key"
	TLSCertPath    = TLSVolumeMount + "/tls.crt"
)
