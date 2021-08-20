package exec

import (
	u "atmos/internal/utils"
)

var (
	commonFlags = []string{"--stack", "-s"}
)

// RemoveCommonFlags removes common CLI flags from the provided list of arguments
func RemoveCommonFlags(args []string) []string {
	result := []string{}

	for _, arg := range args {
		if !u.SliceContainsStringThatTheStringStartsWith(commonFlags, arg) {
			result = append(result, arg)
		}
	}

	return result
}
