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

package tuf

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TufReconciler reconciles a Tuf object
type TufReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *TufReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	var instance rhtasv1alpha1.Tuf

	if err := r.Client.Get(ctx, req.NamespacedName, &instance); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue

			// TODO: HACK - redefine APIe
			owner := rhtasv1alpha1.Securesign{}
			r.Client.Get(ctx, req.NamespacedName, &owner)
			if owner.Status.Tuf != "" {
				r.Client.Get(ctx, types.NamespacedName{Name: owner.Status.Tuf, Namespace: req.Namespace}, &instance)
			}
		} else {
			// Error reading the object - requeue the request.
			return reconcile.Result{}, err
		}
	}
	target := instance.DeepCopy()
	actions := []Action{
		NewPendingAction(),
		NewInitializeAction(),
		NewWaitAction(),
	}

	for _, a := range actions {
		a.InjectClient(r.Client)

		if a.CanHandle(target) {
			newTarget, err := a.Handle(ctx, target)
			if err != nil {
				if newTarget != nil {
					_ = r.Status().Update(ctx, newTarget)
				}
				return reconcile.Result{}, err
			}

			if newTarget != nil {
				if err := r.Status().Update(ctx, newTarget); err != nil {
					return reconcile.Result{}, err
				}
			}
			break
		}
	}
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TufReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rhtasv1alpha1.Tuf{}).
		// TODO: redefine API - we need to watch Rekor changes as well
		Watches(&rhtasv1alpha1.Rekor{}, handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &rhtasv1alpha1.Securesign{})).
		Complete(r)
}
