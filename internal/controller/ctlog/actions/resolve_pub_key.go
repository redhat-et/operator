package actions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"

	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/securesign/operator/internal/controller/annotations"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const PublicKeySecretNameFormat = "ctlog-%s-pub-"

func NewResolvePubKeyAction() action.Action[*v1alpha1.CTlog] {
	return &resolvePubKeyAction{}
}

type resolvePubKeyAction struct {
	action.BaseAction
}

func (g resolvePubKeyAction) Name() string {
	return "resolve public key"
}

func (g resolvePubKeyAction) CanHandle(ctx context.Context, instance *v1alpha1.CTlog) bool {
	if instance.Status.PublicKeyRef == nil {
		return true
	}

	if instance.Spec.PublicKeyRef != nil && !equality.Semantic.DeepDerivative(instance.Spec.PublicKeyRef, instance.Status.PublicKeyRef) {
		return true
	}

	return !meta.IsStatusConditionTrue(instance.Status.Conditions, PublicKeyCondition)
}

func (g resolvePubKeyAction) Handle(ctx context.Context, instance *v1alpha1.CTlog) *action.Result {
	// Force to change PublicKeyCondition when spec has changed
	if meta.IsStatusConditionTrue(instance.Status.Conditions, PublicKeyCondition) {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    PublicKeyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Pending,
			Message: "resolving public key",
		})
		return g.StatusUpdate(ctx, instance)
	}

	var (
		err           error
		config        *utils.SignerKey
		discoveredSks *v1alpha1.SecretKeySelector
	)

	if instance.Spec.PublicKeyRef != nil {
		scr := &metav1.PartialObjectMetadata{}
		scr.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		})
		err = g.Client.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.PublicKeyRef.Name}, scr)
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:    PublicKeyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  constants.Pending,
					Message: "Waiting for secret " + instance.Spec.PublicKeyRef.Name,
				})
				g.StatusUpdate(ctx, instance)
				return g.Requeue()
			}
			return g.Failed(fmt.Errorf("ResolvePubKey: %w", err))
		}
		// Add ctfe.pub label to secret
		if err = constants.AddLabel(ctx, scr, g.Client, CTLPubLabel, instance.Spec.PublicKeyRef.Key); err != nil {
			return g.Failed(fmt.Errorf("ResolvePubKey: %w", err))
		}
	}

	config, err = utils.ResolveSignerConfig(g.Client, instance)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrResolvePrivateKey):
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    PublicKeyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Pending,
				Message: "Waiting for secret " + instance.Status.PrivateKeyRef.Name,
			})
			g.StatusUpdate(ctx, instance)
			return g.Requeue()
		case errors.Is(err, utils.ErrResolvePrivateKeyPassword):
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    PublicKeyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Pending,
				Message: "Waiting for secret " + instance.Status.PrivateKeyPasswordRef.Name,
			})
			g.StatusUpdate(ctx, instance)
			return g.Requeue()
		default:
			return g.Failed(fmt.Errorf("%w: %w", ErrParseSignerKey, err))
		}
	}

	publicKey, err := config.PublicKeyPEM()
	if err != nil {
		return g.Failed(fmt.Errorf("%w: %w", ErrGenerateSignerKey, err))
	}

	scrl := &metav1.PartialObjectMetadataList{}
	scrl.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	})

	if err = k8sutils.FindByLabelSelector(ctx, g.Client, scrl, instance.Namespace, CTLPubLabel); err != nil {
		return g.Failed(fmt.Errorf("ResolvePubKey: find secrets failed: %w", err))
	}

	// Search if exists a secret with rhtas.redhat.com/ctfe.pub label
	for _, secret := range scrl.Items {
		sks := v1alpha1.SecretKeySelector{
			LocalObjectReference: v1alpha1.LocalObjectReference{Name: secret.Name},
			Key:                  secret.Labels[CTLPubLabel],
		}

		// Compare key from API and from discovered secret
		var sksPublicKey utils.PEM
		sksPublicKey, err = k8sutils.GetSecretData(g.Client, instance.Namespace, &sks)
		if err != nil {
			return g.Failed(fmt.Errorf("ResolvePubKey: failed to read `%s` secret's data: %w", sks.Name, err))
		}

		if bytes.Equal(sksPublicKey, publicKey) {
			discoveredSks = &sks
			continue
		}

		// Remove label from secret
		if err = constants.RemoveLabel(ctx, &secret, g.Client, CTLPubLabel); err != nil {
			return g.Failed(fmt.Errorf("ResolvePubKey: %w", err))
		}

		message := fmt.Sprintf("Removed '%s' label from %s secret", CTLPubLabel, secret.Name)
		g.Recorder.Event(instance, v1.EventTypeNormal, "PublicKeySecretLabelRemoved", message)
		g.Logger.Info(message)
	}

	if discoveredSks == nil {
		// Create new secret with public key
		const keyName = "public"
		labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)
		labels[CTLPubLabel] = keyName

		newConfig := k8sutils.CreateImmutableSecret(
			fmt.Sprintf(PublicKeySecretNameFormat, instance.Name),
			instance.Namespace,
			map[string][]byte{
				keyName: publicKey,
			},
			labels)

		if newConfig.Annotations == nil {
			newConfig.Annotations = make(map[string]string)
		}
		newConfig.Annotations[annotations.TreeId] = strconv.FormatInt(ptr.Deref(instance.Status.TreeID, 0), 10)

		if err = g.Client.Create(ctx, newConfig); err != nil {
			return g.FailedWithStatusUpdate(ctx, err, instance)
		}

		g.Recorder.Eventf(instance, v1.EventTypeNormal, "PublicKeySecretCreated", "New CTlog public key created: %s", newConfig.Name)
		instance.Status.PublicKeyRef = &v1alpha1.SecretKeySelector{
			LocalObjectReference: v1alpha1.LocalObjectReference{Name: newConfig.Name},
			Key:                  keyName,
		}
	} else {
		instance.Status.PublicKeyRef = discoveredSks
		g.Recorder.Eventf(instance, v1.EventTypeNormal, "PublicKeySecretDiscovered", "Existing public key discovered: %s", discoveredSks.Name)
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   PublicKeyCondition,
		Status: metav1.ConditionTrue,
		Reason: constants.Ready,
	})
	return g.StatusUpdate(ctx, instance)
}
