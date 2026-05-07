package provider

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// StatusOptions contains options for fetching CI status.
type StatusOptions struct {
	// Owner is the repository owner.
	Owner string

	// Repo is the repository name.
	Repo string

	// Branch is the branch to check (optional, defaults to current branch).
	Branch string

	// SHA is the commit SHA to check (optional, defaults to HEAD).
	SHA string

	// IncludeUserPRs includes PRs created by the authenticated user.
	IncludeUserPRs bool

	// IncludeReviewRequests includes PRs requesting review from the user.
	IncludeReviewRequests bool
}

// Status represents the CI status for display (like gh pr status).
type Status struct {
	// Repository is the full repository name (e.g., "owner/repo").
	Repository string

	// CurrentBranch contains status for the current branch.
	CurrentBranch *BranchStatus

	// CreatedByUser contains PRs created by the authenticated user.
	CreatedByUser []*PRStatus

	// ReviewRequests contains PRs requesting review from the user.
	ReviewRequests []*PRStatus
}

// BranchStatus contains status information for a specific branch.
type BranchStatus struct {
	// Branch is the branch name.
	Branch string

	// PullRequest is the PR associated with this branch (nil if none).
	PullRequest *PRStatus

	// CommitSHA is the HEAD commit SHA.
	CommitSHA string

	// Checks are the status checks for this branch/commit.
	Checks []*CheckStatus
}

// PRStatus contains status information for a pull request.
type PRStatus struct {
	// Number is the PR number.
	Number int

	// Title is the PR title.
	Title string

	// Branch is the head branch name.
	Branch string

	// BaseBranch is the target branch name.
	BaseBranch string

	// URL is the PR URL.
	URL string

	// Checks are the status checks for this PR.
	Checks []*CheckStatus

	// AllPassed is true if all checks have passed.
	AllPassed bool
}

// CheckStatus contains status information for a single check.
type CheckStatus struct {
	// Name is the check name.
	Name string

	// Status is the check status (e.g., "queued", "in_progress", "completed").
	Status string

	// Conclusion is the check conclusion (e.g., "success", "failure", "neutral").
	Conclusion string

	// DetailsURL is a link to the check details.
	DetailsURL string
}

// CheckState returns the simplified state for display.
func (c *CheckStatus) CheckState() CheckStatusState {
	defer perf.Track(nil, "provider.CheckStatus.CheckState")()

	switch c.Status {
	case "queued", "in_progress", "pending":
		return CheckStatusStatePending
	case "completed":
		switch c.Conclusion {
		case "success":
			return CheckStatusStateSuccess
		case "failure", "timed_out", "action_required":
			return CheckStatusStateFailure
		case "cancelled":
			return CheckStatusStateCancelled
		case "skipped", "neutral":
			return CheckStatusStateSkipped
		default:
			return CheckStatusStatePending
		}
	default:
		return CheckStatusStatePending
	}
}

// CheckStatusState represents the simplified state for display.
type CheckStatusState string

const (
	// CheckStatusStatePending indicates the check is pending or in progress.
	CheckStatusStatePending CheckStatusState = "pending"

	// CheckStatusStateSuccess indicates the check passed.
	CheckStatusStateSuccess CheckStatusState = "success"

	// CheckStatusStateFailure indicates the check failed.
	CheckStatusStateFailure CheckStatusState = "failure"

	// CheckStatusStateCancelled indicates the check was cancelled.
	CheckStatusStateCancelled CheckStatusState = "cancelled"

	// CheckStatusStateSkipped indicates the check was skipped.
	CheckStatusStateSkipped CheckStatusState = "skipped"
)
