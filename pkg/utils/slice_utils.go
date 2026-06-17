package utils

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// SliceContainsInt checks if an int is present in a slice.
func SliceContainsInt(s []int, i int) bool {
	defer perf.Track(nil, "utils.SliceContainsInt")()

	for _, v := range s {
		if v == i {
			return true
		}
	}
	return false
}

// SliceOfInterfacesToSliceOfStrings converts a slice of any to a slice of strings.
// Each element is converted to a string using fmt.Sprintf with the "%v" format.
func SliceOfInterfacesToSliceOfStrings(input []any) []string {
	defer perf.Track(nil, "utils.SliceOfInterfacesToSliceOfStrings")()

	res := make([]string, len(input))

	for i, v := range input {
		res[i] = fmt.Sprintf("%v", v)
	}

	return res
}

// SliceRemoveFlag removes all occurrences of a flag from a slice, handling both "--flag" and "--flag=value" forms.
// This function safely handles multiple occurrences and returns a new slice with all flag instances removed.
// The function preserves the original slice and returns a new slice with the flags removed.
func SliceRemoveFlag(slice []string, flagName string) []string {
	defer perf.Track(nil, "utils.SliceRemoveFlag")()

	if slice == nil {
		return nil
	}
	if flagName == "" {
		// No-op for empty flag names.
		return append([]string(nil), slice...)
	}

	result := make([]string, 0, len(slice))
	flagPrefix := "--" + flagName + "="

	for _, item := range slice {
		// Skip exact flag matches (--flag).
		if item == "--"+flagName {
			continue
		}
		// Skip flag with value matches (--flag=value).
		if strings.HasPrefix(item, flagPrefix) {
			continue
		}
		result = append(result, item)
	}

	return result
}
