package tsaUtils

import (
	"fmt"
	"strings"

	"github.com/securesign/operator/internal/images"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/constants"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	FileType            = "file"
	KmsType             = "kms"
	TinkType            = "tink"
	chainVolume         = "tsa-cert-chain"
	fileSignerVolume    = "tsa-file-signer-config"
	tinkSignerVolume    = "tsa-tink-signer-config"
	ntpConfigVolume     = "ntp-config"
	authVolumeName      = "auth"
	secretMountPath     = "/var/run/secrets/tas"
	authMountPath       = secretMountPath + "/auth"
	certChainMountPath  = secretMountPath + "/certificate_chain"
	fileSignerMountPath = secretMountPath + "/file_signer"
	tinkSignerMountPath = secretMountPath + "/tink_signer"
	NtpMountPath        = secretMountPath + "/ntp_config"
)

func CreateTimestampAuthorityDeployment(instance *v1alpha1.TimestampAuthority, name string, sa string, labels map[string]string) (*apps.Deployment, error) {
	env := make([]core.EnvVar, 0)
	volumes := make([]core.Volume, 0)
	volumeMounts := make([]core.VolumeMount, 0)
	replicas := int32(1)

	appArgs := []string{
		"timestamp-server",
		"serve",
		"--host=0.0.0.0",
		"--port=3000",
		fmt.Sprintf("--log-type=%s", utils.GetOrDefault(instance.GetAnnotations(), annotations.LogType, string(constants.Prod))),
		fmt.Sprintf("--certificate-chain-path=%s/certificate-chain.pem", certChainMountPath),
		fmt.Sprintf("--disable-ntp-monitoring=%v", !instance.Spec.NTPMonitoring.Enabled),
	}

	volumes = append(volumes, core.Volume{
		Name: chainVolume,
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName: instance.Status.Signer.CertificateChain.CertificateChainRef.Name,
				Items: []core.KeyToPath{
					{
						Key:  instance.Status.Signer.CertificateChain.CertificateChainRef.Key,
						Path: "certificate-chain.pem",
					},
				},
			},
		},
	})

	volumeMounts = append(volumeMounts, core.VolumeMount{
		Name:      chainVolume,
		MountPath: certChainMountPath,
		ReadOnly:  true,
	})

	if instance.Spec.NTPMonitoring.Enabled {
		if instance.Spec.NTPMonitoring.Config != nil {
			volumes = append(volumes, core.Volume{
				Name: ntpConfigVolume,
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{
							Name: instance.Status.NTPMonitoring.Config.NtpConfigRef.Name,
						},
					},
				},
			})

			volumeMounts = append(volumeMounts, core.VolumeMount{
				Name:      ntpConfigVolume,
				MountPath: NtpMountPath,
				ReadOnly:  true,
			})

			appArgs = append(appArgs,
				fmt.Sprintf("--ntp-monitoring=%s/ntp-config.yaml", NtpMountPath),
			)
		}
	}

	switch GetSignerType(&instance.Spec.Signer) {
	case FileType:
		{
			volumes = append(volumes, core.Volume{
				Name: fileSignerVolume,
				VolumeSource: core.VolumeSource{
					Secret: &core.SecretVolumeSource{
						SecretName: instance.Status.Signer.File.PrivateKeyRef.Name,
						Items: []core.KeyToPath{
							{
								Key:  instance.Status.Signer.File.PrivateKeyRef.Key,
								Path: "private_key.pem",
							},
						},
					},
				},
			})

			volumeMounts = append(volumeMounts, core.VolumeMount{
				Name:      fileSignerVolume,
				MountPath: fileSignerMountPath,
				ReadOnly:  true,
			})

			if instance.Status.Signer.File.PasswordRef != nil {
				env = append(env, core.EnvVar{
					Name: "SIGNER_PASSWORD",
					ValueFrom: &core.EnvVarSource{
						SecretKeyRef: &core.SecretKeySelector{
							LocalObjectReference: core.LocalObjectReference{
								Name: instance.Status.Signer.File.PasswordRef.Name,
							},
							Key: instance.Status.Signer.File.PasswordRef.Key,
						},
					},
				})
			}

			appArgs = append(appArgs,
				"--timestamp-signer=file",
				fmt.Sprintf("--file-signer-key-path=%s/private_key.pem", fileSignerMountPath),
				"--file-signer-passwd=$(SIGNER_PASSWORD)",
			)
		}
	case KmsType:
		{

			if instance.Spec.Signer.Kms.Auth != nil {
				if len(instance.Spec.Signer.Kms.Auth.Env) > 0 {
					env = append(env, instance.Spec.Signer.Kms.Auth.Env...)
				}

				if len(instance.Spec.Signer.Kms.Auth.SecretMount) > 0 {
					for _, secret := range instance.Spec.Signer.Kms.Auth.SecretMount {
						volumeName := fmt.Sprintf("%s-%s", authVolumeName, secret.Name)
						volumes = append(volumes, core.Volume{
							Name: volumeName,
							VolumeSource: core.VolumeSource{
								Secret: &core.SecretVolumeSource{
									SecretName: secret.Name,
								},
							},
						})

						volumeMounts = append(volumeMounts, core.VolumeMount{
							Name:      volumeName,
							MountPath: authMountPath,
							ReadOnly:  true,
						})
					}
				}
			}

			appArgs = append(appArgs,
				"--timestamp-signer=kms",
				fmt.Sprintf("--kms-key-resource=%s", instance.Spec.Signer.Kms.KeyResource),
			)
		}
	case TinkType:
		{

			if instance.Spec.Signer.Tink.Auth != nil {
				if len(instance.Spec.Signer.Tink.Auth.Env) > 0 {
					env = append(env, instance.Spec.Signer.Tink.Auth.Env...)
				}

				if len(instance.Spec.Signer.Tink.Auth.SecretMount) > 0 {
					for _, secret := range instance.Spec.Signer.Tink.Auth.SecretMount {
						volumeName := fmt.Sprintf("%s-%s", authVolumeName, secret.Name)
						volumes = append(volumes, core.Volume{
							Name: volumeName,
							VolumeSource: core.VolumeSource{
								Secret: &core.SecretVolumeSource{
									SecretName: secret.Name,
								},
							},
						})

						volumeMounts = append(volumeMounts, core.VolumeMount{
							Name:      volumeName,
							MountPath: authMountPath,
							ReadOnly:  true,
						})
					}
				}
			}

			volumes = append(volumes, core.Volume{
				Name: tinkSignerVolume,
				VolumeSource: core.VolumeSource{
					Secret: &core.SecretVolumeSource{
						SecretName: instance.Spec.Signer.Tink.KeysetRef.Name,
						Items: []core.KeyToPath{
							{
								Key:  instance.Spec.Signer.Tink.KeysetRef.Key,
								Path: "encryptedKeySet",
							},
						},
					},
				},
			})

			volumeMounts = append(volumeMounts, core.VolumeMount{
				Name:      tinkSignerVolume,
				MountPath: tinkSignerMountPath,
				ReadOnly:  true,
			})

			appArgs = append(appArgs,
				"--timestamp-signer=tink",
				fmt.Sprintf("--tink-key-resource=%s", instance.Spec.Signer.Tink.KeyResource),
				fmt.Sprintf("--tink-keyset-path=%s/encryptedKeySet", tinkSignerMountPath),
			)

			if strings.HasPrefix(instance.Spec.Signer.Tink.KeyResource, "hcvault://") {
				appArgs = append(appArgs, "--tink-hcvault-token=$(VAULT_TOKEN)")
			}

		}
	}

	dep := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: core.PodSpec{
					ServiceAccountName: sa,
					Volumes:            volumes,
					Containers: []core.Container{
						{
							Name:         name,
							VolumeMounts: volumeMounts,
							Env:          env,
							Image:        images.Registry.Get(images.TimestampAuthority),
							Ports: []core.ContainerPort{
								{
									Protocol:      core.ProtocolTCP,
									ContainerPort: 3000,
								},
							},
							Command: appArgs,
							LivenessProbe: &core.Probe{
								ProbeHandler: core.ProbeHandler{
									HTTPGet: &core.HTTPGetAction{
										Path: "/ping",
										Port: intstr.FromInt32(3000),
									},
								},
								InitialDelaySeconds: 5,
								TimeoutSeconds:      3,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							ReadinessProbe: &core.Probe{
								ProbeHandler: core.ProbeHandler{
									HTTPGet: &core.HTTPGetAction{
										Path: "/ping",
										Port: intstr.FromInt32(3000),
									},
								},
								InitialDelaySeconds: 5,
								TimeoutSeconds:      3,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
						},
					},
				},
			},
		},
	}
	utils.SetProxyEnvs(dep)

	caRef := utils.TrustedCAAnnotationToReference(instance.Annotations)
	// override if spec.trustedCA is defined
	if instance.Spec.TrustedCA != nil {
		caRef = instance.Spec.TrustedCA
	}
	err := utils.SetTrustedCA(&dep.Spec.Template, caRef)
	if err != nil {
		return nil, err
	}

	return dep, nil
}
