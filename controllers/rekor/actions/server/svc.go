package server

import (
	"context"
	"fmt"

	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/rekor/actions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewCreateServiceAction() action.Action[rhtasv1alpha1.Rekor] {
	return &createServiceAction{}
}

type createServiceAction struct {
	action.BaseAction
}

func (i createServiceAction) Name() string {
	return "create service"
}

func (i createServiceAction) CanHandle(instance *rhtasv1alpha1.Rekor) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhaseCreating || instance.Status.Phase == rhtasv1alpha1.PhaseReady
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {

	var (
		err     error
		updated bool
	)

	labels := constants.LabelsFor(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)
	svc := k8sutils.CreateService(instance.Namespace, actions.ServerDeploymentName, 2112, labels)
	svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
		Name:       "80-tcp",
		Protocol:   corev1.ProtocolTCP,
		Port:       80,
		TargetPort: intstr.FromInt(3000),
	})

	if err = controllerutil.SetControllerReference(instance, svc, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for service: %w", err))
	}

	if updated, err = i.Ensure(ctx, svc); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if updated {
		return i.Return()
	} else {
		return i.Continue()
	}
}
