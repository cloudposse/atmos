package utils

import (
	"encoding/csv"
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

// SplitStringByDelimiter splits a string by the delimiter, not splitting inside quotes
func SplitStringByDelimiter(str string, delimiter rune) ([]string, error) {
	r := csv.NewReader(strings.NewReader(str))
	r.Comma = delimiter

	parts, err := r.Read()
	if err != nil {
		return nil, err
	}

	return parts, nil
}

// SplitStringAtFirstOccurrence splits a string into two parts at the first occurrence of the separator
func SplitStringAtFirstOccurrence(s string, sep string) [2]string {
	parts := strings.SplitN(s, sep, 2)
	if len(parts) == 1 {
		return [2]string{parts[0], ""}
	}
	return [2]string{parts[0], parts[1]}
}
