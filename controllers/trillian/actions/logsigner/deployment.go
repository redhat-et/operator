package logsigner

import (
	"context"
	"fmt"

	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/trillian/actions"
	trillianUtils "github.com/securesign/operator/controllers/trillian/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewDeployAction() action.Action[rhtasv1alpha1.Trillian] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	var (
		err     error
		updated bool
	)

	labels := constants.LabelsFor(actions.LogSignerComponentName, actions.LogsignerDeploymentName, instance.Name)
	signer := trillianUtils.CreateTrillDeployment(instance.Namespace, constants.TrillianLogSignerImage,
		actions.LogsignerDeploymentName,
		actions.RBACName,
		*instance.Spec.Db.DatabaseSecretRef,
		labels)
	signer.Spec.Template.Spec.Containers[0].Args = append(signer.Spec.Template.Spec.Containers[0].Args, "--force_master=true")

	if err = controllerutil.SetControllerReference(instance, signer, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for LogSigner deployment: %w", err))
	}

	if updated, err = i.Ensure(ctx, signer); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.SignerCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Trillian LogSigner deployment: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.SignerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Deployment created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
