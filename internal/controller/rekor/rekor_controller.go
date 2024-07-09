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

package rekor

import (
	"context"

	olpredicate "github.com/operator-framework/operator-lib/predicate"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/action/transitions"

	actions2 "github.com/securesign/operator/internal/controller/rekor/actions"
	backfillredis "github.com/securesign/operator/internal/controller/rekor/actions/backfillRedis"
	"github.com/securesign/operator/internal/controller/rekor/actions/redis"
	"github.com/securesign/operator/internal/controller/rekor/actions/server"
	"github.com/securesign/operator/internal/controller/rekor/actions/ui"
	v13 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/tools/record"

	"github.com/securesign/operator/internal/controller/common/action"
	v12 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
)

// RekorReconciler reconciles a Rekor object
type RekorReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=rekors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=rekors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=rekors/finalizers,verbs=update
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=secrets,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups="batch",resources=cronjobs,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Rekor object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *RekorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var instance rhtasv1alpha1.Rekor
	log := ctrllog.FromContext(ctx)
	log.V(1).Info("Reconciling Rekor", "request", req)

	if err := r.Client.Get(ctx, req.NamespacedName, &instance); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	target := instance.DeepCopy()
	actions := []action.Action[*rhtasv1alpha1.Rekor]{
		transitions.NewToPendingPhaseAction[*rhtasv1alpha1.Rekor](func(rekor *rhtasv1alpha1.Rekor) []string {
			components := []string{actions2.ServerCondition, actions2.RedisCondition, actions2.SignerCondition}
			if *rekor.Spec.RekorSearchUI.Enabled {
				components = append(components, actions2.UICondition)
			}
			return components
		}),

		server.NewGenerateSignerAction(),

		transitions.NewToCreatePhaseAction[*rhtasv1alpha1.Rekor](),

		actions2.NewRBACAction(),
		server.NewServerConfigAction(),
		server.NewCAConfigMapAction(),
		server.NewCreatePvcAction(),
		server.NewCreateTrillianTreeAction(),
		server.NewDeployAction(),
		server.NewCreateServiceAction(),
		server.NewCreateMonitorAction(),
		server.NewIngressAction(),
		server.NewStatusUrlAction(),

		redis.NewDeployAction(),
		redis.NewCreateServiceAction(),

		ui.NewDeployAction(),
		ui.NewCreateServiceAction(),
		ui.NewIngressAction(),

		backfillredis.NewBackfillRedisCronJobAction(),

		transitions.NewToInitializePhaseAction[*rhtasv1alpha1.Rekor](),
		// INITIALIZE
		server.NewInitializeAction(),
		server.NewResolvePubKeyAction(),

		ui.NewInitializeAction(),
		redis.NewInitializeAction(),

		// INITIALIZE -> READY
		actions2.NewInitializeAction(),
	}

	for _, a := range actions {
		a.InjectClient(r.Client)
		a.InjectLogger(log.WithName(a.Name()))
		a.InjectRecorder(r.Recorder)

		if a.CanHandle(ctx, target) {
			log.V(2).Info("Executing " + a.Name())
			result := a.Handle(ctx, target)
			if result != nil {
				return result.Result, result.Err
			}
		}
	}
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RekorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Filter out with the pause annotation.
	pause, err := olpredicate.NewPause(annotations.PausedReconciliation)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(pause).
		For(&rhtasv1alpha1.Rekor{}).
		Owns(&v12.Deployment{}).
		Owns(&v13.Service{}).
		Owns(&v13.ConfigMap{}).
		Owns(&v1.Ingress{}).
		Owns(&batchv1.CronJob{}).
		Complete(r)
}
