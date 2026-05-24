package github

import (
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

// IsDebugMode reports whether the current GitHub Actions run has runner-level
// or step-level debug logging enabled. Satisfies the optional
// provider.DebugModeDetector interface so generic callers can auto-promote
// their log level without depending on GitHub-specific environment variables.
//
// Returns true when at least one of the documented debug toggles is set to
// the literal string "true":
//
//   - ACTIONS_RUNNER_DEBUG — runner diagnostic logging.
//   - ACTIONS_STEP_DEBUG   — step debug logging.
//
// GITHUB_ACTIONS is also required to be "true" so the method behaves safely
// when invoked outside of a real GHA environment (e.g., direct unit tests).
// In production this is redundant — the provider only registers itself when
// GITHUB_ACTIONS=true (see provider.go).
//
// Reference: https://docs.github.com/actions/how-tos/manage-workflow-runs/enable-debug-logging
func (p *Provider) IsDebugMode() bool {
	defer perf.Track(nil, "github.Provider.IsDebugMode")()

	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return false
	}
	return os.Getenv("ACTIONS_RUNNER_DEBUG") == "true" ||
		os.Getenv("ACTIONS_STEP_DEBUG") == "true"
}
