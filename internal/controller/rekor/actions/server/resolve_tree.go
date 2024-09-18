package server

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/trillian"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func NewResolveTreeAction(opts ...func(*resolveTreeAction)) action.Action[*rhtasv1alpha1.Rekor] {
	a := &resolveTreeAction{
		timeout: time.Duration(constants.CreateTreeDeadline) * time.Second,
	}

	for _, opt := range opts {
		opt(a)
	}
	return a
}

type resolveTreeAction struct {
	action.BaseAction
	timeout time.Duration
}

func (i resolveTreeAction) Name() string {
	return "resolve treeID"
}

func (i resolveTreeAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	switch {
	case c == nil:
		return false
	case c.Reason != constants.Creating && c.Reason != constants.Ready:
		return false
	case instance.Status.TreeID == nil:
		return true
	case instance.Spec.TreeID != nil:
		return !equality.Semantic.DeepEqual(instance.Spec.TreeID, instance.Status.TreeID)
	default:
		return false
	}
}

func (i resolveTreeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	if instance.Spec.TreeID != nil && *instance.Spec.TreeID != int64(0) {
		instance.Status.TreeID = instance.Spec.TreeID
		return i.StatusUpdate(ctx, instance)
	}
	var err error
	var tree *trillian.Tree

	cm := &v1.ConfigMap{}
	deadline := time.Now().Add(i.timeout)
	for time.Now().Before(deadline) {
		err = i.Client.Get(ctx, types.NamespacedName{Name: "rekor-tree-id-config", Namespace: instance.Namespace}, cm)
		if err == nil && cm.Data != nil {
			break
		}
		if err != nil {
			i.Logger.V(1).Error(fmt.Errorf("waiting for the ConfigMap"), err.Error())
		}
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		i.Logger.V(1).Error(fmt.Errorf("timed out waiting for the ConfigMap"), err.Error())
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: fmt.Sprintf("timed out waiting for the ConfigMap: %v", err),
		})
		return i.Failed(fmt.Errorf("timed out waiting for the ConfigMap: %s", "configmap not found"))
	}

	if cm.Data == nil {
		err = fmt.Errorf("ConfigMap data is empty")
		i.Logger.V(1).Error(err, err.Error())
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.Failed(err)
	}

	treeId, exists := cm.Data["tree_id"]
	treeIdInt, err := strconv.ParseInt(treeId, 10, 64)
	tree = &trillian.Tree{TreeId: treeIdInt}
	if !exists {
		i.Logger.V(1).Error(err, err.Error())
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.Failed(err)
	}

	if !exists {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create trillian tree: %v", err), instance)
	}
	i.Recorder.Eventf(instance, v1.EventTypeNormal, "TrillianTreeCreated", "New Trillian tree created: %d", tree.TreeId)
	instance.Status.TreeID = &tree.TreeId

	return i.StatusUpdate(ctx, instance)
}
