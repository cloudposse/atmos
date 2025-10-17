package utils

import (
	"encoding/csv"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// UniqueStrings returns a unique subset of the string slice provided.
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

// SplitStringAtFirstOccurrence splits a string into two parts at the first occurrence of the separator.
func SplitStringAtFirstOccurrence(s string, sep string) [2]string {
	defer perf.Track(nil, "utils.SplitStringAtFirstOccurrence")()

	parts := strings.SplitN(s, sep, 2)
	if len(parts) == 1 {
		return [2]string{parts[0], ""}
	}
	return [2]string{parts[0], parts[1]}
}

// String interning pool for deduplicating common strings.
// This saves memory by ensuring duplicate strings share the same underlying storage.
var (
	// The internPool stores interned strings using sync.Map for thread-safe concurrent access.
	internPool sync.Map

	// Atomic counters for string interning statistics (lock-free for high performance).
	internStatsRequests atomic.Int64 // Total intern requests.
	internStatsHits     atomic.Int64 // Cache hits (string already interned).
	internStatsMisses   atomic.Int64 // Cache misses (new string added).
	internStatsSavedMem atomic.Int64 // Estimated memory saved (bytes).
)

// Intern returns a canonical representation of the string.
// If the string already exists in the intern pool, returns the existing instance.
// Otherwise, adds the string to the pool and returns it.
// This reduces memory usage by deduplicating common strings across YAML files.
//
// Common interned strings in Atmos:
//   - YAML keys: "vars", "settings", "metadata", "env", "backend", "terraform", "helmfile"
//   - Stack names, component names, file paths
//   - Common values: "true", "false", "default", region names
//
// Thread-safe for concurrent use.
// Note: perf.Track removed from this critical path function as it's called millions of times.
// Statistics use atomic operations instead of locks to avoid contention in the hot path.
func Intern(_ *schema.AtmosConfiguration, s string) string {
	// Empty strings are not interned.
	if s == "" {
		return s
	}

	internStatsRequests.Add(1)

	// Fast path: check if string is already interned.
	if existing, ok := internPool.Load(s); ok {
		internStatsHits.Add(1)
		// Track memory saved (approximate: deduplicated string data length).
		internStatsSavedMem.Add(int64(len(s)))
		return existing.(string)
	}

	// Slow path: intern the string.
	// Use LoadOrStore to handle race conditions where another goroutine
	// might have interned the same string while we were checking.
	actual, loaded := internPool.LoadOrStore(s, s)

	if loaded {
		// Another goroutine beat us to it.
		internStatsHits.Add(1)
		// Track memory saved (approximate: deduplicated string data length).
		internStatsSavedMem.Add(int64(len(s)))
	} else {
		// We successfully added a new string.
		internStatsMisses.Add(1)
	}

	return actual.(string)
}

// InternSlice interns all strings in a slice.
// Returns a new slice with interned strings.
// Note: perf.Track removed to avoid overhead on frequently-called function.
func InternSlice(atmosConfig *schema.AtmosConfiguration, strings []string) []string {
	if len(strings) == 0 {
		return strings
	}

	result := make([]string, len(strings))
	for i, s := range strings {
		result[i] = Intern(atmosConfig, s)
	}
	return result
}

// InternMapKeys interns all keys in a map.
// Returns a new map with interned keys and original values.
// Note: Values are not interned as they may be of various types.
// Note: perf.Track removed to avoid overhead on frequently-called function.
func InternMapKeys(atmosConfig *schema.AtmosConfiguration, m map[string]any) map[string]any {
	if len(m) == 0 {
		return m
	}

	result := make(map[string]any, len(m))
	for k, v := range m {
		result[Intern(atmosConfig, k)] = v
	}
	return result
}

// InternStats represents string interning statistics.
type InternStats struct {
	Requests   int64 // Total intern requests.
	Hits       int64 // Cache hits (string already interned).
	Misses     int64 // Cache misses (new string added).
	SavedBytes int64 // Estimated memory saved (bytes).
}

// GetInternStats returns current interning statistics.
// Useful for debugging and performance analysis.
// Uses atomic loads for lock-free access.
func GetInternStats() InternStats {
	return InternStats{
		Requests:   internStatsRequests.Load(),
		Hits:       internStatsHits.Load(),
		Misses:     internStatsMisses.Load(),
		SavedBytes: internStatsSavedMem.Load(),
	}
}

// ResetInternStats resets interning statistics.
// Primarily for testing.
// Uses atomic stores for lock-free access.
func ResetInternStats() {
	internStatsRequests.Store(0)
	internStatsHits.Store(0)
	internStatsMisses.Store(0)
	internStatsSavedMem.Store(0)
}

// ClearInternPool clears the intern pool.
// Should only be used in tests.
func ClearInternPool() {
	internPool.Range(func(key, value interface{}) bool {
		internPool.Delete(key)
		return true
	})
	ResetInternStats()
}
