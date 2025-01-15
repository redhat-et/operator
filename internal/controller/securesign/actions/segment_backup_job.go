package actions

import (
	"fmt"
	"strconv"

	"github.com/securesign/operator/internal/images"

	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/job"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/labels"

	"context"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"

	"github.com/operator-framework/operator-lib/proxy"
)

func NewSegmentBackupJobAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &segmentBackupJob{}
}

type segmentBackupJob struct {
	action.BaseAction
}

func (i segmentBackupJob) Name() string {
	return SegmentBackupJobName
}

func (i segmentBackupJob) CanHandle(_ context.Context, instance *rhtasv1alpha1.Securesign) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, MetricsCondition)
	if c == nil || c.Reason == constants.Ready {
		return false
	}

	val, found := instance.Annotations[annotations.Metrics]
	if !found {
		return true
	}
	if boolVal, err := strconv.ParseBool(val); err == nil {
		return boolVal
	}
	return true
}

func (i segmentBackupJob) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err error
	)

	labels := labels.For(SegmentBackupJobName, SegmentBackupJobName, instance.Name)
	parallelism := int32(1)
	completions := int32(1)
	activeDeadlineSeconds := int64(600)
	backoffLimit := int32(5)
	command := []string{"python3", "/opt/app-root/src/src/script.py"}
	env := []corev1.EnvVar{
		{
			Name:  "RUN_TYPE",
			Value: "installation",
		},
		{
			Name:  "REQUESTS_CA_BUNDLE",
			Value: "/etc/pki/tls/certs/ca-bundle.crt", // Certificate used to verify requests externally i.e communication with segment
		},
		{
			Name:  "REQUESTS_CA_BUNDLE_INTERNAL",
			Value: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", // Certificate used to verify requests internally i.e queries to thanos
		},
	}

	// Adding proxy variables to operand
	env = append(env, proxy.ReadProxyVarsFromEnv()...)

	// Logic to delete old SBJ to avoid SECURESIGN-1207, can be removed after next release
	if sbj, err := job.GetJob(ctx, i.Client, instance.Namespace, SegmentBackupJobName); sbj != nil {
		if err = i.Client.Delete(ctx, sbj); err != nil {
			i.Logger.Error(err, "problem with removing SBJ resources", "namespace", instance.Namespace, "name", SegmentBackupJobName)
		}
	} else if client.IgnoreNotFound(err) != nil {
		i.Logger.Error(err, "unable to retrieve SBJ resource", "namespace", instance.Namespace, "name", SegmentBackupJobName)
	}

	job := job.CreateJob(instance.Namespace, SegmentBackupJobName, labels, images.Registry.Get(images.SegmentBackup), SegmentRBACName, parallelism, completions, activeDeadlineSeconds, backoffLimit, command, env)
	if err = ctrl.SetControllerReference(instance, job, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Job: %w", err))
	}
	_, err = i.Ensure(ctx, job)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    MetricsCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: err.Error(),
		})
		return i.StatusUpdate(ctx, instance)
	}
	return i.Continue()
}
