package tree

import (
	"errors"
)

var (
	TrillianPortNotSpecified = errors.New("trillian port not specified")
	JobFailed                = errors.New("createtree job failed")
)
