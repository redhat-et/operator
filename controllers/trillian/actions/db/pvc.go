package db

import (
	"context"
	"fmt"

	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/trillian/actions"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewCreatePvcAction() action.Action[rhtasv1alpha1.Trillian] {
	return &createPvcAction{}
}

type createPvcAction struct {
	action.BaseAction
}

func (i createPvcAction) Name() string {
	return "create PVC"
}

func (i createPvcAction) CanHandle(instance *rhtasv1alpha1.Trillian) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhaseCreating && instance.Spec.Db.Create && instance.Spec.Db.PvcName == ""
}

func (i createPvcAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	var err error

	// PVC does not exist, create a new one
	i.Logger.V(1).Info("Creating new PVC")
	i.Recorder.Event(instance, v1.EventTypeNormal, "PersistentVolumeCreated", "New PersistentVolume created")
	pvc := k8sutils.CreatePVC(instance.Namespace, actions2.DbPvcName, "5Gi", constants.LabelsFor(actions2.ComponentName, actions2.DbDeploymentName, instance.Name))
	if err = controllerutil.SetControllerReference(instance, pvc, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for PVC: %w", err))
	}

	if _, err = i.Ensure(ctx, pvc); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create DB PVC: %w", err), instance)
	}

	instance.Spec.Db.PvcName = pvc.Name
	return i.Update(ctx, instance)
}
