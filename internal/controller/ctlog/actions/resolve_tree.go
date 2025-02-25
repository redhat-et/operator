package actions

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/action/tree"
)

func NewResolveTreeAction() action.Action[*rhtasv1alpha1.CTlog] {
	wrapper := tree.Wrapper[*rhtasv1alpha1.CTlog](
		func(rekor *rhtasv1alpha1.CTlog) *int64 {
			return rekor.Spec.TreeID
		},
		func(rekor *rhtasv1alpha1.CTlog) *int64 {
			return rekor.Status.TreeID
		},
		func(rekor *rhtasv1alpha1.CTlog, i *int64) {
			rekor.Status.TreeID = i
		},
		func(rekor *rhtasv1alpha1.CTlog) *rhtasv1alpha1.TrillianService {
			return &rekor.Spec.Trillian
		})
	return tree.NewResolveTreeAction[*rhtasv1alpha1.CTlog]("ctlog", wrapper)
}
