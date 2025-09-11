package utils

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
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

// Deprecated: use SliceOfInterfacesToSliceOfStrings
func SliceOfInterfacesToSliceOdStrings(input []any) []string {
	return SliceOfInterfacesToSliceOfStrings(input)
}

// SliceOfInterfacesToSliceOfStringsWithTypeAssertion takes a slice of interfaces and converts it to a slice of strings using type assertion.
// This function returns an error if any element is not a string.
func SliceOfInterfacesToSliceOfStringsWithTypeAssertion(input []any) ([]string, error) {
	if input == nil {
		return nil, errors.New("input must not be nil")
	}

	output := make([]string, len(input))
	for i, current := range input {
		s, ok := current.(string)
		if !ok {
			return nil, errors.New(fmt.Sprintf("element at index %d is not a string, got %T", i, current))
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
