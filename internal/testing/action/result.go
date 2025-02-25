package action

import (
	"time"

	"github.com/securesign/operator/internal/controller/common/action"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Continue() *action.Result {
	return nil
}

func StatusUpdate() *action.Result {
	return &action.Result{Result: reconcile.Result{Requeue: false}}
}

func Error(err error) *action.Result {
	return &action.Result{
		Result: reconcile.Result{},
		Err:    err,
	}
}

func Failed(err error) *action.Result {
	return &action.Result{
		Result: reconcile.Result{RequeueAfter: time.Duration(5) * time.Second},
		Err:    err,
	}
}

func FailedWithStatusUpdate(err error) *action.Result {
	return &action.Result{Result: reconcile.Result{Requeue: false}, Err: err}
}

func Return() *action.Result {
	return &action.Result{
		Result: reconcile.Result{Requeue: false},
		Err:    nil,
	}
}

func Requeue() *action.Result {
	return &action.Result{
		Result: reconcile.Result{RequeueAfter: 5 * time.Second},
		Err:    nil,
	}
}

func IsFailed(result *action.Result) bool {
	if result == nil {
		return false
	}
	return result.Err != nil
}

func IsRequeue(result *action.Result) bool {
	if result == nil {
		return false
	}
	return result.Result.Requeue || result.Result.RequeueAfter > 0
}
