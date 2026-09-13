package tests

// gitHubTokenEnvVars lists every environment variable atmos's own token resolution
// (pkg/downloader/custom_git_detector.go's resolveToken) checks for a GitHub credential to
// inject into a git URL; effectiveGitHubTokens walks exactly this list so it stays in lockstep
// with what atmos itself would actually pick up.
var gitHubTokenEnvVars = []string{"GITHUB_TOKEN", "ATMOS_GITHUB_TOKEN", "ATMOS_PRO_GITHUB_TOKEN"}

// effectiveGitHubTokens returns the deduplicated set of GitHub tokens atmos could actually inject
// into a git URL for one test case: whichever of GITHUB_TOKEN, ATMOS_GITHUB_TOKEN, and
// ATMOS_PRO_GITHUB_TOKEN the fixture's own tc.Env sets, unioned with whatever the ambient test
// process already has for those same names (via lookup, typically os.Getenv).
//
// This union matters because the git mirror's insteadOf rules only rewrite a URL carrying a token
// they were written for (see gitmirror.WriteGitConfig -- insteadOf is a literal string-prefix
// match, not URL-aware). TestMain registers only the ambient process tokens; a fixture that sets
// one of these three vars to its own value (e.g. a literal test string) makes atmos inject a
// token none of those rules match, so an unforced GitHub git source would fall through to neither
// the token-specific rules nor the anonymous rule and silently go live. Callers use this function
// to detect that gap and extend the mirror config with a test-scoped rule for the fixture's own
// token before running the command (see runCLICommandTest).
func effectiveGitHubTokens(tcEnv map[string]string, lookup func(string) string) []string {
	seen := make(map[string]struct{}, len(gitHubTokenEnvVars)*2)
	var tokens []string

	add := func(token string) {
		if token == "" {
			return
		}
		if _, ok := seen[token]; ok {
			return
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}

	// Ambient process tokens come first, matching the order TestMain registers them in.
	for _, name := range gitHubTokenEnvVars {
		add(lookup(name))
	}
	// Then any fixture-defined value not already covered by an ambient token.
	for _, name := range gitHubTokenEnvVars {
		add(tcEnv[name])
	}

	return tokens
}
