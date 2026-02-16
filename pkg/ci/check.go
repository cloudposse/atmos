package ci

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// CheckRunState represents the state of a check run.
type CheckRunState = provider.CheckRunState

const (
	// CheckRunStatePending indicates the check run has not started.
	CheckRunStatePending = provider.CheckRunStatePending

	// CheckRunStateInProgress indicates the check run is in progress.
	CheckRunStateInProgress = provider.CheckRunStateInProgress

	// CheckRunStateSuccess indicates the check run completed successfully.
	CheckRunStateSuccess = provider.CheckRunStateSuccess

	// CheckRunStateFailure indicates the check run failed.
	CheckRunStateFailure = provider.CheckRunStateFailure

	// CheckRunStateError indicates an error occurred during the check run.
	CheckRunStateError = provider.CheckRunStateError

	// CheckRunStateCancelled indicates the check run was cancelled.
	CheckRunStateCancelled = provider.CheckRunStateCancelled
)

// CheckRun represents a status check on a commit (like Atlantis status checks).
type CheckRun = provider.CheckRun

// CreateCheckRunOptions contains options for creating a new check run.
type CreateCheckRunOptions = provider.CreateCheckRunOptions

// UpdateCheckRunOptions contains options for updating an existing check run.
type UpdateCheckRunOptions = provider.UpdateCheckRunOptions

// FormatCheckRunName creates a standardized check run name for Atmos.
func FormatCheckRunName(action, stack, component string) string {
	return provider.FormatCheckRunName(action, stack, component)
}
