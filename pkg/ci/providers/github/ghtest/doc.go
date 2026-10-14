// Package ghtest is test support for the CI subsystem: a fake GitHub REST API
// that records every write the GitHub CI provider makes, plus a fixture that
// sets the GitHub Actions environment (GITHUB_*) so the real provider can be
// exercised end to end without network access.
//
// This package is a regular (non-_test) package only so tests in other packages
// can import it. It must never be imported by production code.
//
// # Typical use
//
//	s := ghtest.NewServer(t)
//	env := ghtest.SetEnv(t, s, ghtest.WithPullRequest(42, "feature", "main"))
//	ghtest.RegisterProvider(t, github.NewProvider())
//	// ... run code that calls ci.Detect() / ci.ResolveProvider() ...
//	require.Len(t, s.Comments(), 1)
//
// # Import-cycle rules
//
// ghtest imports pkg/ci but deliberately does NOT import pkg/ci/providers/github,
// so both external and in-package tests of the github provider (package github)
// can use it. RegisterProvider therefore receives the provider as an argument.
//
// Tests that live inside package ci (internal tests, "package ci") must not
// import ghtest: ghtest imports ci, which would form an import cycle. Reporter
// tests in pkg/ci that need ghtest must be written as "package ci_test".
package ghtest
