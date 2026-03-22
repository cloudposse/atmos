package utils

// ResetGlobMatchesCache clears the glob matches sync map.
// This is exported only for testing to avoid data races from direct struct assignment.
func ResetGlobMatchesCache() {
	getGlobMatchesSyncMap.Clear()
}
