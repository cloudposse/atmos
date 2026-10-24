package milestone

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

// PullRequest includes the live metadata and notes of an associated commit.
type PullRequest struct {
	ReleasePR
	Title       string `json:"title"`
	Body        string `json:"body"`
	Merged      bool   `json:"merged"`
	MergeCommit string `json:"merge_commit_sha"`
	User        struct {
		Login string `json:"login"`
	} `json:"user"`
}

// CheckIncludedPR reports why an already merged PR makes the next release invalid.
func CheckIncludedPR(pr *ReleasePR, milestones []Milestone, baseline string) error {
	err := CheckMilestonePR(pr, milestones, baseline)
	if err == nil {
		return nil
	}
	next, nextErr := NextMilestone(milestones)
	if nextErr != nil {
		return err
	}
	assigned := "no milestone"
	if pr.Milestone != nil {
		assigned = pr.Milestone.Title
	}
	return fmt.Errorf("cannot release %s: source already includes merged PR #%d assigned to %s: %w", next.Title, pr.Number, assigned, err)
}

// CheckReleaseHistory rejects milestones that cannot contain the unreleased
// changes already on the default branch. It never mutates release intent.
func (c *Client) CheckReleaseHistory(milestones []Milestone, baseline string) error {
	branch, err := c.defaultBranch()
	if err != nil {
		return err
	}
	var head struct {
		SHA string `json:"sha"`
	}
	if err = c.Get("commits/"+url.PathEscape(branch), &head); err != nil {
		return err
	}
	if !githubapi.SHAPattern.MatchString(head.SHA) {
		return fmt.Errorf("%w: invalid default branch SHA", ErrMilestonePolicy)
	}
	prs, err := c.ReleaseChanges(baseline, head.SHA, branch)
	if err != nil {
		return err
	}
	var failures []error
	for i := range prs {
		pr := &prs[i]
		if err = CheckIncludedPR(&pr.ReleasePR, milestones, baseline); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// ReleaseChanges resolves merged PRs from the exact commit range, including
// pagination and deduplication. Direct commits and incomplete evidence fail closed.
func (c *Client) ReleaseChanges(baseline, head, branch string) ([]PullRequest, error) {
	commits, err := c.CompareCommits(baseline, head)
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	var changes []PullRequest
	for _, commit := range commits {
		prs, err := c.commitPullRequests(commit, branch, seen)
		if err != nil {
			return nil, err
		}
		changes = append(changes, prs...)
	}
	return changes, nil
}

func (c *Client) commitPullRequests(commit, branch string, seen map[int]bool) ([]PullRequest, error) {
	associated, err := Pages[struct {
		Number int `json:"number"`
	}](c, "commits/"+commit+"/pulls?")
	if err != nil {
		return nil, err
	}
	if len(associated) == 0 {
		return nil, fmt.Errorf("%w: commit %s has no associated PR", ErrMilestonePolicy, commit)
	}
	var prs []PullRequest
	included := false
	for _, item := range associated {
		if seen[item.Number] {
			included = true
			continue
		}
		var pr PullRequest
		if err = c.Get(fmt.Sprintf("pulls/%d", item.Number), &pr); err != nil {
			return nil, err
		}
		if !pr.Merged || pr.Base.Ref != branch {
			continue
		}
		seen[pr.Number] = true
		included = true
		prs = append(prs, pr)
	}
	if !included {
		return nil, fmt.Errorf("%w: commit %s has no eligible merged PR", ErrMilestonePolicy, commit)
	}
	return prs, nil
}

// CompareCommits returns the complete source delta from a published baseline.
func (c *Client) CompareCommits(base, head string) ([]string, error) {
	var all []string
	for page := 1; page <= githubapi.PageLimit; page++ {
		var comparison struct {
			Status  string `json:"status"`
			Commits []struct {
				SHA string `json:"sha"`
			} `json:"commits"`
			Total int `json:"total_commits"`
		}
		err := c.Get(fmt.Sprintf("compare/%s...%s?per_page=100&page=%d", url.PathEscape(base), url.PathEscape(head), page), &comparison)
		if err != nil {
			return nil, err
		}
		if comparison.Status != "ahead" && comparison.Status != "identical" {
			return nil, fmt.Errorf("%w: source diverges from previous release", ErrMilestonePolicy)
		}
		for _, commit := range comparison.Commits {
			if !githubapi.SHAPattern.MatchString(commit.SHA) {
				return nil, fmt.Errorf("%w: invalid compared commit", ErrMilestonePolicy)
			}
			all = append(all, commit.SHA)
		}
		if len(comparison.Commits) < githubapi.PageSize {
			if len(all) != comparison.Total {
				return nil, fmt.Errorf("%w: incomplete commit comparison", ErrMilestonePolicy)
			}
			return all, nil
		}
	}
	return nil, fmt.Errorf("%w: commit comparison pagination limit", ErrMilestonePolicy)
}
