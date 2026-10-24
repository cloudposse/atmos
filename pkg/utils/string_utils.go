package utils

import (
	"fmt"
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
//   - YAML keys: "vars", "settings", "metadata", "env", "backend", "terraform", "helmfile".
//   - Stack names, component names, file paths.
//   - Common values: "true", "false", "default", region names.
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

// FormatList formats a list of strings into a markdown bullet list.
// Each item is formatted as a backtick-quoted bullet point.
func FormatList(items []string) string {
	defer perf.Track(nil, "utils.FormatList")()

	var result strings.Builder
	for _, item := range items {
		result.WriteString(fmt.Sprintf("- `%s`\n", item))
	}
	return result.String()
}
