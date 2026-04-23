package utils

// ResetGlobMatchesCache clears the glob matches sync map.
// This is exported only for testing to avoid data races from direct struct assignment.
func ResetGlobMatchesCache() {
	getGlobMatchesSyncMap.Clear()
}

// StoreGlobCacheSentinel stores an arbitrary (possibly non-[]string) value in the glob
// matches sync map under the given (already-normalized) key.  Use this in tests that need
// to exercise the "unexpected cache type" invalidation branch of GetGlobMatches.
func StoreGlobCacheSentinel(pattern string, value any) {
	getGlobMatchesSyncMap.Store(pattern, value)
}

// ResetPathMatchCache clears the path match cache.
// This is exported only for testing to ensure consistent state between tests.
func ResetPathMatchCache() {
	pathMatchCacheMu.Lock()
	pathMatchCache = make(map[pathMatchKey]bool)
	pathMatchCacheMu.Unlock()
}
