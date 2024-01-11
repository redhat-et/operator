package ctlog

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	utils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	ctlogUtils "github.com/securesign/operator/controllers/ctlog/utils"
	fulcioUtils "github.com/securesign/operator/controllers/fulcio/utils"
	trillianUtils "github.com/securesign/operator/controllers/trillian/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	deploymentName = "ctlog"
	ComponentName  = "ctlog"
)

func NewCreateAction() Action {
	return &createAction{}
}

type createAction struct {
	common.BaseAction
}

func (i createAction) Name() string {
	return "create"
}

func (i createAction) CanHandle(ctlog *rhtasv1alpha1.CTlog) bool {
	return ctlog.Status.Phase == rhtasv1alpha1.PhaseCreating
}

func (i createAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) (*rhtasv1alpha1.CTlog, error) {
	var err error
	labels := utils.FilterCommonLabels(instance.Labels)
	labels["app.kubernetes.io/component"] = ComponentName
	labels["app.kubernetes.io/name"] = deploymentName

	fulcio, err := fulcioUtils.FindFulcio(ctx, i.Client, instance.Namespace, utils.FilterCommonLabels(instance.Labels))
	if err != nil || fulcio.Status.Phase != rhtasv1alpha1.PhaseReady {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not find Fulcio: %s", err)
	}
	trillian, err := trillianUtils.FindTrillian(ctx, i.Client, instance.Namespace, utils.FilterCommonLabels(instance.Labels))
	if err != nil || trillian.Status.Phase != rhtasv1alpha1.PhaseReady {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("can't find trillian: %s", err)
	}

	var config, pubKey *corev1.Secret
	if config, pubKey, err = ctlogUtils.CreateCtlogConfig(ctx, instance.Namespace, trillian.Status.Url, trillian.Status.TreeID, fulcio.Status.Url, labels); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create CTLog configuration: %w", err)
	}
	controllerutil.SetControllerReference(instance, config, i.Client.Scheme())
	controllerutil.SetControllerReference(instance, pubKey, i.Client.Scheme())
	if err = i.Client.Create(ctx, config); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create CTLog configuration secret: %w", err)
	}
	if err = i.Client.Create(ctx, pubKey); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create CTLog public key secret: %w", err)
	}

	server := ctlogUtils.CreateDeployment(instance.Namespace, deploymentName, config.Name, labels)
	controllerutil.SetControllerReference(instance, server, i.Client.Scheme())
	if err = i.Client.Create(ctx, server); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create job: %w", err)
	}

	svc := utils.CreateService(instance.Namespace, "ctlog", 6963, labels)
	svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
		Name:       "80-tcp",
		Protocol:   corev1.ProtocolTCP,
		Port:       80,
		TargetPort: intstr.FromInt(6962),
	})
	controllerutil.SetControllerReference(instance, svc, i.Client.Scheme())
	if err = i.Client.Create(ctx, svc); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create service: %w", err)
	}

	instance.Status.Phase = rhtasv1alpha1.PhaseInitialize
	return instance, nil

}
