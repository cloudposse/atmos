package retry

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrInvalidCondition is returned when a retry condition pattern fails to compile.
var ErrInvalidCondition = errors.New("invalid retry condition pattern")

// CompileConditions compiles a slice of regex pattern strings into a slice of *regexp.Regexp.
// Patterns may be wrapped in optional /.../ delimiters for readability — surrounding slashes
// are stripped before compilation. An empty or nil input returns (nil, nil).
// A pattern that fails to compile produces an error joined with ErrInvalidCondition.
func CompileConditions(patterns []string) ([]*regexp.Regexp, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, raw := range patterns {
		stripped := stripSlashDelimiters(raw)
		if stripped == "" {
			continue
		}
		re, err := regexp.Compile(stripped)
		if err != nil {
			return nil, errors.Join(ErrInvalidCondition, fmt.Errorf("pattern %q: %w", raw, err))
		}
		compiled = append(compiled, re)
	}
	return compiled, nil
}

// MatchesAny reports whether any of the compiled patterns matches the given output.
// Returns false when patterns is empty or output is empty.
func MatchesAny(patterns []*regexp.Regexp, output string) bool {
	if len(patterns) == 0 || output == "" {
		return false
	}
	for _, re := range patterns {
		if re.MatchString(output) {
			return true
		}
	}
	return false
}

// stripSlashDelimiters removes a surrounding pair of `/` characters from a pattern string.
// This lets users write patterns like `/Bad Gateway/` in YAML for readability.
// Whitespace around the pattern is trimmed first. If the input is not wrapped in slashes
// (or is a single `/`), it is returned trimmed but unchanged.
func stripSlashDelimiters(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && strings.HasPrefix(s, "/") && strings.HasSuffix(s, "/") {
		return s[1 : len(s)-1]
	}
	return s
}
