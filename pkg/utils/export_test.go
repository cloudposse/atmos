package utils

// ResetGlobMatchesCache clears the glob matches sync map.
// This is exported only for testing to avoid data races from direct struct assignment.
func ResetGlobMatchesCache() {
	getGlobMatchesSyncMap.Clear()
}

// ResetPathMatchCache clears the path match cache.
// This is exported only for testing to ensure consistent state between tests.
func ResetPathMatchCache() {
	pathMatchCacheMu.Lock()
	pathMatchCache = make(map[pathMatchKey]bool)
	pathMatchCacheMu.Unlock()
}
