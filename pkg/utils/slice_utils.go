package utils

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// SliceContainsString checks if a string is present in a slice.
func SliceContainsString(s []string, str string) bool {
	defer perf.Track(nil, "utils.SliceContainsString")()

	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

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

// SliceContainsStringStartsWith checks if a slice contains a string that the given string begins with.
func SliceContainsStringStartsWith(s []string, str string) bool {
	for _, v := range s {
		if strings.HasPrefix(str, v) {
			return true
		}
	}
	return false
}

// SliceContainsStringHasPrefix checks if a slice contains a string that begins with the given prefix.
func SliceContainsStringHasPrefix(s []string, prefix string) bool {
	for _, v := range s {
		if strings.HasPrefix(v, prefix) {
			return true
		}
	}
	return false
}

// SliceOfStringsToSpaceSeparatedString joins a slice of strings into a single space-separated string.
func SliceOfStringsToSpaceSeparatedString(s []string) string {
	return strings.Join(s, " ")
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

// SliceOfInterfacesToSliceOfStringsWithTypeAssertion takes a slice of interfaces and converts it to a slice of strings using type assertion.
// This function returns an error if any element is not a string.
func SliceOfInterfacesToSliceOfStringsWithTypeAssertion(input []any) ([]string, error) {
	defer perf.Track(nil, "utils.SliceOfInterfacesToSliceOfStringsWithTypeAssertion")()

	if input == nil {
		return nil, errUtils.ErrNilInput
	}

	output := make([]string, len(input))
	for i, current := range input {
		s, ok := current.(string)
		if !ok {
			return nil, fmt.Errorf("%w: index=%d, got=%T", errUtils.ErrNonStringElement, i, current)
		}
		output[i] = s
	}

	return output, nil
}

// SliceRemoveString removes only the first occurrence of the provided string from a slice.
// This function may mutate the input slice's backing array (i.e., it is not a pure/non-mutating operation).
// Callers must use the returned slice (assign it) because the original slice may be modified or re-sliced.
// Advise callers to copy the slice before calling if they need to preserve the original contents.
func SliceRemoveString(slice []string, str string) []string {
	defer perf.Track(nil, "utils.SliceRemoveString")()

	for i, v := range slice {
		if v == str {
			// Avoid retaining reference to the removed element.
			copy(slice[i:], slice[i+1:])
			slice[len(slice)-1] = ""
			return slice[:len(slice)-1]
		}
	}
	return slice
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

// SliceRemoveFlagAndValue removes --flag and an optional following value (if the next arg
// does not start with "-"). It preserves order of remaining args.
func SliceRemoveFlagAndValue(args []string, flagName string) []string {
	defer perf.Track(nil, "utils.SliceRemoveFlagAndValue")()

	if args == nil || flagName == "" {
		return append([]string(nil), args...)
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--"+flagName {
			// Skip the flag and (optionally) its value.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "--"+flagName+"=") {
			continue
		}
		out = append(out, arg)
	}
	return out
}
