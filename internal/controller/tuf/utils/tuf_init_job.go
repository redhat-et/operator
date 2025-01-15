package utils

import (
	"path/filepath"

	"github.com/securesign/operator/internal/images"

	"github.com/operator-framework/operator-lib/proxy"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	constants2 "github.com/securesign/operator/internal/controller/tuf/constants"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

const (
	secretsMonthPath = "/var/run/tuf-secrets"
	targetMonthPath  = "/var/run/target"
)

func CreateTufInitJob(instance *rhtasv1alpha1.Tuf, sa string, labels map[string]string) func(*batchv1.Job) error {
	return func(job *batchv1.Job) error {

		// prepare env vars
		env := []v1.EnvVar{
			{
				Name:  "NAMESPACE",
				Value: instance.Namespace,
			},
		}
		env = append(env, proxy.ReadProxyVarsFromEnv()...)

		// prepare args
		args := []string{"--export-keys", instance.Spec.RootKeySecretRef.Name}
		for _, key := range instance.Spec.Keys {
			switch key.Name {
			case "rekor.pub":
				args = append(args, "--rekor-key", filepath.Join(secretsMonthPath, key.Name))
			case "ctfe.pub":
				args = append(args, "--ctlog-key", filepath.Join(secretsMonthPath, key.Name))
			case "fulcio_v1.crt.pem":
				args = append(args, "--fulcio-cert", filepath.Join(secretsMonthPath, key.Name))
			case "tsa.certchain.pem":
				args = append(args, "--tsa-cert", filepath.Join(secretsMonthPath, key.Name))
			}
		}
		args = append(args, targetMonthPath)

		jobSpec := &job.Spec
		jobSpec.Parallelism = ptr.To[int32](1)
		jobSpec.Completions = ptr.To[int32](1)
		jobSpec.BackoffLimit = ptr.To(int32(0))
		jobSpec.Template.Labels = labels

		templateSpec := &jobSpec.Template.Spec
		templateSpec.ServiceAccountName = sa
		templateSpec.RestartPolicy = v1.RestartPolicyNever

		// initialize volumes
		secretsVolume := kubernetes.FindVolumeByName(templateSpec, "tuf-secrets")
		if secretsVolume == nil {
			templateSpec.Volumes = append(templateSpec.Volumes, v1.Volume{Name: "tuf-secrets"})
			secretsVolume = &templateSpec.Volumes[len(templateSpec.Volumes)-1]
		}
		secretsVolume.VolumeSource = v1.VolumeSource{
			Projected: secretsVolumeProjection(instance.Status.Keys),
		}

		repositoryVolume := kubernetes.FindVolumeByName(templateSpec, constants2.VolumeName)
		if repositoryVolume == nil {
			templateSpec.Volumes = append(templateSpec.Volumes, v1.Volume{Name: constants2.VolumeName})
			repositoryVolume = &templateSpec.Volumes[len(templateSpec.Volumes)-1]
		}
		repositoryVolume.VolumeSource = v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: instance.Status.PvcName,
			},
		}
		// init containers
		container := kubernetes.FindContainerByName(templateSpec, "tuf-init")
		if container == nil {
			templateSpec.Containers = append(templateSpec.Containers, v1.Container{Name: "tuf-init"})
			container = &templateSpec.Containers[len(templateSpec.Containers)-1]
		}
		container.Image = images.Registry.Get(images.Tuf)
		container.Env = env
		container.Args = args
		container.VolumeMounts = []v1.VolumeMount{
			{
				Name:      "tuf-secrets",
				MountPath: secretsMonthPath,
			},
			{
				Name:      "repository",
				MountPath: targetMonthPath,
				ReadOnly:  false,
			},
		}

		return nil
	}
}
