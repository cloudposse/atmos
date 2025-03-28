package utils

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/list/errors"
)

// IsNoValuesFoundError checks if an error is a NoValuesFoundError.
func IsNoValuesFoundError(err error) bool {
	_, ok := err.(*errors.NoValuesFoundError)
	return ok
}

// IsEmptyTable checks if the output is an empty table (only contains headers and formatting).
func IsEmptyTable(output string) bool {
	if output == "" {
		return true
	}

	newlineCount := strings.Count(output, "\n")
	if newlineCount <= 4 {
		return true
	}

	return false
}
