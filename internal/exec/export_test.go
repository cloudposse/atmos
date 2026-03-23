package exec

// ResetWarnedArchivedReposForTest clears the per-run warning-deduplication map so that
// tests in other packages that call warnIfArchivedGitHubRepo indirectly (e.g., integration
// tests that exercise the full vendor pipeline) can reset state between test runs.
//
// NOTE: test utility — exported via export_test.go so it is only compiled during
// `go test ./internal/exec/...` and is not part of the production binary. Tests within
// this package can call the unexported resetWarnedRepos helper directly; this export is
// only needed if integration tests in other packages ever need to reset the map.
func ResetWarnedArchivedReposForTest() {
	warnedArchivedRepos.Range(func(k, _ any) bool {
		warnedArchivedRepos.Delete(k)
		return true
	})
}
