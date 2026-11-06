package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v59/github"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

var _ provider.MergedPullRequestResolver = (*Provider)(nil)

const mergedPullRequestsPerPage = 100

// MergedPullRequest returns the merged pull request whose merge commit is sha.
//
// GitHub associates a commit with every pull request that contains it, so a commit on the base
// branch can be linked to several pull requests. Only a merged pull request whose
// merge_commit_sha equals sha landed as this commit. That covers squash merges, merge commits,
// rebase merges (the last rebased commit), and merge-queue merges. A pull request whose head is
// sha (a fast-forward merge) is skipped: its plan is already stored under sha.
func (p *Provider) MergedPullRequest(ctx context.Context, run *provider.Context, sha string) (*provider.PullRequestRevision, error) {
	defer perf.Track(nil, "github.Provider.MergedPullRequest")()

	if run == nil || run.RepoOwner == "" || run.RepoName == "" || sha == "" {
		return nil, nil
	}
	if err := p.ensureClient(); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrCIMergedPRLookupFailed, err)
	}

	merged, err := p.searchMergedAs(ctx, run.RepoOwner, run.RepoName, sha)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrCIMergedPRLookupFailed, err)
	}
	return merged, nil
}

// searchMergedAs pages through the pull requests GitHub associates with sha until it finds the
// one merged as sha.
func (p *Provider) searchMergedAs(ctx context.Context, owner, repo, sha string) (*provider.PullRequestRevision, error) {
	options := &github.ListOptions{PerPage: mergedPullRequestsPerPage}
	var found *provider.PullRequestRevision
	for {
		page, response, err := p.client.GitHub().PullRequests.ListPullRequestsWithCommit(ctx, owner, repo, sha, options)
		if err != nil {
			return nil, err
		}
		for _, pr := range page {
			merged := findMergedAs([]*github.PullRequest{pr}, sha)
			if merged == nil {
				continue
			}
			if found != nil && found.Number != merged.Number {
				return nil, fmt.Errorf("%w: multiple pull requests were merged as %s", errUtils.ErrCIMergedPRLookupFailed, sha)
			}
			found = merged
		}
		if response == nil || response.NextPage == 0 {
			return found, nil
		}
		options.Page = response.NextPage
	}
}

// findMergedAs returns the pull request in prs that was merged as sha, or nil.
func findMergedAs(prs []*github.PullRequest, sha string) *provider.PullRequestRevision {
	for _, pr := range prs {
		if pr == nil || pr.MergedAt == nil || !strings.EqualFold(pr.GetMergeCommitSHA(), sha) {
			continue
		}
		head := pr.GetHead().GetSHA()
		if head == "" || strings.EqualFold(head, sha) {
			continue
		}
		return &provider.PullRequestRevision{Number: pr.GetNumber(), HeadSHA: head}
	}
	return nil
}
