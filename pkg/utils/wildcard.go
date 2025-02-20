package utils

import (
	"path/filepath"
)

// MatchWildcard checks if a string matches a wildcard pattern.
// The pattern can include simple file-style glob patterns:
// - '*' matches any sequence of non-separator characters
// - '?' matches any single non-separator character
// - '[abc]' matches any character within the brackets
// - '[a-z]' matches any character in the range
func MatchWildcard(pattern, str string) (bool, error) {
	// Handle empty pattern as match all
	if pattern == "" {
		return true, nil
	}

	// Convert pattern to filepath-style pattern
	pattern = filepath.ToSlash(pattern)
	str = filepath.ToSlash(str)

	return filepath.Match(pattern, str)
}
