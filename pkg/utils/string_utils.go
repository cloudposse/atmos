package utils

import (
	"encoding/csv"
	"errors"
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

	read := func(lazy bool) ([]string, error) {
		r := csv.NewReader(strings.NewReader(str))
		r.Comma = delimiter
		r.TrimLeadingSpace = true // Trim leading spaces in fields.
		r.LazyQuotes = lazy
		return r.Read()
	}

	parts, err := read(false)
	if err != nil {
		var parseErr *csv.ParseError
		if errors.As(err, &parseErr) && errors.Is(parseErr.Err, csv.ErrBareQuote) {
			parts, err = read(true)
		}
	}
	if err != nil {
		return nil, err
	}

	// Remove empty strings caused by multiple spaces and trim matching quotes.
	filteredParts := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := trimMatchingQuotes(part)
		if trimmed == "" {
			continue
		}
		filteredParts = append(filteredParts, trimmed)
	}

	return filteredParts, nil
}

// trimMatchingQuotes removes matching leading and trailing quote characters and normalizes any escaped quotes within the value.
func trimMatchingQuotes(value string) string {
	if len(value) < 2 {
		return value
	}

	first := value[0]
	if first != '\'' && first != '"' {
		return value
	}

	if value[len(value)-1] != first {
		return value
	}

	inner := value[1 : len(value)-1]

	switch first {
	case '\'':
		inner = strings.ReplaceAll(inner, "''", "'")
	case '"':
		inner = strings.ReplaceAll(inner, "\"\"", "\"")
	}

	return inner
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
