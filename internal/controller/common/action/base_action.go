package action

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/securesign/operator/internal/controller/annotations"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	client2 "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// OptimisticLockErrorMsg - ignore update error: https://github.com/kubernetes/kubernetes/issues/28149
const OptimisticLockErrorMsg = "the object has been modified; please apply your changes to the latest version and try again"

type BaseAction struct {
	Client   client.Client
	Recorder record.EventRecorder
	Logger   logr.Logger
}

func (action *BaseAction) InjectClient(client client.Client) {
	action.Client = client
}

func (action *BaseAction) InjectRecorder(recorder record.EventRecorder) {
	action.Recorder = recorder
}

func (action *BaseAction) InjectLogger(logger logr.Logger) {
	action.Logger = logger
}

func (action *BaseAction) Continue() *Result {
	return nil
}

func (action *BaseAction) StatusUpdate(ctx context.Context, obj client2.Object) *Result {
	if err := action.Client.Status().Update(ctx, obj); err != nil {
		if strings.Contains(err.Error(), OptimisticLockErrorMsg) {
			return &Result{Result: reconcile.Result{RequeueAfter: 1 * time.Second}, Err: nil}
		}
		return action.Failed(err)
	}
	// Requeue will be caused by update
	return &Result{Result: reconcile.Result{Requeue: false}}
}

func (action *BaseAction) Failed(err error) *Result {
	action.Logger.Error(err, "error during action execution")
	return &Result{
		Result: reconcile.Result{RequeueAfter: time.Duration(5) * time.Second},
		Err:    err,
	}
}

func (action *BaseAction) FailedWithStatusUpdate(ctx context.Context, err error, instance client2.Object) *Result {
	if e := action.Client.Status().Update(ctx, instance); e != nil {
		if strings.Contains(err.Error(), OptimisticLockErrorMsg) {
			return &Result{Result: reconcile.Result{RequeueAfter: 1 * time.Second}, Err: err}
		}
		err = errors.Join(e, err)
	}
	// Requeue will be caused by update
	return &Result{Result: reconcile.Result{Requeue: false}, Err: err}
}

func (action *BaseAction) Return() *Result {
	return &Result{
		Result: reconcile.Result{Requeue: false},
		Err:    nil,
	}
}

func (action *BaseAction) Requeue() *Result {
	return &Result{
		// always wait for a while before requeqe
		Result: reconcile.Result{RequeueAfter: 5 * time.Second},
		Err:    nil,
	}
}

func (action *BaseAction) Update(ctx context.Context, key types.NamespacedName, currentObj client2.Object, obj client2.Object) (bool, error) {
	action.Logger.Info("Updating object",
		"kind", reflect.TypeOf(currentObj).Elem().Name(), "Namespace", key.Namespace, "Name", key.Name)
	if err := action.Client.Update(ctx, currentObj); err != nil {
		if strings.Contains(err.Error(), OptimisticLockErrorMsg) {
			return action.Ensure(ctx, obj)
		}
		action.Logger.Error(err, "Failed to update object",
			"kind", reflect.TypeOf(obj).Elem().Name(), "Namespace", key.Namespace, "Name", key.Name)
		return false, err
	}
	return true, nil
}

func (action *BaseAction) Ensure(ctx context.Context, obj client2.Object) (bool, error) {
	key := client2.ObjectKeyFromObject(obj)
	var (
		currentObj client2.Object
		ok         bool
	)
	if currentObj, ok = obj.DeepCopyObject().(client2.Object); !ok {
		return false, errors.New("can't create DeepCopy object")
	}
	if err := action.Client.Get(ctx, key, currentObj); err != nil {
		if apierrors.IsNotFound(err) {
			action.Logger.Info("Creating object",
				"kind", reflect.TypeOf(obj).Elem().Name(), "name", key.Name)
			if err = action.Client.Create(ctx, obj); err != nil {
				if apierrors.IsAlreadyExists(err) {
					action.Logger.Info("Object already exists",
						"kind", reflect.TypeOf(obj).Elem().Name(), "Namespace", key.Namespace, "Name", key.Name)
					return false, nil
				}
				action.Logger.Error(err, "Failed to create new object",
					"kind", reflect.TypeOf(obj).Elem().Name(), "Namespace", key.Namespace, "Name", key.Name)
				return false, err
			}
			return true, nil
		}
		return false, err
	}

	annoStr, find := currentObj.GetAnnotations()[annotations.PausedReconciliation]
	if find {
		annoBool, _ := strconv.ParseBool(annoStr)
		if annoBool {
			return false, nil
		}
	}

	// Normalize both current and desired annotations
	currentAnnotations := normalizeAnnotations(annotations.FilterManaged(currentObj.GetAnnotations()))
	desiredAnnotations := normalizeAnnotations(annotations.FilterManaged(obj.GetAnnotations()))
	// Compare annotations, and update if they are different
	if !reflect.DeepEqual(currentAnnotations, desiredAnnotations) {
		currentObj.SetAnnotations(desiredAnnotations)
		action.Logger.Info("Updating Annotations",
			"kind", reflect.TypeOf(obj).Elem().Name(), "Namespace", key.Namespace, "Name", key.Name)
		if ok, err := action.Update(ctx, key, currentObj, obj); !ok || err != nil {
			return false, err
		}
	}

	currentSpec := reflect.ValueOf(currentObj).Elem().FieldByName("Spec")
	expectedSpec := reflect.ValueOf(obj).Elem().FieldByName("Spec")
	if currentSpec == reflect.ValueOf(nil) {
		// object without spec
		// return without update
		return false, nil
	}
	if !expectedSpec.IsValid() || !currentSpec.IsValid() {
		return false, errors.New("spec is not valid")
	}

	if equality.Semantic.DeepDerivative(expectedSpec.Interface(), currentSpec.Interface()) {
		return false, nil
	}
	if !currentSpec.CanSet() {
		return false, errors.New("can't set expected spec to current object")
	}
	currentSpec.Set(expectedSpec)
	return action.Update(ctx, key, currentObj, obj)
}

func normalizeAnnotations(annotations map[string]string) map[string]string {
	if annotations == nil {
		return map[string]string{}
	}
	return annotations
}
