package tree

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/job"
	"github.com/securesign/operator/internal/controller/common/utils/tls"
	ensureTls "github.com/securesign/operator/internal/controller/common/utils/tls/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	actions2 "github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/images"
	"golang.org/x/exp/maps"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewResolveTreeAction[T tlsAwareObject](component string, wrapper func(T) *wrapper[T]) action.Action[T] {
	return &resolveTree[T]{
		component:       component,
		treeDisplayName: fmt.Sprintf("%s-tree", component),
		wrapper:         wrapper,
	}
}

type resolveTree[T tlsAwareObject] struct {
	action.BaseAction
	component       string
	treeDisplayName string
	wrapper         func(T) *wrapper[T]
}

func (i resolveTree[T]) Name() string {
	return "resolve tree"
}

func (i resolveTree[T]) CanHandle(ctx context.Context, instance T) bool {
	wrapped := i.wrapper(instance)

	switch {
	case wrapped.GetStatusTreeID() == nil:
		return true
	case wrapped.GetTreeID() != nil:
		return !equality.Semantic.DeepEqual(wrapped.GetTreeID(), wrapped.GetStatusTreeID())
	default:
		return !meta.IsStatusConditionTrue(instance.GetConditions(), JobCondition)
	}
}

func (i resolveTree[T]) handleManual(ctx context.Context, instance T) *action.Result {
	wrapped := i.wrapper(instance)

	if wrapped.GetTreeID() != nil && *wrapped.GetTreeID() != int64(0) {
		wrapped.SetStatusTreeID(wrapped.GetTreeID())
		return i.StatusUpdate(ctx, instance)
	}

	return i.Continue()
}

func (i resolveTree[T]) handleRbac(ctx context.Context, instance T) *action.Result {
	var err error
	rbacName := fmt.Sprintf(RBACNameMask, i.component)

	labels := labels.For("createtree", i.component, instance.GetName())

	// ServiceAccount
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbacName,
			Namespace: instance.GetNamespace(),
		},
	},
		ensure.ControllerReference[*corev1.ServiceAccount](instance, i.Client),
		ensure.Labels[*corev1.ServiceAccount](maps.Keys(labels), labels),
	); err != nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create SA: %w", err)), instance)
	}

	// Role
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbacName,
			Namespace: instance.GetNamespace(),
		},
	},
		ensure.ControllerReference[*rbacv1.Role](instance, i.Client),
		ensure.Labels[*rbacv1.Role](maps.Keys(labels), labels),
		kubernetes.EnsureRoleRules(
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"patch"},
			}),
	); err != nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create Role: %w", err)), instance)
	}

	// RoleBinding
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbacName,
			Namespace: instance.GetNamespace(),
		},
	},
		ensure.ControllerReference[*rbacv1.RoleBinding](instance, i.Client),
		ensure.Labels[*rbacv1.RoleBinding](maps.Keys(labels), labels),
		kubernetes.EnsureRoleBinding(
			rbacv1.RoleRef{
				APIGroup: corev1.SchemeGroupVersion.Group,
				Kind:     "Role",
				Name:     rbacName,
			},
			rbacv1.Subject{Kind: "ServiceAccount", Name: rbacName, Namespace: instance.GetNamespace()},
		),
	); err != nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create RoleBinding: %w", err)), instance)
	}

	return i.Continue()
}

func (i resolveTree[T]) handleConfigMap(ctx context.Context, instance T) *action.Result {
	var (
		result controllerutil.OperationResult
		err    error
	)

	labels := labels.For("createtree", i.component, instance.GetName())

	// Needed for configMap clean-up
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(configMapResultMask, i.component, instance.GetName()),
			Namespace: instance.GetNamespace(),
		},
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		configMap,
		ensure.ControllerReference[*corev1.ConfigMap](instance, i.Client),
		ensure.Labels[*corev1.ConfigMap](maps.Keys(labels), labels),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s ConfigMap: %w", configMap.GetName(), err), instance)
	}

	if result != controllerutil.OperationResultNone {
		instance.SetCondition(metav1.Condition{
			Type:    JobCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: fmt.Sprintf("ConfigMap `%s` %s", configMap.GetName(), result)},
		)
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i resolveTree[T]) handleJob(ctx context.Context, instance T) *action.Result {
	var err error
	var trillUrl string
	wrapped := i.wrapper(instance)

	labels := labels.For("createtree", i.component, instance.GetName())

	configMapName := fmt.Sprintf(configMapResultMask, i.component, instance.GetName())
	configMap, err := kubernetes.GetConfigMap(ctx, i.Client, instance.GetNamespace(), configMapName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return i.Requeue()
		}
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not get configmap: %w", err)), instance)
	}

	if val, ok := configMap.GetAnnotations()[jobReferenceAnnotation]; ok && val != "" {
		return i.Continue()
	}

	trillianService := wrapped.GetTrillianService()

	switch {
	case trillianService.Port == nil:
		err = fmt.Errorf("%s: %v", i.Name(), TrillianPortNotSpecified)
	case trillianService.Address == "":
		trillUrl = fmt.Sprintf("%s.%s.svc:%d", actions2.LogserverDeploymentName, instance.GetNamespace(), *trillianService.Port)
	default:
		trillUrl = fmt.Sprintf("%s:%d", trillianService.Address, *trillianService.Port)
	}
	if err != nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not resolve trillian service: %w", err)), instance)
	}
	i.Logger.V(1).Info("trillian logserver", "address", trillUrl)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf(JobNameMask, i.component),
			Namespace:    instance.GetNamespace(),
		},
	}

	extraArgs := []string{}
	if instance.GetTrustedCA() != nil || kubernetes.IsOpenShift() {
		caPath, err := tls.CAPath(ctx, i.Client, instance)
		if err != nil {
			return i.Error(ctx, fmt.Errorf("could not get CA path: %w", err), instance)
		}
		extraArgs = append(extraArgs, "--tls_cert_file", caPath)
	}

	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		job,
		i.ensureJob(fmt.Sprintf(configMapResultMask, i.component, instance.GetName()), trillUrl, i.treeDisplayName, extraArgs...),
		ensure.ControllerReference[*batchv1.Job](instance, i.Client),
		ensure.Labels[*batchv1.Job](maps.Keys(labels), labels),
		func(object *batchv1.Job) error {
			return ensureTls.TrustedCA(instance.GetTrustedCA())(&object.Spec.Template)
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create segment backup job: %w", err), instance,
			metav1.Condition{
				Type:    JobCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Creating,
				Message: err.Error(),
			})
	}

	configMapAnnotations := map[string]string{
		jobReferenceAnnotation: job.GetName(),
	}
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, configMap,
		ensure.Annotations[*corev1.ConfigMap](maps.Keys(configMapAnnotations), configMapAnnotations),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not update annotations on %s ConfigMap: %w", configMap.GetName(), err), instance,
			metav1.Condition{
				Type:    JobCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Creating,
				Message: err.Error(),
			})
	}

	instance.SetCondition(metav1.Condition{
		Type:    JobCondition,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Initialize,
		Message: "createtree job created",
	})

	return i.StatusUpdate(ctx, instance)
}

func (i resolveTree[T]) handleJobFinished(ctx context.Context, instance T) *action.Result {
	var (
		jobName string
		ok      bool
		err     error
	)

	configMapName := fmt.Sprintf(configMapResultMask, i.component, instance.GetName())
	configMap, err := kubernetes.GetConfigMap(ctx, i.Client, instance.GetNamespace(), configMapName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return i.Requeue()
		}
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not get configmap: %w", err)), instance)
	}

	if jobName, ok = configMap.GetAnnotations()[jobReferenceAnnotation]; !ok || jobName == "" {
		return i.Requeue()
	}

	j, err := job.GetJob(ctx, i.Client, instance.GetNamespace(), jobName)
	if client.IgnoreNotFound(err) != nil {
		return i.Error(ctx, err, instance)
	}

	if j == nil {
		return i.Requeue()
	}
	i.Logger.V(1).Info("createtree job is already present.", "Succeeded", j.Status.Succeeded, "Failures", j.Status.Failed)

	if !job.IsCompleted(*j) {
		return i.Requeue()
	}

	if job.IsFailed(*j) {
		instance.SetCondition(metav1.Condition{
			Type:    JobCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: JobFailed.Error(),
		})
		return i.Error(ctx, reconcile.TerminalError(JobFailed), instance)
	}

	return i.Continue()
}

func (i resolveTree[T]) handleExtractJobResult(ctx context.Context, instance T) *action.Result {
	wrapped := i.wrapper(instance)

	configMapName := fmt.Sprintf(configMapResultMask, i.component, instance.GetName())
	configMap, err := kubernetes.GetConfigMap(ctx, i.Client, instance.GetNamespace(), configMapName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return i.Requeue()
		}
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not get configmap: %w", err)), instance)
	}

	if result, ok := configMap.Data[configMapResultField]; ok && result != "" {
		treeID, err := strconv.ParseInt(result, 10, 64)
		if err != nil {
			return i.Error(ctx, reconcile.TerminalError(err), instance)
		}

		wrapped.SetStatusTreeID(&treeID)
		instance.SetCondition(metav1.Condition{
			Type:   JobCondition,
			Status: metav1.ConditionTrue,
			Reason: constants.Ready,
		})
		i.Recorder.Eventf(instance, corev1.EventTypeNormal, "TrillianTreeCreated", "New Trillian tree created: %d", treeID)
		return i.StatusUpdate(ctx, instance)
	} else {
		i.Logger.V(1).Info("ConfigMap not ready or data is empty, requeuing reconciliation")
		return i.Requeue()
	}
}

func (i resolveTree[T]) ensureJob(cfgName, adminServer, displayName string, extraArgs ...string) func(*batchv1.Job) error {
	return func(job *batchv1.Job) error {

		spec := &job.Spec
		spec.Parallelism = utils.Pointer[int32](1)
		spec.Completions = utils.Pointer[int32](1)
		spec.ActiveDeadlineSeconds = utils.Pointer[int64](600)
		spec.BackoffLimit = utils.Pointer[int32](5)

		templateSpec := &spec.Template.Spec
		templateSpec.ServiceAccountName = fmt.Sprintf(RBACNameMask, i.component)
		templateSpec.RestartPolicy = "OnFailure"

		container := kubernetes.FindContainerByNameOrCreate(templateSpec, "createtree")
		container.Image = images.Registry.Get(images.TrillianCreateTree)
		container.Command = []string{"/bin/sh", "-c"}
		container.Args = []string{string(jobScript)}

		cfgNameEnv := kubernetes.FindEnvByNameOrCreate(container, "CONFIGMAP_NAME")
		cfgNameEnv.Value = cfgName

		adminServerEnv := kubernetes.FindEnvByNameOrCreate(container, "ADMIN_SERVER")
		adminServerEnv.Value = adminServer

		displayNameEnv := kubernetes.FindEnvByNameOrCreate(container, "DISPLAY_NAME")
		displayNameEnv.Value = displayName

		extraArgsEnv := kubernetes.FindEnvByNameOrCreate(container, "EXTRA_ARGS")
		extraArgsEnv.Value = strings.Join(extraArgs, " ")

		return nil
	}
}

func (i resolveTree[T]) Handle(ctx context.Context, instance T) *action.Result {
	result := i.handleManual(ctx, instance)
	if result != nil {
		return result
	}

	result = i.handleRbac(ctx, instance)
	if result != nil {
		return result
	}

	result = i.handleConfigMap(ctx, instance)
	if result != nil {
		return result
	}

	result = i.handleJob(ctx, instance)
	if result != nil {
		return result
	}

	result = i.handleJobFinished(ctx, instance)
	if result != nil {
		return result
	}

	result = i.handleExtractJobResult(ctx, instance)
	if result != nil {
		return result
	}

	return i.Continue()
}
