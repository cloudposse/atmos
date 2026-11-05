package milestone

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

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

// githubWebFlowLogin is the committer GitHub records for commits it creates and signs
// itself: web merges and merge-queue merges. A direct push never carries it.
const githubWebFlowLogin = "web-flow"

// subjectPRPattern captures the PR number GitHub appends to a squash or queue
// commit subject, for example "fix: something (#3608)".
var subjectPRPattern = regexp.MustCompile(`\(#(\d+)\)$`)

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
	landed := make(map[string]bool, len(commits))
	for _, commit := range commits {
		landed[commit] = true
	}
	seen := map[int]bool{}
	var changes []PullRequest
	for _, commit := range commits {
		prs, err := c.commitPullRequests(commit, branch, seen, landed)
		if err != nil {
			return nil, err
		}
		changes = append(changes, prs...)
	}
	return changes, nil
}

func (c *Client) commitPullRequests(commit, branch string, seen map[int]bool, landed map[string]bool) ([]PullRequest, error) {
	associated, err := Pages[struct {
		Number int `json:"number"`
	}](c, "commits/"+commit+"/pulls?")
	if err != nil {
		return nil, err
	}
	if len(associated) == 0 {
		return c.signedSubjectPullRequest(commit, branch, seen)
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
		// Native stack merges retain the original parent branch in PR metadata.
		// Accept that PR only when its merge commit is in the verified release
		// range; a merged PR on an unrelated branch remains ineligible.
		if !pr.Merged || (pr.Base.Ref != branch && !landed[pr.MergeCommit]) {
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

// signedSubjectPullRequest resolves a commit that GitHub's association endpoint
// lists no PR for. A merge-queue push can land on the default branch without the
// PR ever being marked merged, which leaves that endpoint empty permanently.
// The commit subject still names the PR, so trust it only when GitHub itself
// signed the commit. Direct pushes and unsigned commits keep failing closed.
// The PR is returned even though it is not marked merged; its milestone and
// release label are then evaluated like any other included PR.
func (c *Client) signedSubjectPullRequest(commit, branch string, seen map[int]bool) ([]PullRequest, error) {
	noPR := fmt.Errorf("%w: commit %s has no associated PR", ErrMilestonePolicy, commit)
	number, err := c.signedCommitPRNumber(commit)
	if err != nil {
		return nil, err
	}
	if number == 0 {
		return nil, noPR
	}
	if seen[number] {
		return nil, nil
	}
	var pr PullRequest
	if err = c.Get(fmt.Sprintf("pulls/%d", number), &pr); err != nil {
		if githubapi.NotFound(err) {
			return nil, noPR
		}
		return nil, err
	}
	if pr.Base.Ref != branch {
		return nil, noPR
	}
	seen[number] = true
	return []PullRequest{pr}, nil
}

// signedCommitPRNumber returns the PR number named by the subject of a commit that
// GitHub signed itself, or zero when the commit is not signed by GitHub or names no PR.
func (c *Client) signedCommitPRNumber(commit string) (int, error) {
	var detail struct {
		Commit struct {
			Message      string `json:"message"`
			Verification struct {
				Verified bool `json:"verified"`
			} `json:"verification"`
		} `json:"commit"`
		Committer *struct {
			Login string `json:"login"`
		} `json:"committer"`
	}
	if err := c.Get("commits/"+commit, &detail); err != nil {
		return 0, err
	}
	if !detail.Commit.Verification.Verified || detail.Committer == nil || detail.Committer.Login != githubWebFlowLogin {
		return 0, nil
	}
	subject, _, _ := strings.Cut(detail.Commit.Message, "\n")
	match := subjectPRPattern.FindStringSubmatch(strings.TrimSpace(subject))
	if match == nil {
		return 0, nil
	}
	number, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, fmt.Errorf("%w: commit %s names an invalid PR number", ErrMilestonePolicy, commit)
	}
	return number, nil
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
