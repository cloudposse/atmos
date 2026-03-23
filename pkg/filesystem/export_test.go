package filesystem

import (
	"path/filepath"
	"time"
)

// ResetGlobMatchesCache clears the glob matches LRU cache.
// This is exported only for testing to avoid data races from direct struct assignment.
func ResetGlobMatchesCache() {
	globMatchesLRUMu.Lock()
	globMatchesLRU.Purge()
	globMatchesLRUMu.Unlock()
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
