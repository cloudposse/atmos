package exec

import (
	u "atmos/internal/utils"
	"strings"
)

var (
	commonFlags = []string{"--stack", "-s"}
)

// RemoveCommonFlags removes common CLI flags from the provided list of arguments/flags
func RemoveCommonFlags(args []string) []string {
	result := []string{}
	indexesToRemove := []int{}

	for i, arg := range args {
		for _, f := range commonFlags {
			if arg == f {
				indexesToRemove = append(indexesToRemove, i)
				indexesToRemove = append(indexesToRemove, i+1)
			} else if strings.HasPrefix(arg, f+"=") {
				indexesToRemove = append(indexesToRemove, i)
			}
		}
	}

	for i, arg := range args {
		if !u.SliceContainsInt(indexesToRemove, i) {
			result = append(result, arg)
		}
	}

	return result
}
