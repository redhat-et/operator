package server

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/action/tree"
)

func NewResolveTreeAction() action.Action[*rhtasv1alpha1.Rekor] {
	wrapper := tree.Wrapper[*rhtasv1alpha1.Rekor](
		func(rekor *rhtasv1alpha1.Rekor) *int64 {
			return rekor.Spec.TreeID
		},
		func(rekor *rhtasv1alpha1.Rekor) *int64 {
			return rekor.Status.TreeID
		},
		func(rekor *rhtasv1alpha1.Rekor, i *int64) {
			rekor.Status.TreeID = i
		},
		func(rekor *rhtasv1alpha1.Rekor) *rhtasv1alpha1.TrillianService {
			return &rekor.Spec.Trillian
		})
	return tree.NewResolveTreeAction[*rhtasv1alpha1.Rekor]("rekor", wrapper)
}
