package utils

import (
	"errors"
	"fmt"
	"strings"
)

// Package-level sentinel errors for slice operations
var (
	ErrNilInput         = errors.New("input must not be nil")
	ErrNonStringElement = errors.New("element is not a string")
)

// SliceContainsString checks if a string is present in a slice
func SliceContainsString(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// SliceContainsInt checks if an int is present in a slice
func SliceContainsInt(s []int, i int) bool {
	for _, v := range s {
		if v == i {
			return true
		}
	}
	return false
}

// SliceContainsStringStartsWith checks if a slice contains a string that the given string begins with
func SliceContainsStringStartsWith(s []string, str string) bool {
	for _, v := range s {
		if strings.HasPrefix(str, v) {
			return true
		}
	}
	return false
}

// SliceContainsStringHasPrefix checks if a slice contains a string that begins with the given prefix
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
	res := make([]string, len(input))

	for i, v := range input {
		res[i] = fmt.Sprintf("%v", v)
	}

	return res
}

// SliceOfInterfacesToSliceOfStringsWithTypeAssertion takes a slice of interfaces and converts it to a slice of strings using type assertion.
// This function returns an error if any element is not a string.
func SliceOfInterfacesToSliceOfStringsWithTypeAssertion(input []any) ([]string, error) {
	if input == nil {
		return nil, ErrNilInput
	}

	output := make([]string, len(input))
	for i, current := range input {
		s, ok := current.(string)
		if !ok {
			return nil, fmt.Errorf("%w: index=%d, got=%T", ErrNonStringElement, i, current)
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
	for i, v := range slice {
		if v == str {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// SliceRemoveFlag removes all occurrences of a flag from a slice, handling both "--flag" and "--flag=value" forms.
// This function safely handles multiple occurrences and returns a new slice with all flag instances removed.
// The function preserves the original slice and returns a new slice with the flags removed.
func SliceRemoveFlag(slice []string, flagName string) []string {
	if slice == nil {
		return nil
	}

	result := make([]string, 0, len(slice))
	flagPrefix := "--" + flagName + "="

	for _, item := range slice {
		// Skip exact flag matches (--flag)
		if item == "--"+flagName {
			continue
		}
		// Skip flag with value matches (--flag=value)
		if strings.HasPrefix(item, flagPrefix) {
			continue
		}
		result = append(result, item)
	}

	return result
}
