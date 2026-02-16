package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v59/github"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
)

// getStatus fetches the CI status for the given options.
func (p *Provider) getStatus(ctx context.Context, opts ci.StatusOptions) (*ci.Status, error) {
	status := &ci.Status{
		Repository: fmt.Sprintf("%s/%s", opts.Owner, opts.Repo),
	}

	// Get status for current branch.
	branchStatus, err := p.getBranchStatus(ctx, opts.Owner, opts.Repo, opts.Branch, opts.SHA)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrCIStatusFetchFailed, err)
	}
	status.CurrentBranch = branchStatus

	// Get PRs created by the authenticated user.
	if opts.IncludeUserPRs {
		userPRs, err := p.getPRsCreatedByUser(ctx, opts.Owner, opts.Repo)
		if err != nil {
			// Non-fatal: continue without user PRs.
			userPRs = nil
		}
		status.CreatedByUser = userPRs
	}

	// Get PRs requesting review from the authenticated user.
	if opts.IncludeReviewRequests {
		reviewPRs, err := p.getPRsRequestingReview(ctx, opts.Owner, opts.Repo)
		if err != nil {
			// Non-fatal: continue without review requests.
			reviewPRs = nil
		}
		status.ReviewRequests = reviewPRs
	}

	return status, nil
}

// getBranchStatus gets the status for a specific branch.
func (p *Provider) getBranchStatus(ctx context.Context, owner, repo, branch, sha string) (*ci.BranchStatus, error) {
	status := &ci.BranchStatus{
		Branch:    branch,
		CommitSHA: sha,
	}

	// Get PR for this branch (if any).
	pr, err := p.getPRForBranch(ctx, owner, repo, branch)
	if err == nil && pr != nil {
		status.PullRequest = pr
	}

	// Get check runs for the SHA.
	checks, err := p.getCheckRuns(ctx, owner, repo, sha)
	if err != nil {
		return nil, err
	}
	status.Checks = checks

	// Also get combined status (legacy status API).
	legacyChecks, err := p.getCombinedStatus(ctx, owner, repo, sha)
	if err == nil {
		status.Checks = append(status.Checks, legacyChecks...)
	}

	return status, nil
}

// getPRForBranch finds the PR associated with a branch.
func (p *Provider) getPRForBranch(ctx context.Context, owner, repo, branch string) (*ci.PRStatus, error) {
	// Search for PRs with this head branch.
	prs, _, err := p.client.GitHub().PullRequests.List(ctx, owner, repo, &github.PullRequestListOptions{
		Head:  owner + ":" + branch,
		State: "open",
		ListOptions: github.ListOptions{
			PerPage: 1,
		},
	})
	if err != nil {
		return nil, err
	}

	if len(prs) == 0 {
		return nil, nil
	}

	pr := prs[0]
	prStatus := &ci.PRStatus{
		Number:     pr.GetNumber(),
		Title:      pr.GetTitle(),
		Branch:     pr.GetHead().GetRef(),
		BaseBranch: pr.GetBase().GetRef(),
		URL:        pr.GetHTMLURL(),
	}

	// Get checks for this PR's head SHA.
	if headSHA := pr.GetHead().GetSHA(); headSHA != "" {
		checks, _ := p.getCheckRuns(ctx, owner, repo, headSHA)
		prStatus.Checks = checks
		prStatus.AllPassed = allChecksPassed(checks)
	}

	return prStatus, nil
}

// getCheckRuns fetches check runs for a commit.
func (p *Provider) getCheckRuns(ctx context.Context, owner, repo, ref string) ([]*ci.CheckStatus, error) {
	result, _, err := p.client.GitHub().Checks.ListCheckRunsForRef(ctx, owner, repo, ref, &github.ListCheckRunsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	})
	if err != nil {
		return nil, err
	}

	checks := make([]*ci.CheckStatus, 0, len(result.CheckRuns))
	for _, cr := range result.CheckRuns {
		checks = append(checks, &ci.CheckStatus{
			Name:       cr.GetName(),
			Status:     cr.GetStatus(),
			Conclusion: cr.GetConclusion(),
			DetailsURL: cr.GetDetailsURL(),
		})
	}

	return checks, nil
}

// getCombinedStatus fetches the combined status for a ref (legacy status API).
func (p *Provider) getCombinedStatus(ctx context.Context, owner, repo, ref string) ([]*ci.CheckStatus, error) {
	status, _, err := p.client.GitHub().Repositories.GetCombinedStatus(ctx, owner, repo, ref, &github.ListOptions{
		PerPage: 100,
	})
	if err != nil {
		return nil, err
	}

	checks := make([]*ci.CheckStatus, 0, len(status.Statuses))
	for _, s := range status.Statuses {
		checks = append(checks, &ci.CheckStatus{
			Name:       s.GetContext(),
			Status:     "completed", // Legacy status is always completed.
			Conclusion: s.GetState(),
			DetailsURL: s.GetTargetURL(),
		})
	}

	return checks, nil
}

// getPRsCreatedByUser fetches PRs created by the authenticated user.
func (p *Provider) getPRsCreatedByUser(ctx context.Context, owner, repo string) ([]*ci.PRStatus, error) {
	// Get authenticated user.
	user, _, err := p.client.GitHub().Users.Get(ctx, "")
	if err != nil {
		return nil, err
	}

	// Search for PRs created by this user.
	query := fmt.Sprintf("repo:%s/%s is:pr is:open author:%s", owner, repo, user.GetLogin())
	return p.searchPRsWithQuery(ctx, owner, repo, query)
}

// getPRsRequestingReview fetches PRs requesting review from the authenticated user.
func (p *Provider) getPRsRequestingReview(ctx context.Context, owner, repo string) ([]*ci.PRStatus, error) {
	// Get authenticated user.
	user, _, err := p.client.GitHub().Users.Get(ctx, "")
	if err != nil {
		return nil, err
	}

	// Search for PRs requesting review from this user.
	query := fmt.Sprintf("repo:%s/%s is:pr is:open review-requested:%s", owner, repo, user.GetLogin())
	return p.searchPRsWithQuery(ctx, owner, repo, query)
}

// searchPRsWithQuery searches for PRs using the given GitHub search query.
func (p *Provider) searchPRsWithQuery(ctx context.Context, owner, repo, query string) ([]*ci.PRStatus, error) {
	result, _, err := p.client.GitHub().Search.Issues(ctx, query, &github.SearchOptions{
		ListOptions: github.ListOptions{
			PerPage: 10,
		},
	})
	if err != nil {
		return nil, err
	}

	prs := make([]*ci.PRStatus, 0, len(result.Issues))
	for _, issue := range result.Issues {
		pr := &ci.PRStatus{
			Number: issue.GetNumber(),
			Title:  issue.GetTitle(),
			URL:    issue.GetHTMLURL(),
		}

		// Get full PR details to get check status.
		fullPR, _, err := p.client.GitHub().PullRequests.Get(ctx, owner, repo, issue.GetNumber())
		if err == nil {
			pr.Branch = fullPR.GetHead().GetRef()
			pr.BaseBranch = fullPR.GetBase().GetRef()

			if headSHA := fullPR.GetHead().GetSHA(); headSHA != "" {
				checks, _ := p.getCheckRuns(ctx, owner, repo, headSHA)
				pr.Checks = checks
				pr.AllPassed = allChecksPassed(checks)
			}
		}

		prs = append(prs, pr)
	}

	return prs, nil
}

// allChecksPassed returns true if all checks have passed.
func allChecksPassed(checks []*ci.CheckStatus) bool {
	if len(checks) == 0 {
		return true
	}

	for _, check := range checks {
		state := check.CheckState()
		if state != ci.CheckStatusStateSuccess && state != ci.CheckStatusStateSkipped {
			return false
		}
	}
	return true
}
