package actions

import (
	"context"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewCreateMonitorAction() action.Action[rhtasv1alpha1.Fulcio] {
	return &monitoringAction{}
}

type monitoringAction struct {
	action.BaseAction
}

func (i monitoringAction) Name() string {
	return "create monitoring"
}

func (i monitoringAction) CanHandle(instance *rhtasv1alpha1.Fulcio) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhaseCreating && instance.Spec.Monitoring.Enabled
}

func (i monitoringAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	var (
		err error
	)

	monitoringLabels := constants.LabelsFor(ComponentName, MonitoringRoleName, instance.Name)

	role := kubernetes.CreateRole(
		instance.Namespace,
		MonitoringRoleName,
		monitoringLabels,
		[]v1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"services", "endpoints", "pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	)

	if err = controllerutil.SetControllerReference(instance, role, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for role: %w", err))
	}

	if _, err = i.Ensure(ctx, role); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    string(rhtasv1alpha1.PhaseReady),
			Status:  metav1.ConditionFalse,
			Reason:  "Failure",
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create monitoring role: %w", err), instance)
	}

	roleBinding := kubernetes.CreateRoleBinding(
		instance.Namespace,
		MonitoringRoleName,
		monitoringLabels,
		v1.RoleRef{
			APIGroup: v1.SchemeGroupVersion.Group,
			Kind:     "Role",
			Name:     MonitoringRoleName,
		},
		[]v1.Subject{
			{Kind: "ServiceAccount", Name: "prometheus-k8s", Namespace: "openshift-monitoring"},
		},
	)
	if err = controllerutil.SetControllerReference(instance, roleBinding, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for role: %w", err))
	}

	if _, err = i.Ensure(ctx, roleBinding); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    string(rhtasv1alpha1.PhaseReady),
			Status:  metav1.ConditionFalse,
			Reason:  "Failure",
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create monitoring RoleBinding: %w", err), instance)
	}

	serviceMonitor := kubernetes.CreateServiceMonitor(
		instance.Namespace,
		DeploymentName,
		monitoringLabels,
		[]monitoringv1.Endpoint{
			{
				Interval: monitoringv1.Duration("30s"),
				Port:     "fulcio-server",
				Scheme:   "http",
			},
		},
		constants.LabelsForComponent(ComponentName, instance.Name),
	)

	if err = controllerutil.SetControllerReference(instance, serviceMonitor, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for serviceMonitor: %w", err))
	}

	if _, err = i.Ensure(ctx, serviceMonitor); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    string(rhtasv1alpha1.PhaseReady),
			Status:  metav1.ConditionFalse,
			Reason:  "Failure",
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create serviceMonitor: %w", err), instance)
	}

	// monitors & RBAC are not watched - do not need to re-enqueue
	return i.Continue()
}
