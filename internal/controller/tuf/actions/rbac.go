package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"golang.org/x/exp/maps"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewRBACAction() action.Action[*rhtasv1alpha1.Tuf] {
	return &rbacAction{}
}

type rbacAction struct {
	action.BaseAction
}

func (i rbacAction) Name() string {
	return "ensure RBAC"
}

func (i rbacAction) CanHandle(_ context.Context, tuf *rhtasv1alpha1.Tuf) bool {
	c := meta.FindStatusCondition(tuf.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i rbacAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	var (
		err error
	)
	labels := labels.For(tufConstants.ComponentName, tufConstants.RBACName, instance.Name)

	// don't re-enqueue for RBAC in any case (except failure)

	// ServiceAccount
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tufConstants.RBACName,
			Namespace: instance.Namespace,
		},
	},
		ensure.ControllerReference[*v1.ServiceAccount](instance, i.Client),
		ensure.Labels[*v1.ServiceAccount](maps.Keys(labels), labels),
	); err != nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create SA: %w", err)), instance)
	}

	// Role
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tufConstants.RBACName,
			Namespace: instance.Namespace,
		},
	},
		ensure.ControllerReference[*rbacv1.Role](instance, i.Client),
		ensure.Labels[*rbacv1.Role](maps.Keys(labels), labels),
		kubernetes.EnsureRoleRules(
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"create", "get", "update"},
			},
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"create", "get", "update"},
			}),
	); err != nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create Role: %w", err)), instance)
	}

	// RoleBinding
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tufConstants.RBACName,
			Namespace: instance.Namespace,
		},
	},
		ensure.ControllerReference[*rbacv1.RoleBinding](instance, i.Client),
		ensure.Labels[*rbacv1.RoleBinding](maps.Keys(labels), labels),
		kubernetes.EnsureRoleBinding(
			rbacv1.RoleRef{
				APIGroup: v1.SchemeGroupVersion.Group,
				Kind:     "Role",
				Name:     tufConstants.RBACName,
			},
			rbacv1.Subject{Kind: "ServiceAccount", Name: tufConstants.RBACName, Namespace: instance.Namespace}),
	); err != nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not set control reference for roleBinding: %w", err)), instance)
	}
	return i.Continue()
}
