package filesystem

import (
	"path/filepath"
	"sync/atomic"
	"time"
)

// ResetGlobMatchesCache clears the glob matches LRU cache and resets the eviction counter.
// This is exported only for testing to avoid data races from direct struct assignment.
func ResetGlobMatchesCache() {
	globMatchesLRUMu.Lock()
	globMatchesLRU.Purge()
	globMatchesLRUMu.Unlock()
	atomic.StoreInt64(&globMatchesEvictions, 0)
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
func SetGlobCacheEntryExpired(pattern string) {
	normalizedPattern := filepath.ToSlash(pattern)
	globMatchesLRUMu.Lock()
	if entry, ok := globMatchesLRU.Get(normalizedPattern); ok {
		entry.expiry = time.Time{} // zero time is in the past
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

// GlobCacheEvictions returns the total number of LRU evictions since the last cache reset.
// This counter is incremented atomically by the LRU eviction callback.
func GlobCacheEvictions() int64 {
	return atomic.LoadInt64(&globMatchesEvictions)
}
