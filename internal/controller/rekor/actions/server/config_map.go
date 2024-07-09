package server

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewCAConfigMapAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &configMapAction{}
}

type configMapAction struct {
	action.BaseAction
}

func (i configMapAction) Name() string {
	return "create CA configMap"
}

func (i configMapAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	cm, _ := k8sutils.GetConfigMap(ctx, i.Client, instance.Namespace, "ca-configmap")
	// signingKeySecret: OCP related
	signingKeySecret, _ := k8sutils.GetSecret(i.Client, "openshift-service-ca", "signing-key")
	return c.Reason == constants.Creating || c.Reason == constants.Ready && cm == nil && signingKeySecret != nil
}

func (i configMapAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err     error
		updated bool
	)

	labels := constants.LabelsFor(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca-configmap",
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
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create configMap: %w", err), instance)
	}

	//TLS: Annotate configMap
	configMap.Annotations = map[string]string{"service.beta.openshift.io/inject-cabundle": "true"}
	err = i.Client.Update(ctx, configMap)
	if err != nil {
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not annotate configMap: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "ConfigMap created"})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
