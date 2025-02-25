package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	v12 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewStatusUrlAction() action.Action[*rhtasv1alpha1.Tuf] {
	return &statusUrlAction{}
}

type statusUrlAction struct {
	action.BaseAction
}

func (i statusUrlAction) Name() string {
	return "status-url"
}

func (i statusUrlAction) CanHandle(_ context.Context, tuf *rhtasv1alpha1.Tuf) bool {
	c := meta.FindStatusCondition(tuf.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i statusUrlAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	var url string
	if instance.Spec.ExternalAccess.Enabled {
		protocol := "http://"
		ingress := &v12.Ingress{}
		err := i.Client.Get(ctx, types.NamespacedName{Name: tufConstants.ComponentName, Namespace: instance.Namespace}, ingress)
		if err != nil {
			return i.Error(ctx, err, instance)
		}
		if len(ingress.Spec.TLS) > 0 {
			protocol = "https://"
		}
		url = protocol + ingress.Spec.Rules[0].Host
	} else {
		url = fmt.Sprintf("http://%s.%s.svc", tufConstants.DeploymentName, instance.Namespace)
	}
	if url == instance.Status.Url {
		return i.Continue()
	}

	instance.Status.Url = url
	return i.StatusUpdate(ctx, instance)
}
