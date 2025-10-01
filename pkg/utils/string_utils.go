package utils

import (
	"encoding/csv"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// UniqueStrings returns a unique subset of the string slice provided
func UniqueStrings(input []string) []string {
	defer perf.Track(nil, "utils.UniqueStrings")()

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

// SplitStringByDelimiter splits a string by the delimiter, not splitting inside quotes.
func SplitStringByDelimiter(str string, delimiter rune) ([]string, error) {
	defer perf.Track(nil, "utils.SplitStringByDelimiter")()

	r := csv.NewReader(strings.NewReader(str))
	r.Comma = delimiter
	r.TrimLeadingSpace = true // Trim leading spaces in fields

	parts, err := r.Read()
	if err != nil {
		return nil, err
	}

	// Remove empty strings caused by multiple spaces
	filteredParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filteredParts = append(filteredParts, part)
		}
	}

	return filteredParts, nil
}

// SplitStringAtFirstOccurrence splits a string into two parts at the first occurrence of the separator
func SplitStringAtFirstOccurrence(s string, sep string) [2]string {
	defer perf.Track(nil, "utils.SplitStringAtFirstOccurrence")()

	parts := strings.SplitN(s, sep, 2)
	if len(parts) == 1 {
		return [2]string{parts[0], ""}
	}
	return [2]string{parts[0], parts[1]}
}
