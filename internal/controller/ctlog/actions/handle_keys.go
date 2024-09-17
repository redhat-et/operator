package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	constants2 "github.com/securesign/operator/internal/controller/ctlog/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const KeySecretNameFormat = "ctlog-%s-keys-"

func NewHandleKeysAction() action.Action[*v1alpha1.CTlog] {
	return &handleKeys{}
}

type handleKeys struct {
	action.BaseAction
}

func (g handleKeys) Name() string {
	return "handle-keys"
}

func (g handleKeys) CanHandle(ctx context.Context, instance *v1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	if c.Reason != constants.Creating && c.Reason != constants.Ready {
		return false
	}

	return instance.Status.PrivateKeyRef == nil || instance.Status.PublicKeyRef == nil ||
		!equality.Semantic.DeepDerivative(instance.Spec.PrivateKeyRef, instance.Status.PrivateKeyRef) ||
		!equality.Semantic.DeepDerivative(instance.Spec.PublicKeyRef, instance.Status.PublicKeyRef) ||
		!equality.Semantic.DeepDerivative(instance.Spec.PrivateKeyPasswordRef, instance.Status.PrivateKeyPasswordRef)
}

func (g handleKeys) Handle(ctx context.Context, instance *v1alpha1.CTlog) *action.Result {
	if meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Reason != constants.Creating {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   constants.Ready,
			Status: metav1.ConditionFalse,
			Reason: constants.Creating,
		},
		)
		return g.StatusUpdate(ctx, instance)
	}
	var (
		data map[string][]byte
	)

	if instance.Spec.PrivateKeyRef == nil {
		config, err := utils.CreatePrivateKey()
		if err != nil {
			return g.Failed(err)
		}
		data = map[string][]byte{
			"private": config.PrivateKey,
			"public":  config.PublicKey,
		}
	} else {
		var (
			private, password []byte
			err               error
			config            *utils.PrivateKeyConfig
		)

		private, err = k8sutils.GetSecretData(g.Client, instance.Namespace, instance.Spec.PrivateKeyRef)
		if err != nil {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    constants.Ready,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Pending,
				Message: "Waiting for secret " + instance.Spec.PrivateKeyRef.Name,
			})
			g.StatusUpdate(ctx, instance)
			// busy waiting - no watch on provided secrets
			return g.Requeue()
		}
		if instance.Spec.PrivateKeyPasswordRef != nil {
			password, err = k8sutils.GetSecretData(g.Client, instance.Namespace, instance.Spec.PrivateKeyPasswordRef)
			if err != nil {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:    constants.Ready,
					Status:  constants.Creating,
					Reason:  constants.Pending,
					Message: "Waiting for secret " + instance.Spec.PrivateKeyPasswordRef.Name,
				})
				g.StatusUpdate(ctx, instance)
				// busy waiting - no watch on provided secrets
				return g.Requeue()
			}
		}
		config, err = utils.GeneratePublicKey(&utils.PrivateKeyConfig{PrivateKey: private, PrivateKeyPass: password})
		if err != nil || config == nil {
			return g.Failed(fmt.Errorf("unable to generate public key: %w", err))
		}
		data = map[string][]byte{"public": config.PublicKey}
	}

	labels := constants.LabelsFor(constants2.ComponentName, constants2.DeploymentName, instance.Name)
	labels[constants2.CTLPubLabel] = "public"
	secret := k8sutils.CreateImmutableSecret(fmt.Sprintf(KeySecretNameFormat, instance.Name), instance.Namespace,
		data, labels)

	if err := controllerutil.SetControllerReference(instance, secret, g.Client.Scheme()); err != nil {
		return g.Failed(fmt.Errorf("could not set controller reference for Secret: %w", err))
	}

	// ensure that only new key is exposed
	if err := g.Client.DeleteAllOf(ctx, &v1.Secret{}, client.InNamespace(instance.Namespace), client.MatchingLabels(constants.LabelsFor(constants2.ComponentName, constants2.DeploymentName, instance.Name)), client.HasLabels{constants2.CTLPubLabel}); err != nil {
		return g.Failed(err)
	}

	if _, err := g.Ensure(ctx, secret); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return g.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Secret: %w", err), instance)
	}

	if instance.Spec.PrivateKeyRef == nil {
		instance.Status.PrivateKeyRef = &v1alpha1.SecretKeySelector{
			Key: "private",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	} else {
		instance.Status.PrivateKeyRef = instance.Spec.PrivateKeyRef
	}

	if _, ok := data["password"]; instance.Spec.PrivateKeyPasswordRef == nil && ok {
		instance.Status.PrivateKeyPasswordRef = &v1alpha1.SecretKeySelector{
			Key: "password",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	} else {
		instance.Status.PrivateKeyPasswordRef = instance.Spec.PrivateKeyPasswordRef
	}

	if instance.Spec.PublicKeyRef == nil {
		instance.Status.PublicKeyRef = &v1alpha1.SecretKeySelector{
			Key: "public",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	} else {
		instance.Status.PublicKeyRef = instance.Spec.PublicKeyRef
	}

	// invalidate server config
	if instance.Status.ServerConfigRef != nil {
		if err := g.Client.Delete(ctx, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instance.Status.ServerConfigRef.Name,
				Namespace: instance.Namespace,
			},
		}); err != nil {
			if !k8sErrors.IsNotFound(err) {
				return g.Failed(err)
			}
		}
		instance.Status.ServerConfigRef = nil
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    constants.Ready,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Creating,
		Message: "Keys resolved",
	})
	return g.StatusUpdate(ctx, instance)
}
