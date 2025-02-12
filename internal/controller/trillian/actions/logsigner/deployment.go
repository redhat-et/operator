package logsigner

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	trillianUtils "github.com/securesign/operator/internal/controller/trillian/utils"
	"golang.org/x/exp/maps"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Trillian] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	labels := labels.For(actions.LogSignerComponentName, actions.LogsignerDeploymentName, instance.Name)

	caTrustRef := ensure.TrustedCAAnnotationToReference(instance.Annotations)
	// override if spec.trustedCA is defined
	if instance.Spec.TrustedCA != nil {
		caTrustRef = instance.Spec.TrustedCA
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.LogsignerDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		trillianUtils.EnsureServerDeployment(instance, constants.TrillianLogSignerImage, actions.LogsignerDeploymentName, actions.RBACName, labels,
			"--election_system=k8s", "--lock_namespace=$(NAMESPACE)", "--lock_holder_identity=$(POD_NAME)"),
		ensure.ControllerReference[*apps.Deployment](instance, i.Client),
		ensure.Labels[*apps.Deployment](maps.Keys(labels), labels),
		ensure.Proxy(),
		ensure.TrustedCA(caTrustRef),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Trillian LogSigner: %w", err), instance, metav1.Condition{
			Type:    actions.SignerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.SignerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Deployment created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
