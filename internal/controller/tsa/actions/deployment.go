package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/images"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/action"
	cutils "github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	"golang.org/x/exp/maps"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	chainVolumeName      = "tsa-cert-chain"
	fileSignerVolumeName = "tsa-file-signer-config"
	tinkSignerVolumeName = "tsa-tink-signer-config"
	ntpConfigVolumeName  = "ntp-config"
	authVolumeName       = "auth"
	secretMountPath      = "/var/run/secrets/tas"
	authMountPath        = secretMountPath + "/auth"
	certChainMountPath   = secretMountPath + "/certificate_chain"
	fileSignerMountPath  = secretMountPath + "/file_signer"
	tinkSignerMountPath  = secretMountPath + "/tink_signer"
	NtpMountPath         = secretMountPath + "/ntp_config"
)

type deployAction struct {
	action.BaseAction
}

func NewDeployAction() action.Action[*rhtasv1alpha1.TimestampAuthority] {
	return &deployAction{}
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.TimestampAuthority) bool {
	c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
	if instance.Spec.Signer.CertificateChain.CertificateChainRef == nil &&
		(instance.Spec.Signer.CertificateChain.RootCA == nil ||
			instance.Spec.Signer.CertificateChain.LeafCA == nil) {
		return false
	}

	return (c.Reason == constants.Ready || c.Reason == constants.Creating)
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.TimestampAuthority) *action.Result {
	var (
		result controllerutil.OperationResult
		err    error
	)

	labels := labels.For(ComponentName, DeploymentName, instance.Name)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      DeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureDeployment(instance, RBACName, labels),
		ensure.ControllerReference[*apps.Deployment](instance, i.Client),
		ensure.Labels[*apps.Deployment](maps.Keys(labels), labels),
		deployment.Proxy(),
		deployment.TrustedCA(instance.GetTrustedCA(), actions.ServerDeploymentName),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create TSA Server: %w", err), instance, metav1.Condition{
			Type:               TSAServerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               TSAServerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Creating,
			Message:            "TSA server deployment created",
			ObservedGeneration: instance.Generation,
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) ensureDeployment(instance *rhtasv1alpha1.TimestampAuthority, sa string, labels map[string]string) func(*apps.Deployment) error {
	return func(dp *apps.Deployment) error {

		appArgs := []string{
			"timestamp-server",
			"serve",
			"--host=0.0.0.0",
			"--port=3000",
			fmt.Sprintf("--log-type=%s", cutils.GetOrDefault(instance.GetAnnotations(), annotations.LogType, string(constants.Prod))),
			fmt.Sprintf("--certificate-chain-path=%s/certificate-chain.pem", certChainMountPath),
			fmt.Sprintf("--disable-ntp-monitoring=%v", !instance.Spec.NTPMonitoring.Enabled),
		}

		spec := &dp.Spec
		spec.Replicas = cutils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, DeploymentName)

		chainVolume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, chainVolumeName)
		if chainVolume.Secret == nil {
			chainVolume.Secret = &core.SecretVolumeSource{}
		}
		chainVolume.Secret.SecretName = instance.Status.Signer.CertificateChain.CertificateChainRef.Name
		chainVolume.Secret.Items = []core.KeyToPath{
			{
				Key:  instance.Status.Signer.CertificateChain.CertificateChainRef.Key,
				Path: "certificate-chain.pem",
			},
		}

		chainVolumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, chainVolumeName)
		chainVolumeMount.MountPath = certChainMountPath
		chainVolumeMount.ReadOnly = true

		if instance.Spec.NTPMonitoring.Enabled {
			if instance.Spec.NTPMonitoring.Config != nil {
				ntpConfigVolume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, ntpConfigVolumeName)
				if ntpConfigVolume.ConfigMap == nil {
					ntpConfigVolume.ConfigMap = &core.ConfigMapVolumeSource{}
				}
				ntpConfigVolume.ConfigMap.Name = instance.Status.NTPMonitoring.Config.NtpConfigRef.Name

				ntpConfigVolumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, ntpConfigVolumeName)
				ntpConfigVolumeMount.ReadOnly = true
				ntpConfigVolumeMount.MountPath = NtpMountPath

				appArgs = append(appArgs,
					fmt.Sprintf("--ntp-monitoring=%s/ntp-config.yaml", NtpMountPath),
				)
			}
		}

		switch tsaUtils.GetSignerType(&instance.Spec.Signer) {
		case tsaUtils.FileType:
			{

				fileSignerVolume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, fileSignerVolumeName)
				if fileSignerVolume.Secret == nil {
					fileSignerVolume.Secret = &core.SecretVolumeSource{}
				}
				fileSignerVolume.Secret.SecretName = instance.Status.Signer.File.PrivateKeyRef.Name
				fileSignerVolume.Secret.Items = []core.KeyToPath{
					{
						Key:  instance.Status.Signer.File.PrivateKeyRef.Key,
						Path: "private_key.pem",
					},
				}

				fileSignerVolumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, fileSignerVolumeName)
				fileSignerVolumeMount.MountPath = fileSignerMountPath
				fileSignerVolumeMount.ReadOnly = true

				if instance.Status.Signer.File.PasswordRef != nil {
					fileSignerPasswordEnv := kubernetes.FindEnvByNameOrCreate(container, "SIGNER_PASSWORD")
					fileSignerPasswordEnv.ValueFrom = &core.EnvVarSource{
						SecretKeyRef: &core.SecretKeySelector{
							LocalObjectReference: core.LocalObjectReference{
								Name: instance.Status.Signer.File.PasswordRef.Name,
							},
							Key: instance.Status.Signer.File.PasswordRef.Key,
						},
					}
				}

				appArgs = append(appArgs,
					"--timestamp-signer=file",
					fmt.Sprintf("--file-signer-key-path=%s/private_key.pem", fileSignerMountPath),
					"--file-signer-passwd=$(SIGNER_PASSWORD)",
				)
			}
		case tsaUtils.KmsType:
			{

				if instance.Spec.Signer.Kms.Auth != nil {
					for _, env := range instance.Spec.Signer.Kms.Auth.Env {
						e := kubernetes.FindEnvByNameOrCreate(container, env.Name)
						e.ValueFrom = env.ValueFrom
					}

					for _, secret := range instance.Spec.Signer.Kms.Auth.SecretMount {
						volumeName := fmt.Sprintf("%s-%s", authVolumeName, secret.Name)
						v := kubernetes.FindVolumeByNameOrCreate(&template.Spec, volumeName)
						if v.Secret == nil {
							v.Secret = &core.SecretVolumeSource{}
						}
						v.Secret.SecretName = secret.Name

						vm := kubernetes.FindVolumeMountByNameOrCreate(container, volumeName)
						vm.MountPath = authMountPath
						vm.ReadOnly = true
					}
				}

				appArgs = append(appArgs,
					"--timestamp-signer=kms",
					fmt.Sprintf("--kms-key-resource=%s", instance.Spec.Signer.Kms.KeyResource),
				)
			}
		case tsaUtils.TinkType:
			{

				if instance.Spec.Signer.Tink.Auth != nil {
					for _, env := range instance.Spec.Signer.Tink.Auth.Env {
						e := kubernetes.FindEnvByNameOrCreate(container, env.Name)
						e.ValueFrom = env.ValueFrom
					}

					if len(instance.Spec.Signer.Tink.Auth.SecretMount) > 0 {
						for _, secret := range instance.Spec.Signer.Tink.Auth.SecretMount {
							volumeName := fmt.Sprintf("%s-%s", authVolumeName, secret.Name)
							v := kubernetes.FindVolumeByNameOrCreate(&template.Spec, volumeName)
							if v.Secret == nil {
								v.Secret = &core.SecretVolumeSource{}
							}
							v.Secret.SecretName = secret.Name

							vm := kubernetes.FindVolumeMountByNameOrCreate(container, volumeName)
							vm.MountPath = authMountPath
							vm.ReadOnly = true
						}
					}
				}

				tinkSignerVolume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, tinkSignerVolumeName)
				if tinkSignerVolume.Secret == nil {
					tinkSignerVolume.Secret = &core.SecretVolumeSource{}
				}
				tinkSignerVolume.Secret.SecretName = instance.Spec.Signer.Tink.KeysetRef.Name
				tinkSignerVolume.Secret.Items = []core.KeyToPath{
					{
						Key:  instance.Spec.Signer.Tink.KeysetRef.Key,
						Path: "encryptedKeySet",
					},
				}

				tinkSignerVolumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, tinkSignerVolumeName)
				tinkSignerVolumeMount.MountPath = tinkSignerMountPath
				tinkSignerVolumeMount.ReadOnly = true

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

		container.Image = images.Registry.Get(images.TimestampAuthority)
		container.Command = appArgs

		port := kubernetes.FindPortByNameOrCreate(container, "3000-tcp")
		port.ContainerPort = 3000
		port.Protocol = core.ProtocolTCP

		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		if container.LivenessProbe.HTTPGet == nil {
			container.LivenessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.LivenessProbe.HTTPGet.Path = "/ping"
		container.LivenessProbe.HTTPGet.Port = intstr.FromInt32(3000)
		container.LivenessProbe.InitialDelaySeconds = 5

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		if container.ReadinessProbe.HTTPGet == nil {
			container.ReadinessProbe.HTTPGet = &core.HTTPGetAction{}
		}

		container.ReadinessProbe.HTTPGet.Path = "/ping"
		container.ReadinessProbe.HTTPGet.Port = intstr.FromInt32(3000)
		container.ReadinessProbe.InitialDelaySeconds = 5

		return nil
	}
}
