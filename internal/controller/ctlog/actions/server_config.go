package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	utils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	ctlogUtils "github.com/securesign/operator/internal/controller/ctlog/utils"
	trillian "github.com/securesign/operator/internal/controller/trillian/actions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const ConfigSecretNameFormat = "ctlog-config-%s"

func NewServerConfigAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &serverConfig{}
}

type serverConfig struct {
	action.BaseAction
}

func (i serverConfig) Name() string {
	return "server config"
}

func (i serverConfig) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {

	if instance.Status.ServerConfigRef == nil {
		return true
	}

	if instance.Spec.ServerConfigRef != nil {
		return !equality.Semantic.DeepEqual(instance.Spec.ServerConfigRef, instance.Status.ServerConfigRef)
	}

	return !meta.IsStatusConditionTrue(instance.Status.Conditions, ServerConfigCondition)
}

func (i serverConfig) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	// Return to pending state due changes in ServerConfigRef
	if meta.IsStatusConditionTrue(instance.Status.Conditions, ServerConfigCondition) {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    ServerConfigCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Pending,
			Message: "resolving server config",
		})
		return i.StatusUpdate(ctx, instance)
	}

	var (
		cfg *ctlogUtils.Config
		err error
	)

	if instance.Spec.ServerConfigRef != nil {
		instance.Status.ServerConfigRef = instance.Spec.ServerConfigRef
		i.Recorder.Eventf(instance, corev1.EventTypeNormal, "CTLogConfigUpdated", "CTLog config updated: %s", instance.Status.ServerConfigRef.Name)
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: ServerConfigCondition,
			Status: metav1.ConditionTrue, Reason: constants.Ready, Message: "CTLog config updated"})
		return i.StatusUpdate(ctx, instance)
	}

	switch {
	case instance.Status.TreeID == nil:
		return i.Failed(fmt.Errorf("%s: %v", i.Name(), ctlogUtils.TreeNotSpecified))
	case instance.Status.PrivateKeyRef == nil:
		return i.Failed(fmt.Errorf("%s: %v", i.Name(), ctlogUtils.PrivateKeyNotSpecified))
	case instance.Spec.Trillian.Port == nil:
		return i.Failed(fmt.Errorf("%s: %v", i.Name(), ctlogUtils.TrillianPortNotSpecified))
	case instance.Spec.Trillian.Address == "":
		instance.Spec.Trillian.Address = fmt.Sprintf("%s.%s.svc", trillian.LogserverDeploymentName, instance.Namespace)
	}

	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)

	trillianService := instance.DeepCopy().Spec.Trillian

	rootCerts, err := i.handleRootCertificates(instance)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    ServerConfigCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Pending,
			Message: fmt.Sprintf("Waiting for Fulcio root certificate: %v", err.Error()),
		})
		i.StatusUpdate(ctx, instance)
		return i.Requeue()
	}

	signerConfig, err := ctlogUtils.ResolveSignerConfig(i.Client, instance)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    ServerConfigCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Pending,
			Message: "Waiting for Ctlog private key secret",
		})
		i.StatusUpdate(ctx, instance)
		return i.Requeue()
	}

	if cfg, err = ctlogUtils.CTlogConfig(fmt.Sprintf("%s:%d", trillianService.Address, *trillianService.Port), *instance.Status.TreeID, rootCerts, signerConfig); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    ServerConfigCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create CTLog configuration: %w", err), instance)
	}

	data, err := cfg.Marshal()
	if err != nil {
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("failed to marshal CTLog configuration: %w", err), instance)
	}

	newConfig := utils.CreateImmutableSecret(fmt.Sprintf(ConfigSecretNameFormat, instance.Name), instance.Namespace, data, labels)

	if err = controllerutil.SetControllerReference(instance, newConfig, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Secret: %w", err))
	}

	_, err = i.Ensure(ctx, newConfig)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    ServerConfigCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}

	// invalidate server config
	if instance.Status.ServerConfigRef != nil {
		if err = i.Client.Delete(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instance.Status.ServerConfigRef.Name,
				Namespace: instance.Namespace,
			},
		}); err != nil {
			if !k8sErrors.IsNotFound(err) {
				return i.Failed(err)
			}
		}
		i.Recorder.Eventf(instance, corev1.EventTypeNormal, "CTLogConfigDeleted", "CTLog config deleted: %s", instance.Status.ServerConfigRef.Name)
	}

	instance.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: newConfig.Name}

	i.Recorder.Eventf(instance, corev1.EventTypeNormal, "CTLogConfigUpdated", "CTLog config updated: %s", newConfig.Name)
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: ServerConfigCondition,
		Status: metav1.ConditionTrue, Reason: constants.Ready, Message: "CTLog config created"})
	return i.StatusUpdate(ctx, instance)
}

func (i serverConfig) handleRootCertificates(instance *rhtasv1alpha1.CTlog) ([]ctlogUtils.RootCertificate, error) {
	certs := make([]ctlogUtils.RootCertificate, 0)

	for _, selector := range instance.Status.RootCertificates {
		data, err := utils.GetSecretData(i.Client, instance.Namespace, &selector)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", selector.Name, selector.Key, err)
		}
		certs = append(certs, data)
	}

	return certs, nil
}
