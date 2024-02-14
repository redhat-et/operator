package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewToInitializeAction() action.Action[rhtasv1alpha1.CTlog] {
	return &toInitializeAction{}
}

type toInitializeAction struct {
	action.BaseAction
}

func (i toInitializeAction) Name() string {
	return "move to initialization phase"
}

func (i toInitializeAction) CanHandle(instance *rhtasv1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating
}

func (i toInitializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionTrue, Reason: constants.Initialize})

	return i.StatusUpdate(ctx, instance)
}
