package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"golang.org/x/exp/maps"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

const (
	RekorPubLabel       = labels.LabelNamespace + "/rekor.pub"
	pubSecretNameFormat = "rekor-public-%s-"
)

func NewResolvePubKeyAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &resolvePubKeyAction{}
}

type resolvePubKeyAction struct {
	action.BaseAction
}

func (i resolvePubKeyAction) Name() string {
	return "resolve public key"
}

func (i resolvePubKeyAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	return meta.IsStatusConditionTrue(instance.Status.Conditions, actions.ServerCondition) &&
		instance.Status.PublicKeyRef == nil
}

func (i resolvePubKeyAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err            error
		publicKey      []byte
		partialSecrets *metav1.PartialObjectMetadataList
	)

	// Resolve public key from Rekors API
	publicKey, err = i.resolvePubKey(*instance)
	if err != nil {
		err := fmt.Errorf("ResolvePubKey: unable to resolve public key: %v", err)
		return i.Error(ctx, err, instance, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
	}

	if partialSecrets, err = k8sutils.ListSecrets(ctx, i.Client, instance.Namespace, RekorPubLabel); err != nil {
		return i.Error(ctx, fmt.Errorf("ResolvePubKey: find secrets failed: %w", err), instance)
	}

	for _, partialSecret := range partialSecrets.Items {
		sks := &rhtasv1alpha1.SecretKeySelector{Key: partialSecret.Labels[RekorPubLabel], LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: partialSecret.Name}}
		existingPublicKey, err := k8sutils.GetSecretData(i.Client, instance.Namespace, sks)
		if err != nil {
			return i.Error(ctx, fmt.Errorf("ResolvePubKey: failed to read `%s` secret's data: %w", sks.Name, err), instance)
		}
		if bytes.Equal(existingPublicKey, publicKey) && instance.Status.PublicKeyRef == nil {
			instance.Status.PublicKeyRef = sks
			i.Recorder.Eventf(instance, v1.EventTypeNormal, "PublicKeySecretDiscovered", "Existing public key discovered: %s", sks.Name)
		} else {
			if err = labels.Remove(ctx, &partialSecret, i.Client, RekorPubLabel); err != nil {
				return i.Error(ctx, fmt.Errorf("ResolvePubKey: %w", err), instance)
			}
			message := fmt.Sprintf("Removed '%s' label from %s secret", RekorPubLabel, partialSecret.Name)
			i.Recorder.Event(instance, v1.EventTypeNormal, "PublicKeySecretLabelRemoved", message)
			i.Logger.Info(message)
		}
	}
	if instance.Status.PublicKeyRef != nil {
		return i.StatusUpdate(ctx, instance)
	}

	// Create new secret with public key
	const keyName = "public"
	componentLabels := labels.For(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)
	keyLabels := map[string]string{RekorPubLabel: keyName}
	anno := map[string]string{annotations.TreeId: strconv.FormatInt(ptr.Deref(instance.Status.TreeID, 0), 10)}

	newConfig := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf(pubSecretNameFormat, instance.Name),
			Namespace:    instance.Namespace,
		},
	}

	if _, err = k8sutils.CreateOrUpdate(ctx, i.Client,
		newConfig,
		ensure.Labels[*v1.Secret](maps.Keys(componentLabels), componentLabels),
		ensure.Labels[*v1.Secret](maps.Keys(keyLabels), keyLabels),
		ensure.Annotations[*v1.Secret](maps.Keys(anno), anno),
		k8sutils.EnsureSecretData(true, map[string][]byte{
			keyName: publicKey,
		}),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Server config: %w", err), instance,
			metav1.Condition{
				Type:    actions.ServerCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
	}

	i.Recorder.Eventf(instance, v1.EventTypeNormal, "PublicKeySecretCreated", "New Rekor public key created: %s", newConfig.Name)
	c := meta.FindStatusCondition(instance.Status.Conditions, actions.ServerCondition)
	c.Message = "Public key resolved"
	meta.SetStatusCondition(&instance.Status.Conditions, *c)
	return i.StatusUpdate(ctx, instance)
}

func (i resolvePubKeyAction) resolvePubKey(instance rhtasv1alpha1.Rekor) ([]byte, error) {
	var (
		data []byte
		err  error
		url  = fmt.Sprintf("http://%s.%s.svc", actions.ServerDeploymentName, instance.Namespace)
	)

	inContainer, err := k8sutils.ContainerMode()
	if err == nil {
		if !inContainer && instance.Status.Url != "" {
			url = instance.Status.Url
		}
	} else {
		klog.Info("Can't recognise operator mode - expecting in-container run")
	}

	if data, err = i.requestPublicKey(url); err == nil {
		return data, nil
	}
	i.Logger.Info("retrying to get rekor public key")

	return nil, err
}

func (i resolvePubKeyAction) requestPublicKey(basePath string) ([]byte, error) {
	response, err := http.Get(fmt.Sprintf("%s/api/v1/log/publicKey", basePath))
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			i.Logger.V(1).Error(err, err.Error())
		}
	}(response.Body)

	if response.StatusCode == http.StatusOK {
		return io.ReadAll(response.Body)
	}
	return nil, fmt.Errorf("unexpected http response %s", response.Status)
}
