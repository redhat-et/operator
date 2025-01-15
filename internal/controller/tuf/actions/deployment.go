package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/images"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"golang.org/x/exp/maps"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Tuf] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, tuf *rhtasv1alpha1.Tuf) bool {
	c := meta.FindStatusCondition(tuf.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	labels := labels.For(tufConstants.ComponentName, tufConstants.DeploymentName, instance.Name)

	var (
		result controllerutil.OperationResult
		err    error
	)
	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      tufConstants.DeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.createTufDeployment(instance, tufConstants.RBACName, labels),
		ensure.ControllerReference[*v1.Deployment](instance, i.Client),
		ensure.Labels[*v1.Deployment](maps.Keys(labels), labels),
		ensure.Proxy(),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create TUF: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "Deployment created"})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) createTufDeployment(instance *rhtasv1alpha1.Tuf, sa string, labels map[string]string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {

		spec := &dp.Spec
		spec.Replicas = utils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}
		spec.Strategy = v1.DeploymentStrategy{
			Type: "Recreate",
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		volume := kubernetes.FindVolumeByName(&template.Spec, tufConstants.VolumeName)
		if volume == nil {
			template.Spec.Volumes = append(template.Spec.Volumes, core.Volume{Name: tufConstants.VolumeName})
			volume = &template.Spec.Volumes[len(template.Spec.Volumes)-1]
		}
		volume.VolumeSource = core.VolumeSource{
			PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
				ClaimName: instance.Status.PvcName,
			},
		}

		container := kubernetes.FindContainerByName(&template.Spec, tufConstants.ContainerName)
		if container == nil {
			template.Spec.Containers = append(template.Spec.Containers, core.Container{Name: tufConstants.ContainerName})
			container = &template.Spec.Containers[len(template.Spec.Containers)-1]
		}
		container.Image = images.Registry.Get(images.HttpServer)
		container.Ports = []core.ContainerPort{
			{
				Protocol:      core.ProtocolTCP,
				ContainerPort: 8080,
			},
		}

		container.VolumeMounts = []core.VolumeMount{
			{
				Name:      tufConstants.VolumeName,
				MountPath: "/var/www/html",
				// let user upload manual update using `oc rsync` command
				ReadOnly: false,
			}}

		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		// server is running returning any status code (including 403 - noindex.html)
		container.LivenessProbe.Exec = &core.ExecAction{Command: []string{"curl", "localhost:8080"}}
		container.LivenessProbe.InitialDelaySeconds = 30
		container.LivenessProbe.PeriodSeconds = 10

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		container.ReadinessProbe.HTTPGet = &core.HTTPGetAction{
			Path:   "/root.json",
			Port:   intstr.FromInt32(8080),
			Scheme: "HTTP",
		}
		container.ReadinessProbe.InitialDelaySeconds = 10
		container.ReadinessProbe.PeriodSeconds = 10
		container.ReadinessProbe.FailureThreshold = 10

		return nil
	}
}
