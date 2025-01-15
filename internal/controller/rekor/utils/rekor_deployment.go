package utils

import (
	"fmt"

	"github.com/securesign/operator/internal/images"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/constants"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateRekorDeployment(instance *v1alpha1.Rekor, dpName string, sa string, labels map[string]string) (*apps.Deployment, error) {
	switch {
	case instance.Status.ServerConfigRef == nil:
		return nil, fmt.Errorf("CreateRekorDeployment: %w", ServerConfigNotSpecified)
	case instance.Status.TreeID == nil:
		return nil, fmt.Errorf("CreateRekorDeployment: %w", TreeNotSpecified)
	case instance.Spec.Trillian.Address == "":
		return nil, fmt.Errorf("CreateRekorDeployment: %w", TrillianAddressNotSpecified)
	case instance.Spec.Trillian.Port == nil:
		return nil, fmt.Errorf("CreateRekorDeployment: %w", TrillianPortNotSpecified)
	}

	env := make([]core.EnvVar, 0)

	appArgs := []string{
		"serve",
		fmt.Sprintf("--trillian_log_server.address=%s", instance.Spec.Trillian.Address),
		fmt.Sprintf("--trillian_log_server.port=%d", *instance.Spec.Trillian.Port),
		"--trillian_log_server.sharding_config=/sharding/sharding-config.yaml",
		"--redis_server.address=rekor-redis",
		"--redis_server.port=6379",
		"--rekor_server.address=0.0.0.0",
		"--enable_retrieve_api=true",
		fmt.Sprintf("--trillian_log_server.tlog_id=%d", *instance.Status.TreeID),
		"--enable_attestation_storage",
		"--attestation_storage_bucket=file:///var/run/attestations",
		fmt.Sprintf("--log_type=%s", utils.GetOrDefault(instance.GetAnnotations(), annotations.LogType, string(constants.Prod))),
	}
	volumes := []core.Volume{
		{
			Name: "rekor-sharding-config",
			VolumeSource: core.VolumeSource{
				ConfigMap: &core.ConfigMapVolumeSource{
					LocalObjectReference: core.LocalObjectReference{
						Name: instance.Status.ServerConfigRef.Name,
					},
				},
			},
		},
		{
			Name: "storage",
			VolumeSource: core.VolumeSource{
				PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
					ClaimName: instance.Status.PvcName,
				},
			},
		},
	}
	volumeMounts := []core.VolumeMount{
		{
			Name:      "rekor-sharding-config",
			MountPath: "/sharding",
		},
		{
			Name:      "storage",
			MountPath: "/var/run/attestations",
		},
	}

	containerPorts := []core.ContainerPort{
		{
			ContainerPort: 3000,
			Name:          "rekor-server",
		},
	}

	if instance.Spec.Monitoring.Enabled {
		containerPorts = append(containerPorts, core.ContainerPort{
			ContainerPort: 2112,
			Protocol:      "TCP",
		})
	}

	// KMS memory
	if instance.Spec.Signer.KMS == "memory" {
		appArgs = append(appArgs, "--rekor_server.signer=memory")
	}

	// KMS secret
	if instance.Spec.Signer.KMS == "secret" || instance.Spec.Signer.KMS == "" {
		if instance.Status.Signer.KeyRef == nil {
			return nil, SignerKeyNotSpecified
		}
		svsPrivate := &core.SecretVolumeSource{
			SecretName: instance.Status.Signer.KeyRef.Name,
			Items: []core.KeyToPath{
				{
					Key:  instance.Status.Signer.KeyRef.Key,
					Path: "private",
				},
			},
		}

		appArgs = append(appArgs, "--rekor_server.signer=/key/private")

		volumes = append(volumes, core.Volume{
			Name: "rekor-private-key-volume",
			VolumeSource: core.VolumeSource{
				Secret: svsPrivate,
			},
		})

		volumeMounts = append(volumeMounts, core.VolumeMount{
			Name:      "rekor-private-key-volume",
			MountPath: "/key",
			ReadOnly:  true,
		})

		// Add signer password
		if instance.Status.Signer.PasswordRef != nil {
			appArgs = append(appArgs, "--rekor_server.signer-passwd=$(SIGNER_PASSWORD)")
			env = append(env, core.EnvVar{
				Name: "SIGNER_PASSWORD",
				ValueFrom: &core.EnvVarSource{
					SecretKeyRef: &core.SecretKeySelector{
						Key: instance.Status.Signer.PasswordRef.Key,
						LocalObjectReference: core.LocalObjectReference{
							Name: instance.Status.Signer.PasswordRef.Name,
						},
					},
				},
			})
		}
	}
	//TODO mount additional ENV variables and secrets to enable cloud KMS service

	replicas := int32(1)
	dep := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpName,
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
							Name:         dpName,
							Image:        images.Registry.Get(images.RekorServer),
							Ports:        containerPorts,
							Env:          env,
							Args:         appArgs,
							VolumeMounts: volumeMounts,
							LivenessProbe: &core.Probe{
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      1,
								FailureThreshold:    3,
								SuccessThreshold:    1,
								ProbeHandler: core.ProbeHandler{
									HTTPGet: &core.HTTPGetAction{
										Port: intstr.FromInt32(3000),
										Path: "/ping",
									},
								},
							},
							ReadinessProbe: &core.Probe{
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								TimeoutSeconds:      1,
								FailureThreshold:    3,
								SuccessThreshold:    1,
								ProbeHandler: core.ProbeHandler{
									HTTPGet: &core.HTTPGetAction{
										Port: intstr.FromInt32(3000),
										Path: "/ping",
									},
								},
							},
						},
					},
				},
			},
			Strategy: apps.DeploymentStrategy{
				Type: "Recreate",
			},
		},
	}
	utils.SetProxyEnvs(dep)
	return dep, nil
}
