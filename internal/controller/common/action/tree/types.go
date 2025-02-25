package tree

import (
	_ "embed"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/apis"
)

//go:embed data/script.sh
var jobScript []byte

const (
	RBACNameMask           = "%s-createtree-job"
	JobNameMask            = "%s-createtree-job-"
	JobCondition           = "Tree"
	configMapResultMask    = "%s-%s-createtree-result"
	configMapResultField   = "tree_id"
	jobReferenceAnnotation = "rhtas.redhat.com/createtree-job"
)

func Wrapper[T tlsAwareObject](getTree, getStatusTree func(T) *int64, setStatusTree func(T, *int64), getTrillianService func(T) *v1alpha1.TrillianService) func(T) *wrapper[T] {
	return func(obj T) *wrapper[T] {
		return &wrapper[T]{
			object:              obj,
			callTree:            getTree,
			callStatusTree:      getStatusTree,
			callSetStatusTree:   setStatusTree,
			callTrillianService: getTrillianService,
		}
	}
}

type wrapper[T tlsAwareObject] struct {
	object T

	callTree            func(T) *int64
	callStatusTree      func(T) *int64
	callSetStatusTree   func(T, *int64)
	callTrillianService func(T) *v1alpha1.TrillianService
}

func (c *wrapper[T]) GetTreeID() *int64 {
	return c.callTree(c.object)
}

func (c *wrapper[T]) GetStatusTreeID() *int64 {
	return c.callStatusTree(c.object)
}

func (c *wrapper[T]) SetStatusTreeID(treeID *int64) {
	c.callSetStatusTree(c.object, treeID)
}

func (c *wrapper[T]) GetTrillianService() *v1alpha1.TrillianService {
	return c.callTrillianService(c.object)
}

type tlsAwareObject interface {
	apis.ConditionsAwareObject
	apis.TlsClient
}
