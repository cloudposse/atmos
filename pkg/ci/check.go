package ci

import (
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// CheckRunState represents the state of a check run.
type CheckRunState string

const (
	// CheckRunStatePending indicates the check run has not started.
	CheckRunStatePending CheckRunState = "pending"

	// CheckRunStateInProgress indicates the check run is in progress.
	CheckRunStateInProgress CheckRunState = "in_progress"

	// CheckRunStateSuccess indicates the check run completed successfully.
	CheckRunStateSuccess CheckRunState = "success"

	// CheckRunStateFailure indicates the check run failed.
	CheckRunStateFailure CheckRunState = "failure"

	// CheckRunStateError indicates an error occurred during the check run.
	CheckRunStateError CheckRunState = "error"

	// CheckRunStateCancelled indicates the check run was cancelled.
	CheckRunStateCancelled CheckRunState = "cancelled"
)

// CheckRun represents a status check on a commit (like Atlantis status checks).
type CheckRun struct {
	// ID is the unique identifier for this check run.
	ID int64

	// Name is the check run name (e.g., "atmos/plan: plat-ue2-dev/vpc").
	Name string

	// Status is the current state of the check run.
	Status CheckRunState

	// Conclusion is the final conclusion (success, failure, etc.).
	Conclusion string

	// Title is a short title for the check run output.
	Title string

	// Summary is a markdown summary of the check run results.
	Summary string

	// DetailsURL is a link back to the CI run details.
	DetailsURL string

	// StartedAt is when the check run started.
	StartedAt time.Time

	// CompletedAt is when the check run completed.
	CompletedAt time.Time
}

// CreateCheckRunOptions contains options for creating a new check run.
type CreateCheckRunOptions struct {
	// Owner is the repository owner.
	Owner string

	// Repo is the repository name.
	Repo string

	// SHA is the commit SHA to create the check run on.
	SHA string

	// Name is the check run name (e.g., "atmos/plan: plat-ue2-dev/vpc").
	Name string

	// Status is the initial status (typically "queued" or "in_progress").
	Status CheckRunState

	// Title is a short title for the check run output.
	Title string

	// Summary is a markdown summary of the check run.
	Summary string

	// DetailsURL is a link back to the CI run.
	DetailsURL string

	// ExternalID is an optional external reference ID.
	ExternalID string
}

// UpdateCheckRunOptions contains options for updating an existing check run.
type UpdateCheckRunOptions struct {
	// Owner is the repository owner.
	Owner string

	// Repo is the repository name.
	Repo string

	// CheckRunID is the ID of the check run to update.
	CheckRunID int64

	// Name is the check run name (required for GitHub API updates).
	Name string

	// Status is the new status.
	Status CheckRunState

	// Conclusion is the final conclusion (required when status is "completed").
	Conclusion string

	// Title is the output title (distinct from the check run name).
	Title string

	// Summary is an updated markdown summary.
	Summary string

	// CompletedAt is when the check run completed.
	CompletedAt *time.Time
}

// FormatCheckRunName creates a standardized check run name for Atmos.
func FormatCheckRunName(action, stack, component string) string {
	defer perf.Track(nil, "ci.FormatCheckRunName")()

	return "atmos/" + action + ": " + stack + "/" + component
}
