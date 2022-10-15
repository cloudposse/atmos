package utils

import (
	"strings"
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

// SliceOfStringsToSpaceSeparatedString checks if an int is present in a slice
func SliceOfStringsToSpaceSeparatedString(s []string) string {
	return strings.Join(s, " ")
}

// SliceOfInterfacesToSliceOdStrings converts a slice of any to a slice os strings
func SliceOfInterfacesToSliceOdStrings(input []any) []string {
	res := make([]string, len(input))

	for i, v := range input {
		res[i] = v.(string)
	}

	return res
}
