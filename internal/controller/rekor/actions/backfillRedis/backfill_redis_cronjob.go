package backfillredis

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/images"

	"github.com/robfig/cron/v3"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewBackfillRedisCronJobAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &backfillRedisCronJob{}
}

type backfillRedisCronJob struct {
	action.BaseAction
}

func (i backfillRedisCronJob) Name() string {
	return "backfill-redis"
}

func (i backfillRedisCronJob) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && utils.OptionalBool(instance.Spec.BackFillRedis.Enabled)
}

func (i backfillRedisCronJob) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err     error
		updated bool
	)

	if _, err := cron.ParseStandard(instance.Spec.BackFillRedis.Schedule); err != nil {
		return i.Failed(fmt.Errorf("could not create backfill redis cron job: %w", err))
	}

	labels := labels.For(actions.BackfillRedisCronJobName, actions.BackfillRedisCronJobName, instance.Name)
	backfillRedisCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      actions.BackfillRedisCronJobName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: instance.Spec.BackFillRedis.Schedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: actions.RBACName,
							RestartPolicy:      "OnFailure",
							Containers: []corev1.Container{
								{
									Name:    actions.BackfillRedisCronJobName,
									Image:   images.Registry.Get(images.BackfillRedis),
									Command: []string{"/bin/sh", "-c"},
									Args: []string{
										fmt.Sprintf(`endIndex=$(curl -sS http://%s/api/v1/log | sed -E 's/.*"treeSize":([0-9]+).*/\1/'); endIndex=$((endIndex-1)); if [ $endIndex -lt 0 ]; then echo "info: no rekor entries found"; exit 0; fi; backfill-redis --redis-hostname=rekor-redis --redis-port=6379 --rekor-address=http://%s --start=0 --end=$endIndex`, actions.ServerComponentName, actions.ServerComponentName),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err = controllerutil.SetControllerReference(instance, backfillRedisCronJob, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for backfill redis cron job: %w", err))
	}

	if updated, err = i.Ensure(ctx, backfillRedisCronJob); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.RedisCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create backfill redis cron job: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Backfill redis job created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
