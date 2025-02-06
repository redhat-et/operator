package action

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/controller/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	client2 "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

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
	current := obj.DeepCopyObject().(client2.Object)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var (
			currentStatus, expectedStatus *reflect.Value
			e                             error
		)

		if e = action.Client.Get(ctx, client.ObjectKeyFromObject(obj), current); e != nil {
			return e
		}

		if currentStatus, e = getStatus(current); e != nil {
			return e
		}

		if expectedStatus, e = getStatus(obj); e != nil {
			return e
		}

		if !reflect.DeepEqual(expectedStatus.Interface(), currentStatus.Interface()) {
			if !currentStatus.CanSet() {
				return errors.New("can not set status field")
			}
			currentStatus.Set(*expectedStatus)
		}
		return action.Client.Status().Update(ctx, current)
	})
	return &Result{Err: err}
}

func (action *BaseAction) Error(ctx context.Context, err error, instance apis.ConditionsAwareObject, conditions ...metav1.Condition) *Result {
	if errors.Is(err, reconcile.TerminalError(err)) {
		instance.SetCondition(metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.GetGeneration(),
		})
	}

	for _, condition := range conditions {
		instance.SetCondition(condition)
	}
	if errors.Is(err, reconcile.TerminalError(err)) || len(conditions) != 0 {
		if updateErr := action.Client.Status().Update(ctx, instance); updateErr != nil {
			err = errors.Join(err, updateErr)
		}
	}

	action.Logger.Error(err, "error during action execution")
	return &Result{
		Err: err,
	}
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

func getStatus(obj client2.Object) (*reflect.Value, error) {
	stat := reflect.ValueOf(obj).Elem().FieldByName("Status")
	if stat == reflect.ValueOf(nil) {
		return nil, errors.New("status field not found")
	}
	if !stat.IsValid() {
		return nil, errors.New("status field is not valid")
	}
	return &stat, nil
}
