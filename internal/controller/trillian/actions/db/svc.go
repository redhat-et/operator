package db

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/utils"

	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewCreateServiceAction() action.Action[*rhtasv1alpha1.Trillian] {
	return &createServiceAction{}
}

type createServiceAction struct {
	action.BaseAction
}

func (i createServiceAction) Name() string {
	return "create service"
}

func (i createServiceAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && utils.OptionalBool(instance.Spec.Db.Create)
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {

	var (
		err     error
		updated bool
	)

	labels := constants.LabelsFor(actions.DbComponentName, actions.DbDeploymentName, instance.Name)
	mysql := k8sutils.CreateService(instance.Namespace, host, host, port, port, labels)

	//TLS: Annotate service
	if k8sutils.IsOpenShift() && instance.Spec.Db.TLS.CertRef == nil {
		if mysql.Annotations == nil {
			mysql.Annotations = make(map[string]string)
		}
		mysql.Annotations["service.beta.openshift.io/serving-cert-secret-name"] = instance.Name + "-trillian-db-tls"
	}

	if err = controllerutil.SetControllerReference(instance, mysql, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for DB service: %w", err))
	}

	if updated, err = i.Ensure(ctx, mysql, action.EnsureSpec(), action.EnsureAnnotations("service.beta.openshift.io/serving-cert-secret-name")); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.DbCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Trillian DB service: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.DbCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Service created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
