package utils

import (
	"strings"
)

// UniqueStrings returns a unique subset of the string slice provided
func UniqueStrings(input []string) []string {
	u := make([]string, 0, len(input))
	m := make(map[string]bool)

	for _, val := range input {
		if _, ok := m[val]; !ok {
			m[val] = true
			u = append(u, val)
		}
	}

	return u
}

func RemoveWhitespace(input string) string {
	replaceable := [...]string{"\t", "\n", " "}

	for _, item := range replaceable {
		input = strings.Replace(input, item, "", -1)
	}

	return input
}
