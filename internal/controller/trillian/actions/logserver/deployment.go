package logserver

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/utils"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	trillianUtils "github.com/securesign/operator/internal/controller/trillian/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
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
		err     error
		updated bool
	)

	labels := constants.LabelsFor(actions.LogServerComponentName, actions.LogserverDeploymentName, instance.Name)
	server, err := trillianUtils.CreateTrillDeployment(instance, constants.TrillianServerImage, actions.LogserverDeploymentName, actions.RBACName, labels)
	if err != nil {
		return i.Failed(err)
	}

	err = utils.SetTrustedCA(&server.Spec.Template, utils.TrustedCAAnnotationToReference(instance.Annotations))
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Trillian server: %w", err), instance)
	}

	// TLS certificate
	if instance.Spec.TrillianServer.TLSCertificate.CertRef != nil {
		server.Spec.Template.Spec.Volumes = append(server.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "tls-cert",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{
						Sources: []corev1.VolumeProjection{
							{
								Secret: &corev1.SecretProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Spec.TrillianServer.TLSCertificate.CertRef.Name,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  instance.Spec.TrillianServer.TLSCertificate.CertRef.Key,
											Path: "tls.crt",
										},
									},
								},
							},
							{
								Secret: &corev1.SecretProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Spec.TrillianServer.TLSCertificate.PrivateKeyRef.Name,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  instance.Spec.TrillianServer.TLSCertificate.PrivateKeyRef.Key,
											Path: "tls.key",
										},
									},
								},
							},
							{
								ConfigMap: &corev1.ConfigMapProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Spec.TrillianServer.TLSCertificate.CACertRef.Name,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  "ca.crt", // User should use this key.
											Path: "ca.crt",
										},
									},
								},
							},
						},
					},
				},
			})
	} else if k8sutils.IsOpenShift() {
		i.Logger.V(1).Info("TLS: Using secrets/signing-key secret")
		server.Spec.Template.Spec.Volumes = append(server.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "tls-cert",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{
						Sources: []corev1.VolumeProjection{
							{
								Secret: &corev1.SecretProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Name + "-trillian-log-server-tls-secret",
									},
								},
							},
							{
								ConfigMap: &corev1.ConfigMapProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "ca-configmap",
									},
									Items: []corev1.KeyToPath{
										{
											Key:  "service-ca.crt",
											Path: "ca.crt",
										},
									},
								},
							},
						},
					},
				},
			})
	} else {
		i.Logger.V(1).Info("Communication between services is insecure")
	}

	if instance.Spec.TrillianServer.TLSCertificate.CertRef != nil || k8sutils.IsOpenShift() {
		server.Spec.Template.Spec.Containers[0].VolumeMounts = append(server.Spec.Template.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "tls-cert",
				MountPath: "/etc/ssl/certs",
				ReadOnly:  true,
			})
		server.Spec.Template.Spec.Containers[0].Args = append(server.Spec.Template.Spec.Containers[0].Args, "--mysql_tls_ca", "/etc/ssl/certs/ca.crt")
		server.Spec.Template.Spec.Containers[0].Args = append(server.Spec.Template.Spec.Containers[0].Args, "--mysql_server_name", "$(MYSQL_HOSTNAME)."+instance.Namespace+".svc")
	}

	if err = controllerutil.SetControllerReference(instance, server, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for server: %w", err))
	}

	if updated, err = i.Ensure(ctx, server); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Trillian server: %w", err), instance)
	}

	if updated {
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
