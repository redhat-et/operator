package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	cutils "github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/job"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	actions2 "github.com/securesign/operator/internal/controller/trillian/actions"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewCreateTreeJobAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &createTreeJobAction{}
}

type createTreeJobAction struct {
	action.BaseAction
}

func (i createTreeJobAction) Name() string {
	return "create tree job"
}

func (i createTreeJobAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.CTlog) bool {
	cm, _ := kubernetes.GetConfigMap(ctx, i.Client, instance.Namespace, CtlogTreeJobConfigMapName)
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && cm == nil && instance.Status.TreeID == nil
}

func (i createTreeJobAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var (
		err     error
		updated bool
	)

	var trillUrl string

	switch {
	case instance.Spec.Trillian.Port == nil:
		err = fmt.Errorf("%s: %v", i.Name(), utils.TrillianPortNotSpecified)
	case instance.Spec.Trillian.Address == "":
		trillUrl = fmt.Sprintf("%s.%s.svc:%d", actions2.LogserverDeploymentName, instance.Namespace, *instance.Spec.Trillian.Port)
	default:
		trillUrl = fmt.Sprintf("%s:%d", instance.Spec.Trillian.Address, *instance.Spec.Trillian.Port)
	}
	if err != nil {
		return i.Failed(err)
	}
	i.Logger.V(1).Info("trillian logserver", "address", trillUrl)

	if c := meta.FindStatusCondition(instance.Status.Conditions, CtlogTreeJobName); c == nil {
		instance.SetCondition(metav1.Condition{
			Type:    CtlogTreeJobName,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Creating tree Job",
		})
	}

	labels := constants.LabelsFor(ComponentName, ComponentName, instance.Name)

	// Needed for configMap clean-up
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CtlogTreeJobConfigMapName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{},
	}
	if err = controllerutil.SetControllerReference(instance, configMap, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for configMap: %w", err))
	}
	if updated, err = i.Ensure(ctx, configMap); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    CtlogTreeJobName,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
	}
	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: CtlogTreeJobName,
			Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "ConfigMap created"})
		i.Recorder.Event(instance, corev1.EventTypeNormal, "ConfigMapCreated", "New ConfigMap created")
	}

	parallelism := int32(1)
	completions := int32(1)
	activeDeadlineSeconds := int64(600)
	backoffLimit := int32(5)

	trustedCAAnnotation := cutils.TrustedCAAnnotationToReference(instance.Annotations)
	caPath, err := utils.CAPath(ctx, i.Client, instance)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    CtlogTreeJobName,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		i.StatusUpdate(ctx, instance)
		if apiErrors.IsNotFound(err) {
			return i.Requeue()
		}
		return i.Failed(err)
	}
	cmd := ""
	switch {
	case trustedCAAnnotation != nil:
		cmd = fmt.Sprintf("/createtree --admin_server=%s --display_name=%s --tls_cert_file=%s", trillUrl, CtlogTreeName, caPath)
	case kubernetes.IsOpenShift():
		cmd = fmt.Sprintf("/createtree --admin_server=%s --display_name=%s --tls_cert_file=/var/run/secrets/tas/tls.crt", trillUrl, CtlogTreeName)
	default:
		cmd = fmt.Sprintf("/createtree --admin_server=%s --display_name=%s", trillUrl, CtlogTreeName)
	}
	command := []string{
		"/bin/sh",
		"-c",
		fmt.Sprintf(`
		TREE_ID=$(%s)
		if [ $? -eq 0 ]; then
			echo "TREE_ID=$TREE_ID"
			TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
			NAMESPACE=$(cat /var/run/secrets/kubernetes.io/serviceaccount/namespace)
			API_SERVER=https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}
			curl -k -X PATCH $API_SERVER/api/v1/namespaces/$NAMESPACE/configmaps/"%s" \
				-H "Authorization: Bearer $TOKEN" \
				-H "Content-Type: application/merge-patch+json" \
				-d '{
					"data": {
						"tree_id": "'$TREE_ID'"
					}
				}'
			if [ $? -ne 0 ]; then
				echo "Failed to update ConfigMap" >&2
				exit 1
			fi
		else
			echo "Failed to create tree" >&2
			exit 1
		fi
		`, cmd, CtlogTreeJobConfigMapName),
	}
	env := []corev1.EnvVar{}

	job := job.CreateJob(instance.Namespace, CtlogTreeJobName, labels, constants.CreateTreeImage, RBACName, parallelism, completions, activeDeadlineSeconds, backoffLimit, command, env)
	if err = ctrl.SetControllerReference(instance, job, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Job: %w", err))
	}

	err = cutils.SetTrustedCA(&job.Spec.Template, trustedCAAnnotation)
	if err != nil {
		return i.Failed(err)
	}

	if kubernetes.IsOpenShift() && trustedCAAnnotation == nil {
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "tls-cert",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: instance.Name + "-trillian-server-tls",
					},
				},
			})
		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "tls-cert",
				MountPath: "/var/run/secrets/tas",
				ReadOnly:  true,
			})
	}

	_, err = i.Ensure(ctx, job)
	if err != nil {
		return i.Failed(fmt.Errorf("failed to Ensure the job: %w", err))
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    CtlogTreeJobName,
		Status:  metav1.ConditionTrue,
		Reason:  constants.Creating,
		Message: "tree Job Created",
	})

	return i.Continue()
}
