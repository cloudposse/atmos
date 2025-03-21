package utils

import (
	"github.com/cloudposse/atmos/pkg/list/errors"
)

// IsNoValuesFoundError checks if an error is a NoValuesFoundError.
func IsNoValuesFoundError(err error) bool {
	_, ok := err.(*errors.NoValuesFoundError)
	return ok
}

// Common flag names and descriptions.
const (
	FlagFormat     = "format"
	FlagMaxColumns = "max-columns"
	FlagDelimiter  = "delimiter"
	FlagStack      = "stack"
	FlagQuery      = "query"
)
