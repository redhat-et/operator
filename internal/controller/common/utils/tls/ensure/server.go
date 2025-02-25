package ensure

import (
	"slices"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/tls"
	corev1 "k8s.io/api/core/v1"
)

// TLS mount secret with tls cert to all deployment's containers.
func TLS(tlsCfg v1alpha1.TLS, containerNames ...string) func(*corev1.PodTemplateSpec) error {
	return func(template *corev1.PodTemplateSpec) error {
		for i, c := range template.Spec.Containers {
			if slices.Contains(containerNames, c.Name) {
				volumeMount := kubernetes.FindVolumeMountByNameOrCreate(&template.Spec.Containers[i], tls.TLSVolumeName)
				volumeMount.MountPath = tls.TLSVolumeMount
				volumeMount.ReadOnly = true
			}
		}

		volume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, tls.TLSVolumeName)
		if volume.Projected == nil {
			volume.Projected = &corev1.ProjectedVolumeSource{}
		}
		volume.Projected.Sources = []corev1.VolumeProjection{
			{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: tlsCfg.CertRef.Name,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  tlsCfg.CertRef.Key,
							Path: "tls.crt",
						},
					},
				},
			},
			{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: tlsCfg.PrivateKeyRef.Name,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  tlsCfg.PrivateKeyRef.Key,
							Path: "tls.key",
						},
					},
				},
			},
		}
		return nil
	}
}
