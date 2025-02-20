package redis

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/images"

	"github.com/securesign/operator/internal/controller/common/action"
	cutils "github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"golang.org/x/exp/maps"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

const storageVolumeName = "storage"

func NewDeployAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)
	labels := labels.For(actions.RedisComponentName, actions.RedisDeploymentName, instance.Name)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.RedisDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureRedisDeployment(actions.RBACName, labels),
		ensure.ControllerReference[*v1.Deployment](instance, i.Client),
		ensure.Labels[*v1.Deployment](maps.Keys(labels), labels),
		deployment.Proxy(),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s deployment: %w", actions.RedisDeploymentName, err), instance,
			metav1.Condition{
				Type:    actions.RedisCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			},
		)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.RedisCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Redis created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}

}

func (i deployAction) ensureRedisDeployment(sa string, labels map[string]string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {

		spec := &dp.Spec
		spec.Replicas = cutils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.RedisDeploymentName)
		container.Image = images.Registry.Get(images.RekorRedis)
		port := kubernetes.FindPortByNameOrCreate(container, "redis")
		port.Protocol = core.ProtocolTCP
		port.ContainerPort = 6379

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		if container.ReadinessProbe.Exec == nil {
			container.ReadinessProbe.Exec = &core.ExecAction{}
		}

		container.ReadinessProbe.Exec.Command = []string{
			"/bin/sh",
			"-c",
			"-i",
			"test $(redis-cli -h 127.0.0.1 ping) = 'PONG'",
		}
		container.ReadinessProbe.InitialDelaySeconds = 5

		volumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, storageVolumeName)
		volumeMount.MountPath = "/data"

		volume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, storageVolumeName)
		if volume.EmptyDir == nil {
			volume.EmptyDir = &core.EmptyDirVolumeSource{}
		}
		return nil
	}
}
