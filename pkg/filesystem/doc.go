// Package filesystem provides file-system utilities for the Atmos CLI, including
// atomic file writes (POSIX rename and a Windows-compatible remove-before-rename
// variant) and glob-pattern matching with a bounded, time-limited LRU cache.
//
// # GetGlobMatches contract
//
// [GetGlobMatches] returns a non-nil []string when err == nil.  An empty result
// set is returned as []string{}, never nil.  On error, the returned slice is nil.
// Callers must check err first, then use len(result) == 0, not result == nil.
//
// # Cache configuration
//
// The glob LRU cache is configurable at startup via environment variables:
//
//   - ATMOS_FS_GLOB_CACHE_MAX_ENTRIES – maximum number of cached patterns
//     (default: 1024, minimum: 16).  Positive values below 16 are clamped up
//     to 16.  Zero, negative, or unparseable values
//     fall back to the default (1024).
//   - ATMOS_FS_GLOB_CACHE_TTL – TTL per entry as a Go duration string, e.g.
//     "10m" (default: 5m, minimum: 1s).  Positive durations below 1s are
//     clamped up to 1s.  Zero, negative, or unparseable values fall back to
//     the default (5m).
//   - ATMOS_FS_GLOB_CACHE_EMPTY – controls caching of empty (no-match) results.
//     Set to "0" or "false" to disable; any other value (including the default
//     unset state) enables empty-result caching.
//
// # Observability
//
// Three atomic int64 counters track cache activity:
//   - hits, misses, evictions (accessible via [GlobCacheHits], [GlobCacheMisses],
//     [GlobCacheEvictions] in tests).
//
// Call [RegisterGlobCacheExpvars] once at startup to expose these counters via
// the expvar /debug/vars HTTP endpoint:
//
//	filesystem.RegisterGlobCacheExpvars()
package filesystem
