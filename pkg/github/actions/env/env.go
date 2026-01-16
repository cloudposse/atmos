// Package env provides helpers for GitHub Actions environment files.
// It supports reading paths to $GITHUB_OUTPUT, $GITHUB_ENV, $GITHUB_PATH,
// and $GITHUB_STEP_SUMMARY files, plus writing formatted data to them.
package env

import (
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

// IsGitHubActions returns true if running in GitHub Actions.
// It checks for the GITHUB_ACTIONS environment variable set by GitHub.
func IsGitHubActions() bool {
	defer perf.Track(nil, "github.actions.env.IsGitHubActions")()

	// GITHUB_ACTIONS is an external CI environment variable set by GitHub Actions,
	// not an Atmos configuration variable, so os.Getenv is appropriate here.
	//nolint:forbidigo // GITHUB_ACTIONS is an external CI env var, not Atmos config
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// GetOutputPath returns the path to $GITHUB_OUTPUT file, or empty string if not set.
// GITHUB_OUTPUT is used to pass outputs between workflow steps.
func GetOutputPath() string {
	defer perf.Track(nil, "github.actions.env.GetOutputPath")()

	// GITHUB_OUTPUT is an external CI environment variable set by GitHub Actions,
	// not an Atmos configuration variable, so os.Getenv is appropriate here.
	//nolint:forbidigo // GITHUB_OUTPUT is an external CI env var, not Atmos config
	return os.Getenv("GITHUB_OUTPUT")
}

// GetEnvPath returns the path to $GITHUB_ENV file, or empty string if not set.
// GITHUB_ENV is used to set environment variables for subsequent workflow steps.
func GetEnvPath() string {
	defer perf.Track(nil, "github.actions.env.GetEnvPath")()

	// GITHUB_ENV is an external CI environment variable set by GitHub Actions,
	// not an Atmos configuration variable, so os.Getenv is appropriate here.
	//nolint:forbidigo // GITHUB_ENV is an external CI env var, not Atmos config
	return os.Getenv("GITHUB_ENV")
}

// GetPathPath returns the path to $GITHUB_PATH file, or empty string if not set.
// GITHUB_PATH is used to prepend directories to the system PATH for subsequent steps.
func GetPathPath() string {
	defer perf.Track(nil, "github.actions.env.GetPathPath")()

	// GITHUB_PATH is an external CI environment variable set by GitHub Actions,
	// not an Atmos configuration variable, so os.Getenv is appropriate here.
	//nolint:forbidigo // GITHUB_PATH is an external CI env var, not Atmos config
	return os.Getenv("GITHUB_PATH")
}

// GetSummaryPath returns the path to $GITHUB_STEP_SUMMARY file, or empty string if not set.
// GITHUB_STEP_SUMMARY is used to write job summary markdown.
func GetSummaryPath() string {
	defer perf.Track(nil, "github.actions.env.GetSummaryPath")()

	// GITHUB_STEP_SUMMARY is an external CI environment variable set by GitHub Actions,
	// not an Atmos configuration variable, so os.Getenv is appropriate here.
	//nolint:forbidigo // GITHUB_STEP_SUMMARY is an external CI env var, not Atmos config
	return os.Getenv("GITHUB_STEP_SUMMARY")
}
