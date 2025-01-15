package trillianUtils

import (
	"errors"

	"github.com/securesign/operator/internal/images"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateTrillDb(instance *v1alpha1.Trillian, dpName string, sa string, secCont *core.PodSecurityContext, labels map[string]string, useTLS bool) (*apps.Deployment, error) {
	if instance.Status.Db.DatabaseSecretRef == nil {
		return nil, errors.New("reference to database secret is not set")
	}
	if instance.Status.Db.Pvc.Name == "" {
		return nil, errors.New("reference to database pvc is not set")
	}
	replicas := int32(1)
	readinessProbeCommand := "mariadb -u ${MYSQL_USER} -p${MYSQL_PASSWORD} -e \"SELECT 1;\""
	livenessProbeCommand := "mariadb-admin -u ${MYSQL_USER} -p${MYSQL_PASSWORD} ping"

	args := []string{}
	if useTLS {
		readinessProbeCommand += " --ssl"
		livenessProbeCommand += " --ssl"
		args = append(args, "--ssl-cert", "/var/run/secrets/tas/tls.crt")
		args = append(args, "--ssl-key", "/var/run/secrets/tas/tls.key")
	}

	template := core.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
		Spec: core.PodSpec{
			ServiceAccountName: sa,
			SecurityContext:    secCont,
			Volumes: []core.Volume{
				{
					Name: "storage",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: instance.Status.Db.Pvc.Name,
						},
					},
				},
			},
			Containers: []core.Container{
				{
					Command: []string{
						"run-mysqld",
					},
					Args:  args,
					Name:  dpName,
					Image: images.Registry.Get(images.TrillianDb),
					ReadinessProbe: &core.Probe{
						ProbeHandler: core.ProbeHandler{
							Exec: &core.ExecAction{
								Command: []string{
									"bash",
									"-c",
									readinessProbeCommand,
								},
							},
						},
						InitialDelaySeconds: 10,
						PeriodSeconds:       10,
						TimeoutSeconds:      1,
						SuccessThreshold:    1,
						FailureThreshold:    3,
					},
					LivenessProbe: &core.Probe{
						ProbeHandler: core.ProbeHandler{
							Exec: &core.ExecAction{
								Command: []string{
									"bash",
									"-c",
									livenessProbeCommand,
								},
							},
						},
						InitialDelaySeconds: 30,
						TimeoutSeconds:      1,
						PeriodSeconds:       10,
						SuccessThreshold:    1,
						FailureThreshold:    3,
					},
					Ports: []core.ContainerPort{
						{
							Protocol:      core.ProtocolTCP,
							ContainerPort: 3306,
						},
					},
					// Env variables from secret trillian-mysql
					Env: []core.EnvVar{
						{
							Name: "MYSQL_USER",
							ValueFrom: &core.EnvVarSource{
								SecretKeyRef: &core.SecretKeySelector{
									Key: SecretUser,
									LocalObjectReference: core.LocalObjectReference{
										Name: instance.Status.Db.DatabaseSecretRef.Name,
									},
								},
							},
						},
						{
							Name: "MYSQL_PASSWORD",
							ValueFrom: &core.EnvVarSource{
								SecretKeyRef: &core.SecretKeySelector{
									Key: SecretPassword,
									LocalObjectReference: core.LocalObjectReference{
										Name: instance.Status.Db.DatabaseSecretRef.Name,
									},
								},
							},
						},
						{
							Name: "MYSQL_ROOT_PASSWORD",
							ValueFrom: &core.EnvVarSource{
								SecretKeyRef: &core.SecretKeySelector{
									Key: SecretRootPassword,
									LocalObjectReference: core.LocalObjectReference{
										Name: instance.Status.Db.DatabaseSecretRef.Name,
									},
								},
							},
						},
						{
							Name: "MYSQL_PORT",
							ValueFrom: &core.EnvVarSource{
								SecretKeyRef: &core.SecretKeySelector{
									Key: SecretPort,
									LocalObjectReference: core.LocalObjectReference{
										Name: instance.Status.Db.DatabaseSecretRef.Name,
									},
								},
							},
						},
						{
							Name: "MYSQL_DATABASE",
							ValueFrom: &core.EnvVarSource{
								SecretKeyRef: &core.SecretKeySelector{
									Key: SecretDatabaseName,
									LocalObjectReference: core.LocalObjectReference{
										Name: instance.Status.Db.DatabaseSecretRef.Name,
									},
								},
							},
						},
					},
					VolumeMounts: []core.VolumeMount{
						{
							Name:      "storage",
							MountPath: "/var/lib/mysql",
						},
					},
				},
			},
		},
	}

	if err := utils.SetTLS(&template, instance.Status.Db.TLS); err != nil {
		return nil, errors.New("could not set TLS: " + err.Error())
	}

	return &apps.Deployment{
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
			Template: template,
			Strategy: apps.DeploymentStrategy{
				Type: "Recreate",
			},
		},
	}, nil
}
