package utils

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/images"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateRekorSearchUiDeployment(instance *v1alpha1.Rekor, dpName string, sa string, labels map[string]string) *apps.Deployment {
	replicas := int32(1)

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
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: core.PodSpec{
					ServiceAccountName: sa,
					Containers: []core.Container{
						{
							Name: dpName,
							Env: []core.EnvVar{
								{
									Name:  "NEXT_PUBLIC_REKOR_DEFAULT_DOMAIN",
									Value: instance.Status.Url,
								},
							},
							Image: images.Registry.Get(images.RekorSearchUi),
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
