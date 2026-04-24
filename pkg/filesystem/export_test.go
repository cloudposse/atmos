package filesystem

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// ResetGlobMatchesCache clears the glob matches LRU cache and resets all counters.
// This is exported only for testing to avoid data races from direct struct assignment.
func ResetGlobMatchesCache() {
	globMatchesLRUMu.Lock()
	globMatchesLRU.Purge()
	globMatchesLRUMu.Unlock()
	atomic.StoreInt64(&globMatchesEvictions, 0)
	atomic.StoreInt64(&globMatchesHits, 0)
	atomic.StoreInt64(&globMatchesMisses, 0)
}

// ResetPathMatchCache clears the path match cache.
// This is exported only for testing to ensure consistent state between tests.
func ResetPathMatchCache() {
	pathMatchCacheMu.Lock()
	pathMatchCache = make(map[pathMatchKey]bool)
	pathMatchCacheMu.Unlock()
}

// SetGlobCacheEntryExpired forcibly marks a cache entry as expired for testing TTL eviction.
// It re-adds the entry with an expiry in the past, simulating TTL expiry.
// Uses Peek (not Get) so this helper does not promote the entry to MRU before the Add,
// keeping the test-only "force expire" semantics side-effect free for recency.
func SetGlobCacheEntryExpired(pattern string) {
	normalizedPattern := filepath.ToSlash(pattern)
	globMatchesLRUMu.Lock()
	if entry, ok := globMatchesLRU.Peek(normalizedPattern); ok {
		entry.expiry = time.Time{} // zero time is in the past.
		globMatchesLRU.Add(normalizedPattern, entry)
	}
	globMatchesLRUMu.Unlock()
}

// GlobCacheLen returns the number of entries currently in the glob LRU cache.
func GlobCacheLen() int {
	globMatchesLRUMu.RLock()
	defer globMatchesLRUMu.RUnlock()
	return globMatchesLRU.Len()
}

// GlobCacheContains reports whether the given pattern currently has an entry in
// the glob LRU cache (regardless of TTL expiry).  It is intended for use in
// tests that need to verify whether a specific key was evicted.
func GlobCacheContains(pattern string) bool {
	normalizedPattern := filepath.ToSlash(pattern)
	globMatchesLRUMu.RLock()
	defer globMatchesLRUMu.RUnlock()
	return globMatchesLRU.Contains(normalizedPattern)
}

// GlobCacheEvictions returns the total number of LRU evictions since the last cache reset.
// This counter is incremented atomically by the LRU eviction callback.
func GlobCacheEvictions() int64 {
	return atomic.LoadInt64(&globMatchesEvictions)
}

// GlobCacheHits returns the total number of cache hits since the last cache reset.
func GlobCacheHits() int64 {
	return atomic.LoadInt64(&globMatchesHits)
}

// GlobCacheMisses returns the total number of cache misses since the last cache reset.
func GlobCacheMisses() int64 {
	return atomic.LoadInt64(&globMatchesMisses)
}

// ApplyGlobCacheConfigForTest re-reads ATMOS_FS_GLOB_CACHE_* env vars and reinitializes
// the glob LRU cache.  Tests should call this after setting env vars via t.Setenv.
// It also resets all counters so tests start from a clean baseline.
func ApplyGlobCacheConfigForTest() {
	applyGlobCacheConfig()
	atomic.StoreInt64(&globMatchesEvictions, 0)
	atomic.StoreInt64(&globMatchesHits, 0)
	atomic.StoreInt64(&globMatchesMisses, 0)
}

// GlobCacheEmptyEnabled returns the current empty-result caching setting.
func GlobCacheEmptyEnabled() bool {
	globMatchesLRUMu.RLock()
	defer globMatchesLRUMu.RUnlock()
	return globCacheEmptyEnabled
}

// ResetGlobExpvarOnce resets the sync.Once guard so RegisterGlobCacheExpvars
// can be called again in the same test binary.  Only for use in tests that
// need to verify expvar registration after a cache reset.
func ResetGlobExpvarOnce() {
	globExpvarOnce = sync.Once{}
}

// GlobCacheTTL returns the currently active cache TTL for test introspection.
func GlobCacheTTL() time.Duration {
	globMatchesLRUMu.RLock()
	defer globMatchesLRUMu.RUnlock()
	return globCacheTTL
}

// GlobCacheMaxEntries returns the currently configured LRU capacity for test introspection.
func GlobCacheMaxEntries() int {
	globMatchesLRUMu.RLock()
	defer globMatchesLRUMu.RUnlock()
	return globCacheMaxEntries
}
