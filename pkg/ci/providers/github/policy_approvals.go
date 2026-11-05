package github

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	gh "github.com/google/go-github/v59/github"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy"
)

const (
	approvalReviewPageLimit = 100
	approvalReviewsPerPage  = 100
)

type pullRequestApprovalTarget struct {
	owner  string
	repo   string
	number int
	head   string
}

// ResolvePolicyApprovals reads PR details and every review using pull-requests: read.
// It never infers approval from a webhook, comments, author association, or team names.
func (p *Provider) ResolvePolicyApprovals(ctx context.Context, request *policy.Request, decision policy.Decision) (policy.Decision, error) {
	defer perf.Track(nil, "github.Provider.ResolvePolicyApprovals")()
	if decision.Status != policy.AwaitingApproval {
		return decision, nil
	}
	evidence, err := p.verifiedPolicyReviews(ctx, request)
	if err != nil {
		return decision, err
	}
	return policy.ResolveGitHubApprovals(decision, evidence.head, evidence.author, evidence.reviews)
}

// CollectPolicyApprovers uses the same fresh, checkout-bound evidence as approval obligations.
func (p *Provider) CollectPolicyApprovers(ctx context.Context, request *policy.Request) (policy.Dataset, error) {
	defer perf.Track(nil, "github.Provider.CollectPolicyApprovers")()
	evidence, err := p.verifiedPolicyReviews(ctx, request)
	if err != nil {
		return policy.Dataset{}, err
	}
	names, err := policy.GitHubApprovers(evidence.head, evidence.author, evidence.reviews)
	if err != nil {
		return policy.Dataset{}, err
	}
	return policy.ApproverDataset(names, policy.Provenance{Source: "github-actions:pull-requests/reviews", CollectedAt: time.Now().UTC(), Revision: evidence.head}), nil
}

type verifiedReviews struct {
	head, author string
	reviews      []policy.GitHubReview
}

func (p *Provider) verifiedPolicyReviews(ctx context.Context, request *policy.Request) (*verifiedReviews, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	target, err := approvalTarget(request)
	if err != nil {
		return nil, err
	}
	if err := p.ensureClient(); err != nil {
		return nil, approvalUnavailable()
	}
	pr, _, err := p.client.client.PullRequests.Get(ctx, target.owner, target.repo, target.number)
	if err != nil || !approvalPRMatches(pr, target) {
		return nil, approvalUnavailable()
	}
	if err := validateApprovalCheckout(request, pr, target.head); err != nil {
		return nil, err
	}
	reviews, err := p.collectApprovalReviews(ctx, target.owner, target.repo, target.number)
	if err != nil {
		return nil, err
	}
	// Detect pushes/base changes during pagination. No API offers an atomic
	// approval-and-execute transaction; the snapshot is checked at each gate.
	after, _, err := p.client.client.PullRequests.Get(ctx, target.owner, target.repo, target.number)
	if err != nil || !approvalRevisionUnchanged(pr, after, target.head) {
		return nil, approvalUnavailable()
	}
	return &verifiedReviews{head: target.head, author: pr.GetUser().GetLogin(), reviews: reviews}, nil
}

func approvalPRMatches(pr *gh.PullRequest, target pullRequestApprovalTarget) bool {
	return pr.GetState() == "open" && pr.GetHead().GetSHA() == target.head &&
		pr.GetBase().GetRepo().GetFullName() == target.owner+"/"+target.repo
}

func validateApprovalCheckout(request *policy.Request, pr *gh.PullRequest, expectedHead string) error {
	checkout, err := atmosgit.GetCurrentCommitSHA()
	if err != nil || (checkout != expectedHead && checkout != pr.GetMergeCommitSHA()) ||
		(request.Revisions.Head != "" && request.Revisions.Head != checkout) {
		return fmt.Errorf("%w: approval evidence does not match the checked-out PR revision", errUtils.ErrPolicyInputUnavailable)
	}
	return nil
}

func approvalRevisionUnchanged(before, after *gh.PullRequest, expectedHead string) bool {
	return after.GetState() == "open" && after.GetHead().GetSHA() == expectedHead &&
		after.GetBase().GetSHA() == before.GetBase().GetSHA() && after.GetMergeCommitSHA() == before.GetMergeCommitSHA()
}

func approvalUnavailable() error {
	return fmt.Errorf("%w: current GitHub PR/reviews unavailable or changed; pull-requests: read is required", errUtils.ErrPolicyInputUnavailable)
}

func approvalTarget(request *policy.Request) (pullRequestApprovalTarget, error) {
	if !approvalSnapshotAvailable(request) {
		return pullRequestApprovalTarget{}, approvalUnavailable()
	}
	var metadata map[string]string
	var event struct {
		PullRequest *gh.PullRequest `json:"pull_request"`
	}
	if json.Unmarshal(request.CI.Metadata.Data, &metadata) != nil || json.Unmarshal(request.CI.Event.Data, &event) != nil {
		return pullRequestApprovalTarget{}, approvalUnavailable()
	}
	parts := strings.Split(metadata["GITHUB_REPOSITORY"], "/")
	kind := metadata["GITHUB_EVENT_NAME"]
	if !validApprovalEvent(parts, kind, event.PullRequest) {
		return pullRequestApprovalTarget{}, approvalUnavailable()
	}
	return pullRequestApprovalTarget{
		owner: parts[0], repo: parts[1], number: event.PullRequest.GetNumber(), head: event.PullRequest.GetHead().GetSHA(),
	}, nil
}

func approvalSnapshotAvailable(request *policy.Request) bool {
	return request != nil && request.CI != nil && request.CI.Provider == ProviderName && request.CI.Schema == PolicySnapshotSchema &&
		request.CI.Event.Status == policy.Available && request.CI.Event.Complete && request.CI.Metadata.Status == policy.Available && request.CI.Metadata.Complete
}

func validApprovalEvent(repository []string, kind string, pr *gh.PullRequest) bool {
	return len(repository) == 2 && repository[0] != "" && repository[1] != "" && !strings.ContainsAny(strings.Join(repository, ""), "?#%\\") &&
		slices.Contains([]string{"pull_request", "pull_request_target", "pull_request_review"}, kind) &&
		pr.GetNumber() > 0 && pr.GetHead().GetSHA() != ""
}

func (p *Provider) collectApprovalReviews(ctx context.Context, owner, repo string, number int) ([]policy.GitHubReview, error) {
	var result []policy.GitHubReview
	page := 1
	for pages := 0; pages < approvalReviewPageLimit; pages++ {
		reviews, response, err := p.client.client.PullRequests.ListReviews(ctx, owner, repo, number, &gh.ListOptions{Page: page, PerPage: approvalReviewsPerPage})
		if err != nil || response == nil {
			return nil, approvalUnavailable()
		}
		for _, review := range reviews {
			result = append(result, policy.GitHubReview{ID: review.GetID(), Login: review.GetUser().GetLogin(), State: review.GetState(), CommitID: review.GetCommitID(), SubmittedAt: review.GetSubmittedAt().Time})
		}
		if response.NextPage == 0 {
			return result, nil
		}
		if response.NextPage <= page {
			return nil, approvalUnavailable()
		}
		page = response.NextPage
	}
	return nil, approvalUnavailable()
}
