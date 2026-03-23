// Package http provides a configurable HTTP client with GitHub authentication support.
//
// # Creating a client
//
// Use [NewDefaultClient] with functional options to configure the client:
//
//	client := http.NewDefaultClient(
//	    http.WithTimeout(30 * time.Second),
//	    http.WithGitHubToken("mytoken"),
//	)
//
// # Option ordering
//
// The order in which options are applied matters when composing transport layers.
// Understanding the ordering rules prevents subtle authentication bugs.
//
// ## WithGitHubToken and WithTransport
//
// [WithGitHubToken] wraps the current transport in a [GitHubAuthenticatedTransport].
// [WithTransport] sets a new base transport. When applied after [WithGitHubToken],
// [WithTransport] replaces the base (inner) transport while preserving the auth wrapper:
//
//	// Recommended: set transport first, then add authentication.
//	client := http.NewDefaultClient(
//	    http.WithTransport(myCustomTransport), // base transport
//	    http.WithGitHubToken("token"),          // wraps myCustomTransport
//	)
//
//	// Also valid: apply in reverse order — WithTransport after WithGitHubToken
//	// replaces the base of the existing GitHubAuthenticatedTransport.
//	client := http.NewDefaultClient(
//	    http.WithGitHubToken("token"),          // wraps http.DefaultTransport
//	    http.WithTransport(myCustomTransport),   // updates base to myCustomTransport
//	)
//
// Triple-composition note: adding a second [WithTransport] after [WithGitHubToken] +
// the first [WithTransport] silently discards the first base transport:
//
//	// t1 is discarded; result is GitHubAuthenticatedTransport{Base: t2, Token: "x"}
//	client := http.NewDefaultClient(
//	    http.WithTransport(t1),
//	    http.WithGitHubToken("x"),
//	    http.WithTransport(t2), // t1 is gone
//	)
//
// ## WithGitHubHostMatcher
//
// [WithGitHubHostMatcher] must be applied AFTER [WithGitHubToken], because the host
// matcher is stored on the [GitHubAuthenticatedTransport] which is only created by
// [WithGitHubToken]. Applying it before [WithGitHubToken] has no effect.
//
//	// Correct: token first, then custom host matcher.
//	client := http.NewDefaultClient(
//	    http.WithGitHubToken("mytoken"),
//	    http.WithGitHubHostMatcher(func(host string) bool {
//	        return host == "github.mycorp.example.com" // GHES
//	    }),
//	)
//
//	// Incorrect: matcher applied before token — has no effect.
//	client := http.NewDefaultClient(
//	    http.WithGitHubHostMatcher(...), // no-op: no transport yet
//	    http.WithGitHubToken("mytoken"),
//	)
//
// ## GitHub Enterprise Server (GHES)
//
// For GHES deployments set the GITHUB_API_URL environment variable to the GHES API
// base URL. The default host matcher ([isGitHubHost]) reads this variable and treats
// the configured hostname as an additional allowed host:
//
//	GITHUB_API_URL=https://github.mycorp.example.com
//
// Alternatively, use [WithGitHubHostMatcher] for programmatic control.
//
// # Host-matcher precedence
//
// The host predicate used to decide whether to inject Authorization follows this
// precedence order (highest to lowest):
//
//  1. [WithGitHubHostMatcher] — an explicit custom predicate always wins.
//  2. GITHUB_API_URL — when set and [WithGitHubHostMatcher] was NOT applied,
//     the GHES hostname from the environment variable is added to the allowlist.
//  3. Built-in allowlist — api.github.com, raw.githubusercontent.com, uploads.github.com.
//
// If you need GHES support together with a custom matcher, include the GHES host
// in your custom predicate; [WithGitHubHostMatcher] bypasses the GITHUB_API_URL lookup:
//
//	ghesHost := "github.mycorp.example.com"
//	client := http.NewDefaultClient(
//	    http.WithGitHubToken("mytoken"),
//	    http.WithGitHubHostMatcher(func(host string) bool {
//	        return host == "api.github.com" || host == ghesHost
//	    }),
//	)
//
// # Security notes
//
// Authorization headers are only injected when ALL of the following are true:
//   - The request URL scheme is "https".
//   - The request hostname matches the host predicate.
//   - The Authorization header is not already set on the request.
//
// Cross-host redirects: [WithGitHubToken] installs a CheckRedirect handler that
// strips the Authorization header from redirect requests that target a different
// host:port, preventing token leakage via open redirects.
//
// This prevents accidental token leakage over unencrypted HTTP and ensures
// that caller-supplied Authorization headers are never overwritten.
package http
