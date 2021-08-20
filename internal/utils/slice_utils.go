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

// SliceContainsStringThatTheStringStartsWith checks if a slice contains a string that the given string begins with
func SliceContainsStringThatTheStringStartsWith(s []string, str string) bool {
	for _, v := range s {
		if strings.HasPrefix(str, v) {
			return true
		}
	}
	return false
}
