package logserver

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/controller/common/utils/tls"
	"github.com/securesign/operator/internal/images"

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

	labels := labels.For(actions.LogServerComponentName, actions.LogserverDeploymentName, instance.Name)
	insCopy := instance.DeepCopy()

	// TLS
	switch {
	case insCopy.Spec.TLS.CertRef != nil:
		insCopy.Status.TLS = insCopy.Spec.TLS
	case kubernetes.IsOpenShift():
		insCopy.Status.TLS = rhtasv1alpha1.TLS{
			CertRef: &rhtasv1alpha1.SecretKeySelector{
				LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: fmt.Sprintf(actions.LogServerTLSSecret, instance.Name)},
				Key:                  "tls.crt",
			},
			PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
				LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: fmt.Sprintf(actions.LogServerTLSSecret, instance.Name)},
				Key:                  "tls.key",
			},
		}
	default:
		i.Logger.V(1).Info("Communication to trillian-db is insecure")
	}

	caPath, err := tls.CAPath(ctx, i.Client, instance)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("failed to get CA path: %w", err), instance)
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.LogserverDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		trillianUtils.EnsureServerDeployment(insCopy, images.Registry.Get(images.TrillianServer), actions.LogserverDeploymentName, actions.RBACName, labels),
		ensure.ControllerReference[*apps.Deployment](insCopy, i.Client),
		ensure.Labels[*apps.Deployment](maps.Keys(labels), labels),
		deployment.Proxy(),
		deployment.TrustedCA(insCopy.GetTrustedCA(), "wait-for-trillian-db", actions.LogserverDeploymentName),
		ensure.Optional(trillianUtils.UseTLSDb(insCopy), trillianUtils.WithTlsDB(insCopy, caPath, actions.LogserverDeploymentName)),
		ensure.Optional(insCopy.Status.TLS.CertRef != nil, trillianUtils.EnsureTLSServer(insCopy, actions.LogserverDeploymentName)),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Trillian server: %w", err), instance, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Deployment created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
