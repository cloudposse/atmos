package utils

import (
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/cloudposse/atmos/pkg/perf"
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
	defer perf.Track(nil, "utils.MatchWildcard")()

	// Handle empty pattern as match all
	if pattern == "" {
		return true, nil
	}

	// Convert pattern to filepath-style pattern
	pattern = filepath.ToSlash(pattern)
	str = filepath.ToSlash(str)

	return doublestar.PathMatch(pattern, str)
}

// hasGlobMeta reports whether pattern contains a doublestar glob
// metacharacter (`*`, `?`, `[`, or `{`, matching the character set
// MatchWildcard's own doc comment describes).
func hasGlobMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[{")
}

// WildcardRelPath returns path with pattern's literal (non-glob) base
// directory prefix stripped, letting a caller that matched path against a
// directory-level glob (e.g. "components/**") recover the matched file's
// position relative to that glob's static root (e.g. "vpc/main.tf").
//
// Returns path unchanged whenever pattern has no glob metacharacter at all:
// doublestar.SplitPattern always splits off the pattern's final path
// segment regardless of whether that segment (or anything else in the
// pattern) is actually a glob -- e.g. SplitPattern("components/main.tf")
// returns ("components", "main.tf"), not (".", "components/main.tf") --
// so calling it unconditionally would incorrectly strip the directory off
// an entirely ordinary, non-glob path. Guarding on hasGlobMeta first keeps
// RelPath == Path for every literal (non-glob) spec.
func WildcardRelPath(pattern, path string) string {
	defer perf.Track(nil, "utils.WildcardRelPath")()

	path = filepath.ToSlash(path)
	if pattern == "" || !hasGlobMeta(pattern) {
		return path
	}

	base, _ := doublestar.SplitPattern(filepath.ToSlash(pattern))
	if base == "." || base == "" {
		return path
	}
	return strings.TrimPrefix(path, base+"/")
}
