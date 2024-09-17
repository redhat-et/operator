package actions

import (
	"context"
	"errors"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	ctlogAction "github.com/securesign/operator/internal/controller/ctlog/constants"
	futils "github.com/securesign/operator/internal/controller/fulcio/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Fulcio] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, tuf *rhtasv1alpha1.Fulcio) bool {
	c := meta.FindStatusCondition(tuf.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	var (
		updated bool
		err     error
	)

	instanceCopy := instance.DeepCopy()
	if err = resolveCtlAddress(ctx, i.Client, instanceCopy); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Resolving CTLog address",
		})
		i.StatusUpdate(ctx, instance)
		return i.Requeue()
	}

	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)

	dp, err := futils.CreateDeployment(instanceCopy, DeploymentName, RBACName, labels)
	if err != nil {
		if err != nil {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    constants.Ready,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
			return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could create server Deployment: %w", err), instance)
		}
	}

	if err = controllerutil.SetControllerReference(instance, dp, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Deployment: %w", err))
	}

	if updated, err = i.Ensure(ctx, dp); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Fulcio: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "Deployment created"})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func resolveCtlAddress(ctx context.Context, cli client.Client, instance *rhtasv1alpha1.Fulcio) error {
	if instance.Spec.Ctlog.Prefix == "" {
		return futils.CtlogPrefixNotSpecified
	}

	if instance.Spec.Ctlog.Address != "" {
		if instance.Spec.Ctlog.Port == nil {
			return futils.CtlogPortNotSpecified
		}
		return nil
	}

	svc, err := kubernetes.FindService(ctx, cli, instance.Namespace, constants.LabelsForComponent(ctlogAction.ComponentName, instance.Name))
	if err != nil {
		return err
	}

	for _, port := range svc.Spec.Ports {
		if port.Name == ctlogAction.ServerPortName {
			var protocol string
			portCopy := port
			instance.Spec.Ctlog.Port = &portCopy.Port
			switch port.Port {
			case 443:
				protocol = "https://"
			case 80:
				protocol = "http://"
			}
			instance.Spec.Ctlog.Address = fmt.Sprintf("%s%s.%s.svc", protocol, svc.Name, svc.Namespace)
			return nil
		}
	}
	return errors.New("protocol name not found")
}
