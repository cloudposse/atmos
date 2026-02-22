package ci

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// StatusOptions contains options for fetching CI status.
type StatusOptions = provider.StatusOptions

// Status represents the CI status for display (like gh pr status).
type Status = provider.Status

// BranchStatus contains status information for a specific branch.
type BranchStatus = provider.BranchStatus

// PRStatus contains status information for a pull request.
type PRStatus = provider.PRStatus

// CheckStatus contains status information for a single check.
type CheckStatus = provider.CheckStatus

// CheckStatusState represents the simplified state for display.
type CheckStatusState = provider.CheckStatusState

const (
	// CheckStatusStatePending indicates the check is pending or in progress.
	CheckStatusStatePending = provider.CheckStatusStatePending

	// CheckStatusStateSuccess indicates the check passed.
	CheckStatusStateSuccess = provider.CheckStatusStateSuccess

	// CheckStatusStateFailure indicates the check failed.
	CheckStatusStateFailure = provider.CheckStatusStateFailure

	// CheckStatusStateCancelled indicates the check was cancelled.
	CheckStatusStateCancelled = provider.CheckStatusStateCancelled

	// CheckStatusStateSkipped indicates the check was skipped.
	CheckStatusStateSkipped = provider.CheckStatusStateSkipped
)
