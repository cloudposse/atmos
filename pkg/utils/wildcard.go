package utils

import (
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
)

// MatchWildcard checks if a string matches a wildcard pattern.
// The pattern can include glob patterns:
// - '*' matches any sequence of non-separator characters.
// - '?' matches any single non-separator character.
// - '[abc]' matches any character within the brackets.
// - '[a-z]' matches any character in the range.
// - '**' matches any number of directories or files recursively.
// - '{abc,xyz}` matches the string "abc" or "xyz".
func MatchWildcard(pattern, str string) (bool, error) {
	// Handle empty pattern as match all
	if pattern == "" {
		return true, nil
	}

	// Convert pattern to filepath-style pattern
	pattern = filepath.ToSlash(pattern)
	str = filepath.ToSlash(str)

	return doublestar.PathMatch(pattern, str)
}
