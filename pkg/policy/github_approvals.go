package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// GitHubApprovalSpec uses explicitly maintained logins, not live organization teams.
type GitHubApprovalSpec struct {
	Reviewers []string `json:"reviewers"`
	Minimum   int      `json:"minimum"`
}

// GitHubReview retains the provider fields required for review verification.
type GitHubReview struct {
	ID          int64     `json:"id"`
	Login       string    `json:"login"`
	State       string    `json:"state"`
	CommitID    string    `json:"commit_id"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// ParseGitHubApproval validates the supported provider-native approval contract.
func ParseGitHubApproval(requirement ApprovalRequirement) (GitHubApprovalSpec, error) {
	defer perf.Track(nil, "policy.ParseGitHubApproval")()
	var spec GitHubApprovalSpec
	decoder := json.NewDecoder(bytes.NewReader(requirement.Spec))
	decoder.DisallowUnknownFields()
	if requirement.ID == "" || requirement.Provider != "github-actions" || decoder.Decode(&spec) != nil {
		return spec, fmt.Errorf("%w: approval requires id, github-actions provider, and reviewer spec", errUtils.ErrPolicyInvalid)
	}
	seen := map[string]bool{}
	for _, login := range spec.Reviewers {
		key := strings.ToLower(login)
		if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,38}$`).MatchString(login) || seen[key] {
			return spec, fmt.Errorf("%w: reviewers must be unique GitHub user logins, not teams", errUtils.ErrPolicyInvalid)
		}
		seen[key] = true
	}
	if spec.Minimum < 1 || spec.Minimum > len(seen) {
		return spec, fmt.Errorf("%w: minimum must be between one and the number of reviewers", errUtils.ErrPolicyInvalid)
	}
	return spec, nil
}

// ResolveGitHubApprovals is pure; the caller must supply complete, freshly fetched
// reviews bound to the current open PR and the revision being executed.
// The latest decisive review wins. Comments never grant or revoke approval.
func ResolveGitHubApprovals(decision Decision, head, author string, reviews []GitHubReview) (Decision, error) {
	defer perf.Track(nil, "policy.ResolveGitHubApprovals")()
	if decision.Status != AwaitingApproval {
		return decision, nil
	}
	if head == "" || author == "" {
		return decision, errUtils.ErrPolicyInputUnavailable
	}
	latest, err := latestDecisiveGitHubReviews(reviews)
	if err != nil {
		return decision, err
	}
	result, err := cloneJSON(decision)
	if err != nil {
		return decision, err
	}
	result.Status = NotApplicable
	for i := range result.Findings {
		finding := &result.Findings[i]
		if err := resolveGitHubFindingApprovals(finding, latest, head, author); err != nil {
			return decision, err
		}
		result.Status = dominantStatus(result.Status, finding.Status)
	}
	return result, nil
}

func latestDecisiveGitHubReviews(reviews []GitHubReview) (map[string]GitHubReview, error) {
	latest := map[string]GitHubReview{}
	seen := map[int64]bool{}
	for _, review := range reviews {
		if review.ID <= 0 || review.Login == "" || seen[review.ID] {
			return nil, errUtils.ErrPolicyInputUnavailable
		}
		seen[review.ID] = true
		switch review.State {
		case "COMMENTED", "PENDING":
			continue
		case "APPROVED", "CHANGES_REQUESTED", "DISMISSED":
		default:
			return nil, errUtils.ErrPolicyInputUnavailable
		}
		if review.SubmittedAt.IsZero() {
			return nil, errUtils.ErrPolicyInputUnavailable
		}
		key := strings.ToLower(review.Login)
		previous, exists := latest[key]
		if shouldReplaceGitHubReview(&previous, &review, exists) {
			latest[key] = review
		}
	}
	return latest, nil
}

func shouldReplaceGitHubReview(previous, current *GitHubReview, hasPrevious bool) bool {
	return !hasPrevious || previous.SubmittedAt.Before(current.SubmittedAt) ||
		(previous.SubmittedAt.Equal(current.SubmittedAt) && previous.ID < current.ID)
}

func resolveGitHubFindingApprovals(finding *Finding, latest map[string]GitHubReview, head, author string) error {
	if finding.Status != AwaitingApproval {
		return nil
	}
	var outstanding []ApprovalRequirement
	for _, requirement := range finding.Effects.Approvals {
		spec, err := ParseGitHubApproval(requirement)
		if err != nil {
			return err
		}
		if countGitHubApprovals(spec.Reviewers, latest, head, author) < spec.Minimum {
			outstanding = append(outstanding, requirement)
		}
	}
	finding.Effects.Approvals = outstanding
	if len(outstanding) == 0 {
		finding.Status = Allow
	}
	return nil
}

func countGitHubApprovals(reviewers []string, latest map[string]GitHubReview, head, author string) int {
	count := 0
	for _, login := range reviewers {
		review := latest[strings.ToLower(login)]
		if !strings.EqualFold(login, author) && review.State == "APPROVED" && review.CommitID == head {
			count++
		}
	}
	return count
}
