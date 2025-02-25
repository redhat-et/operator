package db

import (
	"context"
	"errors"

	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/labels"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	commonUtils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewInitializeAction() action.Action[*rhtasv1alpha1.Trillian] {
	return &initializeAction{}
}

type initializeAction struct {
	action.BaseAction
}

func (i initializeAction) Name() string {
	return "db initialize"
}

func (i initializeAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Trillian) bool {
	return !meta.IsStatusConditionTrue(instance.Status.Conditions, actions.DbCondition) &&
		utils.OptionalBool(instance.Spec.Db.Create)
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	labels := labels.ForComponent(actions.DbComponentName, instance.Name)
	ok, err := commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, labels)
	switch {
	case errors.Is(err, commonUtils.ErrDeploymentNotReady):
		i.Logger.Error(err, "deployment is not ready")
	case err != nil:
		return i.Error(ctx, err, instance)
	}
	if !ok {
		i.Logger.Info("Waiting for deployment")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.DbCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Initialize,
			Message: "Waiting for deployment to be ready",
		})
		return i.StatusUpdate(ctx, instance)
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: actions.DbCondition,
		Status: metav1.ConditionTrue, Reason: constants.Ready})
	return i.StatusUpdate(ctx, instance)
}
