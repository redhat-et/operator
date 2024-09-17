package actions

import (
	"context"
	"fmt"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"golang.org/x/exp/maps"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewIngressAction() action.Action[*rhtasv1alpha1.Fulcio] {
	return &ingressAction{}
}

type ingressAction struct {
	action.BaseAction
}

func (i ingressAction) Name() string {
	return "ingress"
}

func (i ingressAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Fulcio) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready &&
		instance.Spec.ExternalAccess.Enabled
}

func (i ingressAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	var updated bool
	ok := types.NamespacedName{Name: DeploymentName, Namespace: instance.Namespace}
	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)

	svc := &v1.Service{}
	if err := i.Client.Get(ctx, ok, svc); err != nil {
		return i.Failed(fmt.Errorf("could not find service for ingress: %w", err))
	}

	ingress, err := kubernetes.CreateIngress(ctx, i.Client, *svc, instance.Spec.ExternalAccess, ServerPortName, labels)
	if err != nil {
		return i.Failed(fmt.Errorf("could not create ingress object: %w", err))
	}

	if err = controllerutil.SetControllerReference(instance, ingress, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Ingress: %w", err))
	}

	labelKeys := maps.Keys(instance.Spec.ExternalAccess.RouteSelectorLabels)
	labelKeys = append(labelKeys, maps.Keys(labels)...)
	if updated, err = i.Ensure(ctx, ingress, action.EnsureSpec(), action.EnsureLabels(labelKeys...)); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "Ingress created"})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
