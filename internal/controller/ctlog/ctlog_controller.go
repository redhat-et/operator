/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ctlog

import (
	"context"

	olpredicate "github.com/operator-framework/operator-lib/predicate"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/action/transitions"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/securesign/operator/internal/controller/ctlog/actions"
	fulcioActions "github.com/securesign/operator/internal/controller/fulcio/actions"
	v12 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/securesign/operator/internal/controller/common/action"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

// CTlogReconciler reconciles a CTlog object
type CTlogReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=ctlogs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=ctlogs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=ctlogs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CTlog object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *CTlogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	var instance rhtasv1alpha1.CTlog
	rlog := log.FromContext(ctx)

	if err := r.Client.Get(ctx, req.NamespacedName, &instance); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Fetch the namespace
	var namespace v12.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: req.Namespace}, &namespace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the namespace is marked for deletion
	if !namespace.DeletionTimestamp.IsZero() {
		rlog.Info("namespace is marked for deletion, stopping reconciliation", "namespace", req.Namespace)
		return ctrl.Result{}, nil
	}

	target := instance.DeepCopy()
	acs := []action.Action[*rhtasv1alpha1.CTlog]{
		transitions.NewToPendingPhaseAction[*rhtasv1alpha1.CTlog](func(_ *rhtasv1alpha1.CTlog) []string {
			return []string{
				actions.CertCondition,
				actions.SignerCondition,
				actions.ServerConfigCondition,
				actions.PublicKeyCondition,
				actions.TreeCondition,
			}
		}),

		actions.NewGenerateSignerAction(),
		actions.NewHandleFulcioCertAction(),
		actions.NewResolvePubKeyAction(),
		actions.NewResolveTreeAction(),
		actions.NewServerConfigAction(),

		transitions.NewToCreatePhaseAction[*rhtasv1alpha1.CTlog](),

		actions.NewRBACAction(),
		actions.NewDeployAction(),
		actions.NewServiceAction(),
		actions.NewCreateMonitorAction(),

		transitions.NewToInitializePhaseAction[*rhtasv1alpha1.CTlog](),

		actions.NewInitializeAction(),
	}

	for _, a := range acs {
		rlog.V(2).Info("Executing " + a.Name())
		a.InjectClient(r.Client)
		a.InjectLogger(rlog.WithName(a.Name()))
		a.InjectRecorder(r.Recorder)

		if a.CanHandle(ctx, target) {
			rlog.V(1).Info("Executing " + a.Name())
			result := a.Handle(ctx, target)
			if result != nil {
				return result.Result, result.Err
			}
		}
	}
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CTlogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Filter out with the pause annotation.
	pause, err := olpredicate.NewPause(annotations.PausedReconciliation)
	if err != nil {
		return err
	}

	secretPredicate, err := predicate.LabelSelectorPredicate(metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      fulcioActions.FulcioCALabel,
			Operator: metav1.LabelSelectorOpExists,
		},
	}})
	if err != nil {
		return err
	}

	partialSecret := &metav1.PartialObjectMetadata{}
	partialSecret.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	})

	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(pause).
		For(&rhtasv1alpha1.CTlog{}).
		Owns(&v1.Deployment{}).
		Owns(&v12.Service{}).
		WatchesMetadata(partialSecret, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
			val, ok := object.GetLabels()["app.kubernetes.io/instance"]
			if ok {
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Namespace: object.GetNamespace(),
							Name:      val,
						},
					},
				}
			}

			list := &rhtasv1alpha1.CTlogList{}
			err := mgr.GetClient().List(ctx, list, client.InNamespace(object.GetNamespace()))
			if err != nil {
				return make([]reconcile.Request, 0)
			}

			requests := make([]reconcile.Request, len(list.Items))
			for i, k := range list.Items {
				requests[i] = reconcile.Request{NamespacedName: types.NamespacedName{Namespace: object.GetNamespace(), Name: k.Name}}
			}
			return requests

		}), builder.WithPredicates(secretPredicate)).
		Complete(r)
}
