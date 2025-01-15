package utils

import (
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/images"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateRedisDeployment(namespace string, dpName string, sa string, labels map[string]string) *apps.Deployment {
	replicas := int32(1)
	// Define a new Namespace object
	dep := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpName,
			Namespace: namespace,
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
					Volumes: []core.Volume{
						{
							Name: "storage",
							VolumeSource: core.VolumeSource{
								EmptyDir: &core.EmptyDirVolumeSource{},
							},
						},
					},
					Containers: []core.Container{
						{
							Name:  dpName,
							Image: images.Registry.Get(images.RekorRedis),
							Ports: []core.ContainerPort{
								{
									Protocol:      core.ProtocolTCP,
									ContainerPort: 6379,
								},
							},
							ReadinessProbe: &core.Probe{
								ProbeHandler: core.ProbeHandler{
									Exec: &core.ExecAction{
										Command: []string{
											"/bin/sh",
											"-c",
											"-i",
											"test $(redis-cli -h 127.0.0.1 ping) = 'PONG'",
										},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
								TimeoutSeconds:      1,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							VolumeMounts: []core.VolumeMount{
								{
									Name:      "storage",
									MountPath: "/data",
								},
							},
						},
					},
				},
			},
		},
	}
	utils.SetProxyEnvs(dep)
	return dep
}
