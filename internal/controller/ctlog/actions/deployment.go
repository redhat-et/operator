package actions

import (
	"context"
	"fmt"
	"strconv"

	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/controller/common/utils/tls"
	"github.com/securesign/operator/internal/images"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	cutils "github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/controller/labels"
	trillian "github.com/securesign/operator/internal/controller/trillian/actions"
	"golang.org/x/exp/maps"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	volumeName    = "keys"
	containerName = "ctlog"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var (
		result controllerutil.OperationResult
		err    error
	)

	labels := labels.For(ComponentName, DeploymentName, instance.Name)

	switch {
	case instance.Spec.Trillian.Address == "":
		instance.Spec.Trillian.Address = fmt.Sprintf("%s.%s.svc", trillian.LogserverDeploymentName, instance.Namespace)
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      DeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureDeployment(instance, RBACName, labels),
		ensure.ControllerReference[*v1.Deployment](instance, i.Client),
		ensure.Labels[*v1.Deployment](maps.Keys(labels), labels),
		deployment.Proxy(),
		deployment.TrustedCA(instance.GetTrustedCA(), "server"),
		ensure.Optional(tls.UseTlsClient(instance), i.ensureTlsTrillian(ctx, instance)),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create ctlog server deployment: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Creating,
			Message:            "deployment created",
			ObservedGeneration: instance.Generation,
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) ensureDeployment(instance *rhtasv1alpha1.CTlog, sa string, labels map[string]string) func(deployment *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		switch {
		case instance.Status.ServerConfigRef == nil:
			return fmt.Errorf("CreateCTLogDeployment: %w", utils.ServerConfigNotSpecified)
		case instance.Status.TreeID == nil:
			return fmt.Errorf("CreateCTLogDeployment: %w", utils.TreeNotSpecified)
		case instance.Spec.Trillian.Address == "":
			return fmt.Errorf("CreateCTLogDeployment: %w", utils.TrillianAddressNotSpecified)
		case instance.Spec.Trillian.Port == nil:
			return fmt.Errorf("CreateCTLogDeployment: %w", utils.TrillianPortNotSpecified)
		}

		spec := &dp.Spec
		spec.Replicas = cutils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		volume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, volumeName)
		if volume.Secret == nil {
			volume.Secret = &core.SecretVolumeSource{}
		}
		volume.Secret.SecretName = instance.Status.ServerConfigRef.Name

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, containerName)
		container.Image = images.Registry.Get(images.CTLog)

		serverPort := kubernetes.FindPortByNameOrCreate(container, "server")
		serverPort.ContainerPort = ServerTargetPort
		serverPort.Protocol = core.ProtocolTCP

		appArgs := []string{
			"--http_endpoint=0.0.0.0:" + strconv.Itoa(ServerTargetPort),
			"--log_config=/ctfe-keys/config",
			"--alsologtostderr",
		}

		if instance.Spec.Monitoring.Enabled {
			appArgs = append(appArgs, "--metrics_endpoint=0.0.0.0:"+strconv.Itoa(MetricsPort))
			metricsPort := kubernetes.FindPortByNameOrCreate(container, "metrics")
			metricsPort.ContainerPort = MetricsPort
			metricsPort.Protocol = core.ProtocolTCP
		}

		container.Args = appArgs

		volumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, volumeName)
		volumeMount.MountPath = "/ctfe-keys"
		volumeMount.ReadOnly = true

		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		if container.LivenessProbe.HTTPGet == nil {
			container.LivenessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.LivenessProbe.HTTPGet.Path = "/healthz"
		container.LivenessProbe.HTTPGet.Port = intstr.FromInt32(ServerTargetPort)
		container.LivenessProbe.InitialDelaySeconds = 10
		container.LivenessProbe.PeriodSeconds = 10

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		if container.ReadinessProbe.HTTPGet == nil {
			container.ReadinessProbe.HTTPGet = &core.HTTPGetAction{}
		}

		container.ReadinessProbe.HTTPGet.Path = "/healthz"
		container.ReadinessProbe.HTTPGet.Port = intstr.FromInt32(ServerTargetPort)

		container.ReadinessProbe.InitialDelaySeconds = 10
		container.ReadinessProbe.PeriodSeconds = 10

		return nil
	}
}

func (i deployAction) ensureTlsTrillian(ctx context.Context, instance *rhtasv1alpha1.CTlog) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		caPath, err := tls.CAPath(ctx, i.Client, instance)
		if err != nil {
			return fmt.Errorf("failed to get CA path: %w", err)
		}

		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, containerName)

		container.Args = append(container.Args, "--trillian_tls_ca_cert_file", caPath)
		return nil
	}
}
