package release

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/release/milestone"
)

const (
	releaseModeCut       = "cut"
	releaseModeCandidate = "candidate"
	releaseModeNightly   = "nightly"
	releaseModeFeature   = "feature"
)

// Plan freezes the version and source chosen before dispatch. GitHub recomputes
// the plan before creating a tag; the local plan is never authorization evidence.
type Plan struct {
	Repository  string                 `json:"repository"`
	Milestone   Milestone              `json:"milestone"`
	Version     string                 `json:"version"`
	Commit      string                 `json:"commit"`
	PreviousTag string                 `json:"previous_tag"`
	Branch      string                 `json:"branch"`
	Mode        string                 `json:"mode"`
	Entries     []releasenotes.PREntry `json:"entries"`
	Unfinished  []int                  `json:"unfinished"`
	Resume      *releaseRecovery       `json:"resume,omitempty"`
}

type releasePull = milestone.PullRequest

// plan reads current GitHub state without changing tags, releases or milestones.
// Stable cuts require a complete milestone; candidates validate included changes
// while allowing work assigned to the milestone to remain unfinished.
func (c *milestoneClient) plan(mode, requestedCommit string) (*Plan, error) {
	if mode != releaseModeCut && mode != releaseModeCandidate {
		return nil, fmt.Errorf("%w: unsupported release mode", ErrMilestonePolicy)
	}
	milestones, err := c.ListMilestones()
	if err != nil {
		return nil, err
	}
	next, err := milestone.NextMilestone(milestones)
	if err != nil {
		return nil, err
	}
	baseline, err := c.LatestStable()
	if err != nil {
		return nil, err
	}
	if _, err = milestone.RequiredBump(baseline, next.Title); err != nil {
		return nil, err
	}
	branch, commit, err := c.resolveReleaseSource(requestedCommit)
	if err != nil {
		return nil, err
	}
	version, _ := milestone.MilestoneVersion(next.Title)
	plan := &Plan{
		Repository:  c.Repo,
		Milestone:   *next,
		Version:     "v" + version.String(),
		Commit:      commit,
		PreviousTag: baseline,
		Branch:      branch,
		Mode:        mode,
	}
	if err = c.checkMilestoneCompletion(plan); err != nil {
		return nil, err
	}
	if err = c.collectReleaseEntries(plan, milestones); err != nil {
		return nil, err
	}
	return plan, nil
}

// resolveReleaseSource accepts only a full SHA already on the default branch.
func (c *milestoneClient) resolveReleaseSource(requestedCommit string) (branch, commit string, err error) {
	var repository struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err = c.Get("", &repository); err != nil {
		return "", "", err
	}
	if repository.DefaultBranch == "" {
		return "", "", fmt.Errorf("%w: default branch missing", ErrMilestonePolicy)
	}
	ref := repository.DefaultBranch
	if requestedCommit != "" {
		if !githubapi.SHAPattern.MatchString(requestedCommit) {
			return "", "", fmt.Errorf("%w: full source SHA required", ErrMilestonePolicy)
		}
		ref = requestedCommit
	}
	var source struct {
		SHA string `json:"sha"`
	}
	if err = c.Get("commits/"+url.PathEscape(ref), &source); err != nil {
		return "", "", err
	}
	if err = validateSourceCommit(source.SHA, requestedCommit); err != nil {
		return "", "", err
	}
	included, err := c.isAncestor(source.SHA, repository.DefaultBranch)
	if err != nil {
		return "", "", err
	}
	if !included {
		return "", "", fmt.Errorf("%w: source is not on the default branch", ErrMilestonePolicy)
	}
	return repository.DefaultBranch, source.SHA, nil
}

type milestoneItem struct {
	Number      int             `json:"number"`
	State       string          `json:"state"`
	PullRequest json.RawMessage `json:"pull_request"`
}

func (item milestoneItem) isPullRequest() bool {
	return len(item.PullRequest) > 0 && string(item.PullRequest) != "null"
}

func (c *milestoneClient) checkMilestoneCompletion(plan *Plan) error {
	items, err := milestonePages[milestoneItem](c, fmt.Sprintf("issues?state=all&milestone=%d", plan.Milestone.Number))
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.State == "open" && item.isPullRequest() {
			plan.Unfinished = append(plan.Unfinished, item.Number)
		}
	}
	if plan.Mode != releaseModeCut {
		return nil
	}
	if len(plan.Unfinished) > 0 {
		return fmt.Errorf("%w: milestone has unmerged PRs: %v", ErrMilestonePolicy, plan.Unfinished)
	}
	for _, item := range items {
		if !item.isPullRequest() {
			continue
		}
		if err = c.requireMergedPullRequest(item.Number, plan.Commit); err != nil {
			return err
		}
	}
	return nil
}

func (c *milestoneClient) requireMergedPullRequest(number int, commit string) error {
	var pr releasePull
	if err := c.Get(fmt.Sprintf("pulls/%d", number), &pr); err != nil {
		return err
	}
	if !pr.Merged {
		return fmt.Errorf("%w: milestone PR #%d is not merged; merge it or move it out of the milestone", ErrMilestonePolicy, number)
	}
	if !githubapi.SHAPattern.MatchString(pr.MergeCommit) {
		return fmt.Errorf("%w: milestone PR #%d has no merge SHA", ErrMilestonePolicy, number)
	}
	included, err := c.isAncestor(pr.MergeCommit, commit)
	if err != nil {
		return err
	}
	if !included {
		return fmt.Errorf("%w: milestone PR #%d is absent from source SHA", ErrMilestonePolicy, number)
	}
	return nil
}

func (c *milestoneClient) isAncestor(base, head string) (bool, error) {
	var comparison struct {
		Status string `json:"status"`
	}
	if err := c.Get("compare/"+url.PathEscape(base)+"..."+url.PathEscape(head), &comparison); err != nil {
		return false, err
	}
	return comparison.Status == "ahead" || comparison.Status == "identical", nil
}

func (c *milestoneClient) collectReleaseEntries(plan *Plan, milestones []Milestone) error {
	notes, err := loadReleaseNotesSelection()
	if err != nil {
		return err
	}
	prs, err := c.ReleaseChanges(plan.PreviousTag, plan.Commit, plan.Branch)
	if err != nil {
		return err
	}
	if len(prs) == 0 && plan.Mode == releaseModeCut {
		return fmt.Errorf("%w: no changes since %s", ErrMilestonePolicy, plan.PreviousTag)
	}
	for i := range prs {
		pr := &prs[i]
		if err = milestone.CheckIncludedPR(&pr.ReleasePR, milestones, plan.PreviousTag); err != nil {
			return err
		}
		plan.Entries = append(plan.Entries, notes.noteEntry(pr))
	}
	notes.sortEntries(plan.Entries)
	return nil
}

func validateSourceCommit(actual, requested string) error {
	if !githubapi.SHAPattern.MatchString(actual) || (requested != "" && actual != requested) {
		return fmt.Errorf("%w: source SHA did not resolve exactly", ErrMilestonePolicy)
	}
	return nil
}
