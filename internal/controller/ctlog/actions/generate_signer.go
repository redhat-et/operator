package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/securesign/operator/internal/controller/ctlog/utils"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const KeySecretNameFormat = "ctlog-%s-keys-"

var (
	ErrGenerateSignerKey = errors.New("failed to generate signer key")
	ErrParseSignerKey    = errors.New("failed to parse signer key")
)

func NewGenerateSignerAction() action.Action[*v1alpha1.CTlog] {
	return &generateSigner{}
}

type generateSigner struct {
	action.BaseAction
}

func (g generateSigner) Name() string {
	return "generate-signer"
}

func (g generateSigner) CanHandle(_ context.Context, instance *v1alpha1.CTlog) bool {

	if instance.Status.PrivateKeyRef == nil {
		return true
	}

	if !equality.Semantic.DeepDerivative(instance.Spec.PrivateKeyRef, instance.Status.PrivateKeyRef) {
		return true
	}

	if !equality.Semantic.DeepDerivative(instance.Spec.PrivateKeyPasswordRef, instance.Status.PrivateKeyPasswordRef) {
		return true
	}

	return !meta.IsStatusConditionTrue(instance.Status.Conditions, SignerCondition)
}

func (g generateSigner) Handle(ctx context.Context, instance *v1alpha1.CTlog) *action.Result {
	// Force to change SignerCondition when spec has changed
	if meta.IsStatusConditionTrue(instance.Status.Conditions, SignerCondition) {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    SignerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Pending,
			Message: "resolving signer key",
		})
		return g.StatusUpdate(ctx, instance)
	}

	if instance.Spec.PrivateKeyRef != nil {
		instance.Status.PrivateKeyRef = instance.Spec.PrivateKeyRef
		if instance.Spec.PrivateKeyPasswordRef != nil {
			instance.Status.PrivateKeyPasswordRef = instance.Spec.PrivateKeyPasswordRef
		}

		g.Recorder.Eventf(instance, v1.EventTypeNormal, "SignerKeyCreated", "Using signer key from `%s` secret", instance.Spec.PrivateKeyRef.Name)
	} else {

		var (
			err error
		)

		config, err := utils.NewSignerConfig(utils.WithGeneratedKey())
		if err != nil {
			return g.Failed(fmt.Errorf("%w: %w", ErrGenerateSignerKey, err))
		}

		labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)

		privateKey, err := config.PrivateKeyPEM()
		if err != nil {
			return g.Failed(fmt.Errorf("%w, %w", ErrParseSignerKey, err))
		}
		password := config.PrivateKeyPassword()

		data := map[string][]byte{
			"private":  privateKey,
			"password": password,
		}

		secret := k8sutils.CreateImmutableSecret(fmt.Sprintf(KeySecretNameFormat, instance.Name), instance.Namespace,
			data, labels)
		if _, err = g.Ensure(ctx, secret); err != nil {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    SignerCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
			return g.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create secret: %w", err), instance)
		}
		g.Recorder.Eventf(instance, v1.EventTypeNormal, "SignerKeyCreated", "Signer private key created: %s", secret.Name)

		instance.Status.PrivateKeyRef = &v1alpha1.SecretKeySelector{
			Key: "private",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: secret.Name,
			},
		}

		if len(secret.Data["password"]) > 0 && instance.Spec.PrivateKeyPasswordRef == nil {
			instance.Status.PrivateKeyPasswordRef = &v1alpha1.SecretKeySelector{
				Key: "password",
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: secret.Name,
				},
			}
		} else {
			instance.Status.PrivateKeyPasswordRef = instance.Spec.PrivateKeyPasswordRef
		}
	}

	// invalidate server config
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    ServerConfigCondition,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Pending,
		Message: "signer key changed",
	})
	// invalidate public key
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    PublicKeyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Pending,
		Message: "signer key changed",
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   SignerCondition,
		Status: metav1.ConditionTrue,
		Reason: constants.Ready,
	})
	return g.StatusUpdate(ctx, instance)
}
