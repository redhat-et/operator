package actions

import (
	"context"
	"slices"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/fulcio/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewHandleFulcioCertAction() action.Action[*v1alpha1.CTlog] {
	return &handleFulcioCert{}
}

type handleFulcioCert struct {
	action.BaseAction
}

func (g handleFulcioCert) Name() string {
	return "handle-fulcio-cert"
}

func (g handleFulcioCert) CanHandle(ctx context.Context, instance *v1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
	switch {
	case c == nil:
		return false
	case c.Reason != constants.Creating && c.Reason != constants.Ready:
		return false
	case len(instance.Status.RootCertificates) == 0:
		return true
	case len(instance.Spec.RootCertificates) == 0:
		// autodiscovery
		if scr, _ := k8sutils.FindSecret(ctx, g.Client, instance.Namespace, actions.FulcioCALabel); scr != nil {
			return !slices.Contains(instance.Status.RootCertificates, v1alpha1.SecretKeySelector{
				LocalObjectReference: v1alpha1.LocalObjectReference{Name: scr.Name},
				Key:                  scr.Labels[actions.FulcioCALabel],
			})
		} else {
			return true
		}
	default:
		return !equality.Semantic.DeepDerivative(instance.Spec.RootCertificates, instance.Status.RootCertificates)
	}
}

func (g handleFulcioCert) Handle(ctx context.Context, instance *v1alpha1.CTlog) *action.Result {
	if meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Reason != constants.Creating {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Creating,
			ObservedGeneration: instance.Generation,
		},
		)
		return g.StatusUpdate(ctx, instance)
	}

	if len(instance.Spec.RootCertificates) == 0 {
		scr, err := k8sutils.FindSecret(ctx, g.Client, instance.Namespace, actions.FulcioCALabel)
		if err != nil {
			if !k8sErrors.IsNotFound(err) {
				return g.Error(ctx, err, instance)
			}

			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    CertCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: "Cert not found",
			})
			g.StatusUpdate(ctx, instance)
			return g.Requeue()
		}
		sks := v1alpha1.SecretKeySelector{
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: scr.Name,
			},
			Key: scr.Labels[actions.FulcioCALabel],
		}
		if slices.Contains(instance.Status.RootCertificates, sks) {
			return g.Continue()
		}
		g.Recorder.Event(instance, v1.EventTypeNormal, "FulcioCertDiscovered", "Fulcio certificate detected")
		instance.Status.RootCertificates = append(instance.Status.RootCertificates, sks)
	} else {
		instance.Status.RootCertificates = instance.Spec.RootCertificates
	}

	// invalidate server config
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    ConfigCondition,
		Status:  metav1.ConditionFalse,
		Reason:  FulcioReason,
		Message: "Fulcio certificate changed",
	})

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   CertCondition,
		Status: metav1.ConditionTrue,
		Reason: "Resolved",
	},
	)
	return g.StatusUpdate(ctx, instance)
}
