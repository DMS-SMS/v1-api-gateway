package errors

import "errors"

var (
	AvailableNodeNotExist = errors.New("available service node not exist in consul")
)
