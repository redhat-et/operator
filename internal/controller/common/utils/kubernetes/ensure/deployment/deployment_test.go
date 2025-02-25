package deployment

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/tls"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/apps/v1"
	v3 "k8s.io/api/core/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const name = "dp"

func TestEnsureTrustedCA(t *testing.T) {
	gomega.RegisterTestingT(t)
	t.Run("update existing object", func(t *testing.T) {

		ctx := context.TODO()
		c := testAction.FakeClientBuilder().
			WithObjects(&v1.Deployment{
				ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
				Spec: v1.DeploymentSpec{
					Template: v3.PodTemplateSpec{
						Spec: v3.PodSpec{
							Containers: []v3.Container{
								{
									Name: name, Image: "test",
									Env: []v3.EnvVar{
										{Name: "NAME", Value: "VALUE"},
									},
									VolumeMounts: []v3.VolumeMount{
										{
											MountPath: "path",
											Name:      "mount",
										},
									},
								},
							},
							Volumes: []v3.Volume{
								{
									Name: "mount",
								},
							},
						},
					},
				},
			}).
			Build()

		result, err := kubernetes.CreateOrUpdate(ctx, c,
			&v1.Deployment{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
			TrustedCA(&v1alpha1.LocalObjectReference{Name: "test"}, name),
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		gomega.Expect(result).To(gomega.Equal(controllerutil.OperationResultUpdated))

		existing := &v1.Deployment{}
		gomega.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, existing)).To(gomega.Succeed())
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].Env).To(gomega.HaveLen(2))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].Env[0].Name).To(gomega.Equal("NAME"))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].Env[0].Value).To(gomega.Equal("VALUE"))

		gomega.Expect(existing.Spec.Template.Spec.Containers[0].Env[1].Name).To(gomega.Equal("SSL_CERT_DIR"))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].Env[1].Value).To(gomega.Equal("/var/run/configs/tas/ca-trust:/var/run/secrets/kubernetes.io/serviceaccount"))

		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts).To(gomega.HaveLen(2))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(gomega.Equal("mount"))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath).To(gomega.Equal("path"))

		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[1].Name).To(gomega.Equal("ca-trust"))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[1].MountPath).To(gomega.Equal("/var/run/configs/tas/ca-trust"))

		gomega.Expect(existing.Spec.Template.Spec.Volumes).To(gomega.HaveLen(2))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[0].Name).To(gomega.Equal("mount"))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[1].Name).To(gomega.Equal("ca-trust"))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[1].Projected.Sources).To(gomega.HaveLen(1))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[1].Projected.Sources[0].ConfigMap.Name).To(gomega.Equal("test"))

	})
}

func TestEnsureTLS(t *testing.T) {
	gomega.RegisterTestingT(t)
	t.Run("update existing object", func(t *testing.T) {

		ctx := context.TODO()
		c := testAction.FakeClientBuilder().
			WithObjects(&v1.Deployment{
				ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
				Spec: v1.DeploymentSpec{
					Template: v3.PodTemplateSpec{
						Spec: v3.PodSpec{
							Containers: []v3.Container{
								{Name: name, Image: "test"},
								{Name: "doNotUpdate", Image: "test"},
							},
						},
					},
				},
			}).
			Build()

		result, err := kubernetes.CreateOrUpdate(ctx, c,
			&v1.Deployment{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
			TLS(v1alpha1.TLS{
				PrivateKeyRef: &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: "testSecret",
					},
					Key: "key",
				},
				CertRef: &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: "testSecret",
					},
					Key: "cert",
				},
			}, name),
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		gomega.Expect(result).To(gomega.Equal(controllerutil.OperationResultUpdated))

		existing := &v1.Deployment{}
		gomega.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, existing)).To(gomega.Succeed())

		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts).To(gomega.HaveLen(1))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(gomega.Equal(tls.TLSVolumeName))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath).To(gomega.Equal("/var/run/secrets/tas"))

		gomega.Expect(existing.Spec.Template.Spec.Containers[1].VolumeMounts).To(gomega.BeEmpty())

		gomega.Expect(existing.Spec.Template.Spec.Volumes).To(gomega.HaveLen(1))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[0].Name).To(gomega.Equal(tls.TLSVolumeName))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[0].Projected.Sources).To(gomega.HaveLen(2))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[0].Projected.Sources).To(gomega.ContainElements(
			gomega.And(
				gomega.WithTransform(func(s v3.VolumeProjection) string {
					return s.Secret.Name
				}, gomega.Equal("testSecret")),
				gomega.WithTransform(func(s v3.VolumeProjection) string {
					return s.Secret.Items[0].Key
				}, gomega.Equal("key")),
			),
			gomega.And(
				gomega.WithTransform(func(s v3.VolumeProjection) string {
					return s.Secret.Name
				}, gomega.Equal("testSecret")),
				gomega.WithTransform(func(s v3.VolumeProjection) string {
					return s.Secret.Items[0].Key
				}, gomega.Equal("cert")),
			),
		))

	})
}
