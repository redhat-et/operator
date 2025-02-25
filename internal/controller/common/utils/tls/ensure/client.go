package ensure

import (
	"slices"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/tls"
	corev1 "k8s.io/api/core/v1"
)

// TrustedCA mount config map with trusted CA bundle to all pod template's containers.
func TrustedCA(lor *v1alpha1.LocalObjectReference, containerNames ...string) func(template *corev1.PodTemplateSpec) error {
	return func(template *corev1.PodTemplateSpec) error {
		for i, c := range template.Spec.Containers {
			if slices.Contains(containerNames, c.Name) {
				env := kubernetes.FindEnvByNameOrCreate(&template.Spec.Containers[i], "SSL_CERT_DIR")
				env.Value = tls.CATrustMountPath + ":/var/run/secrets/kubernetes.io/serviceaccount"

				volumeMount := kubernetes.FindVolumeMountByNameOrCreate(&template.Spec.Containers[i], tls.CaTrustVolumeName)
				volumeMount.MountPath = tls.CATrustMountPath
				volumeMount.ReadOnly = true
			}
		}

		projections := make([]corev1.VolumeProjection, 0)
		if lor != nil {
			projections = append(projections, corev1.VolumeProjection{
				ConfigMap: &corev1.ConfigMapProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: lor.Name,
					},
				},
			})
		}

		volume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, tls.CaTrustVolumeName)
		if volume.Projected == nil {
			volume.Projected = &corev1.ProjectedVolumeSource{}
		}
		volume.Projected.Sources = projections

		return nil
	}
}
