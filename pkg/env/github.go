package env

import (
	ghenv "github.com/cloudposse/atmos/pkg/github/actions/env"
	"github.com/cloudposse/atmos/pkg/perf"
)

// IsGitHubActions returns true if running in GitHub Actions.
// It checks for the GITHUB_ACTIONS environment variable set by GitHub.
func IsGitHubActions() bool {
	defer perf.Track(nil, "env.IsGitHubActions")()

	return ghenv.IsGitHubActions()
}

// GetOutputPath returns the path to $GITHUB_OUTPUT file, or empty string if not set.
// GITHUB_OUTPUT is used to pass outputs between workflow steps.
func GetOutputPath() string {
	defer perf.Track(nil, "env.GetOutputPath")()

	return ghenv.GetOutputPath()
}

// GetEnvPath returns the path to $GITHUB_ENV file, or empty string if not set.
// GITHUB_ENV is used to set environment variables for subsequent workflow steps.
func GetEnvPath() string {
	defer perf.Track(nil, "env.GetEnvPath")()

	return ghenv.GetEnvPath()
}

// GetPathPath returns the path to $GITHUB_PATH file, or empty string if not set.
// GITHUB_PATH is used to prepend directories to the system PATH for subsequent steps.
func GetPathPath() string {
	defer perf.Track(nil, "env.GetPathPath")()

	return ghenv.GetPathPath()
}

// GetSummaryPath returns the path to $GITHUB_STEP_SUMMARY file, or empty string if not set.
// GITHUB_STEP_SUMMARY is used to write job summary markdown.
func GetSummaryPath() string {
	defer perf.Track(nil, "env.GetSummaryPath")()

	return ghenv.GetSummaryPath()
}
