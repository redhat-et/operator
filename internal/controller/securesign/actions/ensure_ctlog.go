package actions

import (
	"context"

	"github.com/securesign/operator/internal/controller/annotations"
	ctlogConstants "github.com/securesign/operator/internal/controller/ctlog/constants"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewCtlogAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &ctlogAction{}
}

type ctlogAction struct {
	action.BaseAction
}

func (i ctlogAction) Name() string {
	return "create ctlog"
}

func (i ctlogAction) CanHandle(context.Context, *rhtasv1alpha1.Securesign) bool {
	return true
}

func (i ctlogAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err     error
		updated bool
	)
	ctlog := &rhtasv1alpha1.CTlog{}

	ctlog.Name = instance.Name
	ctlog.Namespace = instance.Namespace
	ctlog.Labels = constants.LabelsFor(ctlogConstants.ComponentName, ctlog.Name, instance.Name)
	ctlog.Annotations = annotations.FilterInheritable(instance.Annotations)

	ctlog.Spec = instance.Spec.Ctlog

	if err = controllerutil.SetControllerReference(instance, ctlog, i.Client.Scheme()); err != nil {
		return i.Failed(err)
	}

	if updated, err = i.Ensure(ctx, ctlog); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    CTlogCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    CTlogCondition,
			Status:  v1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "CTLog resource updated " + ctlog.Name,
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.CopyStatus(ctx, client.ObjectKeyFromObject(ctlog), instance)
}

func (i ctlogAction) CopyStatus(ctx context.Context, ok client.ObjectKey, instance *rhtasv1alpha1.Securesign) *action.Result {
	ctl := &rhtasv1alpha1.CTlog{}
	if err := i.Client.Get(ctx, ok, ctl); err != nil {
		return i.Failed(err)
	}
	objectStatus := meta.FindStatusCondition(ctl.Status.Conditions, constants.Ready)
	if objectStatus == nil {
		// not initialized yet, wait for update
		return i.Continue()
	}
	if !meta.IsStatusConditionPresentAndEqual(instance.Status.Conditions, CTlogCondition, objectStatus.Status) {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   CTlogCondition,
			Status: objectStatus.Status,
			Reason: objectStatus.Reason,
		})
		return i.StatusUpdate(ctx, instance)
	}
	return i.Continue()
}
