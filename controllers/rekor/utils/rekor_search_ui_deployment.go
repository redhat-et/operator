package utils

import (
	"github.com/securesign/operator/controllers/constants"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateRekorSearchUiDeployment(namespace, dpName string, labels map[string]string) *apps.Deployment {
	replicas := int32(1)

	return &apps.Deployment{
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
					ServiceAccountName: constants.ServiceAccountName,
					Containers: []core.Container{
						{
							Name:  dpName,
							Image: constants.RekorSearchUiImage,
							Ports: []core.ContainerPort{
								{
									ContainerPort: 3000,
									Name:          "3000-tcp",
									Protocol:      "TCP",
								},
							},
						},
					},
				},
			},
		},
	}
}
