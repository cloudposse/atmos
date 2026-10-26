package cache

import "fmt"

const (
	// Structured-log field name for a cache key.
	fieldKey = "key"

	// Permission for files written during extraction.
	defaultFilePerm = 0o644

	// Number of hex characters kept from content hashes.
	hashPrefixLen = 16
)

// wrapErr wraps a cause with a static sentinel error, preserving both chains.
func wrapErr(sentinel, cause error) error {
	return fmt.Errorf("%w: %w", sentinel, cause)
}
