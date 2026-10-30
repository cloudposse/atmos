package milestone

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

const milestoneStatus = "Release milestone"

// GitHub limits commit-status descriptions to 140 Unicode characters.
const statusDescriptionLimit = 140

type queueCommit struct {
	OID string `json:"oid"`
}
type queueEntry struct {
	Head *queueCommit `json:"headCommit"`
	Base *queueCommit `json:"baseCommit"`
	PR   struct {
		Number      int          `json:"number"`
		Merged      bool         `json:"merged"`
		BaseRefName string       `json:"baseRefName"`
		MergeCommit *queueCommit `json:"mergeCommit"`
	} `json:"pullRequest"`
}

// Check reconciles PR-head and merge-queue statuses using live metadata.
// It never checks out or executes a PR's source. Require the Release milestone
// status in branch protection/rulesets to enforce this policy at merge time.
func (Milestones) Check(repo string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client, err := NewClient(ctx, repo)
	if err != nil {
		return err
	}
	return client.checkMilestones(os.Getenv("GITHUB_EVENT_NAME"), os.Getenv("GITHUB_EVENT_PATH"))
}

const statusEvidenceError = "Milestone policy could not be verified; see workflow logs"

type commitStatus struct {
	Context     string `json:"context"`
	State       string `json:"state"`
	Description string `json:"description"`
}

func (c *Client) status(sha, state, description string) error {
	if !githubapi.SHAPattern.MatchString(sha) {
		return fmt.Errorf("%w: invalid status SHA", ErrMilestonePolicy)
	}
	description = truncateStatus(description)
	current, err := c.currentStatus(sha)
	if err != nil {
		// Never publish success when status evidence is unreadable. Attempt to
		// revoke a previous green result even if the read endpoint is unavailable.
		return errors.Join(err, c.writeStatus(sha, "error", statusEvidenceError))
	}
	if current.State == state && current.Description == description {
		return nil
	}
	return c.writeStatus(sha, state, description)
}

func truncateStatus(description string) string {
	chars := []rune(description)
	if len(chars) > statusDescriptionLimit {
		return string(chars[:statusDescriptionLimit-len("...")]) + "..."
	}
	return description
}

func (c *Client) writeStatus(sha, state, description string) error {
	return c.Write(http.MethodPost, "statuses/"+sha, map[string]string{"state": state, "context": milestoneStatus, "description": description})
}

func (c *Client) currentStatus(sha string) (commitStatus, error) {
	// The combined endpoint returns the latest status for each context rather
	// than its entire history. Other contexts can occupy additional pages.
	for page := 1; page <= githubapi.PageLimit; page++ {
		var result struct {
			Statuses []commitStatus `json:"statuses"`
		}
		path := fmt.Sprintf("commits/%s/status?per_page=%d&page=%d", sha, githubapi.PageSize, page)
		if err := c.Get(path, &result); err != nil {
			return commitStatus{}, err
		}
		for _, status := range result.Statuses {
			if status.Context == milestoneStatus {
				return status, nil
			}
		}
		if len(result.Statuses) < githubapi.PageSize {
			return commitStatus{}, nil
		}
	}
	return commitStatus{}, fmt.Errorf("%w: commit status pagination limit", ErrMilestonePolicy)
}

func (c *Client) queue(base string) ([]queueEntry, error) {
	parts := strings.Split(c.Repo, "/")
	const query = `query($owner:String!,$name:String!,$branch:String!,$cursor:String){repository(owner:$owner,name:$name){mergeQueue(branch:$branch){entries(first:100,after:$cursor){nodes{headCommit{oid} baseCommit{oid} pullRequest{number merged baseRefName mergeCommit{oid}}} pageInfo{hasNextPage endCursor}}}}}`
	var all []queueEntry
	var cursor *string
	for page := 0; page < githubapi.PageLimit; page++ {
		body, _ := json.Marshal(map[string]any{"query": query, "variables": map[string]any{"owner": parts[0], "name": parts[1], "branch": base, "cursor": cursor}})
		var result struct {
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
			Data struct {
				Repository *struct {
					Queue *struct {
						Entries struct {
							Nodes    []queueEntry `json:"nodes"`
							PageInfo struct {
								HasNext bool   `json:"hasNextPage"`
								Cursor  string `json:"endCursor"`
							} `json:"pageInfo"`
						} `json:"entries"`
					} `json:"mergeQueue"`
				} `json:"repository"`
			} `json:"data"`
		}
		err := c.API.DoWithContext(c.Context, http.MethodPost, "graphql", bytes.NewReader(body), &result)
		if err != nil {
			return nil, err
		}
		if len(result.Errors) > 0 || result.Data.Repository == nil {
			return nil, fmt.Errorf("%w: cannot read merge queue", ErrMilestonePolicy)
		}
		if result.Data.Repository.Queue == nil {
			return all, nil
		}
		entries := result.Data.Repository.Queue.Entries
		all = append(all, entries.Nodes...)
		if !entries.PageInfo.HasNext {
			return all, nil
		}
		if entries.PageInfo.Cursor == "" || (cursor != nil && *cursor == entries.PageInfo.Cursor) {
			return nil, fmt.Errorf("%w: invalid queue pagination cursor", ErrMilestonePolicy)
		}
		cursor = &entries.PageInfo.Cursor
	}
	return nil, fmt.Errorf("%w: merge queue pagination limit", ErrMilestonePolicy)
}

// queueVerdict includes all entries sharing a group head and every preceding
// queued group in its base chain. A later blocked PR cannot taint earlier groups.
func queueVerdict(head string, entries []queueEntry, verdicts map[int]error, branch string) error {
	policy := queuePolicy{entries: entries, verdicts: verdicts, visiting: map[string]bool{}, branch: branch}
	return policy.checkHead(head)
}

type queuePolicy struct {
	entries  []queueEntry
	verdicts map[int]error
	visiting map[string]bool
	branch   string
}

func (p *queuePolicy) checkHead(sha string) error {
	if p.visiting[sha] {
		return fmt.Errorf("%w: cyclic merge queue", ErrMilestonePolicy)
	}
	p.visiting[sha] = true
	defer delete(p.visiting, sha)
	for _, entry := range p.entries {
		if entry.Head == nil || entry.Head.OID != sha {
			continue
		}
		if err := p.checkEntry(entry, sha); err != nil {
			return err
		}
	}
	return nil
}

func (p *queuePolicy) checkEntry(entry queueEntry, sha string) error {
	verdict, ok := p.verdicts[entry.PR.Number]
	// Open PRs and merge-queue entries are separate API snapshots. A landed
	// entry can remain in the queue after its PR leaves the open list. Accept
	// only explicit evidence that this exact group commit merged into our base.
	if !ok && entry.PR.Merged && entry.PR.BaseRefName == p.branch &&
		entry.PR.MergeCommit != nil && entry.PR.MergeCommit.OID == sha {
		ok = true
	}
	if !ok {
		return fmt.Errorf("%w: queue PR #%d is missing from live open PRs", ErrMilestonePolicy, entry.PR.Number)
	}
	if verdict != nil {
		return verdict
	}
	if entry.Base == nil || !githubapi.SHAPattern.MatchString(entry.Base.OID) {
		return fmt.Errorf("%w: queue base missing", ErrMilestonePolicy)
	}
	if entry.Base.OID == sha {
		return nil
	}
	return p.checkHead(entry.Base.OID)
}

func mergeGroupHead(eventPath string) (string, error) {
	root, err := os.OpenRoot(filepath.Dir(eventPath))
	if err != nil {
		return "", err
	}
	defer root.Close()
	data, err := root.ReadFile(filepath.Base(eventPath))
	if err != nil {
		return "", err
	}
	var event struct {
		Group struct {
			Head string `json:"head_sha"`
		} `json:"merge_group"`
	}
	if err = json.Unmarshal(data, &event); err != nil {
		return "", err
	}
	return event.Group.Head, nil
}

func (c *Client) checkMilestones(eventName, eventPath string) error {
	eventHead := ""
	if eventName == "merge_group" {
		var err error
		eventHead, err = mergeGroupHead(eventPath)
		if err != nil {
			return err
		}
		if !githubapi.SHAPattern.MatchString(eventHead) {
			return fmt.Errorf("%w: invalid status SHA", ErrMilestonePolicy)
		}
	}
	return c.reconcileMilestoneStatuses(eventHead)
}

func (c *Client) reconcileMilestoneStatuses(eventHead string) (result error) {
	heads := map[string]bool{}
	if eventHead != "" {
		heads[eventHead] = true
	}
	// Reuse unchanged final statuses: pending/success pairs on every scheduled
	// run exhaust GitHub's per-context status limit. Known heads are revoked on
	// any evidence or publication error, including errors before queue discovery.
	// An existing success remains during evaluation; a killed process or an API
	// write outage cannot atomically revoke it. A later successful reconciliation
	// repairs those cases.
	defer func() {
		if result != nil {
			result = errors.Join(result, c.revokeStatuses(heads))
		}
	}()
	branch, err := c.defaultBranch()
	if err != nil {
		return err
	}
	prs, err := c.OpenPRs(branch)
	if err != nil {
		return err
	}
	for _, pr := range prs {
		heads[pr.Head.SHA] = true
	}
	entries, err := c.queue(branch)
	if err != nil {
		return err
	}
	queueHeads, err := queueGroupHeads(entries, eventHead)
	for head := range queueHeads {
		heads[head] = true
	}
	if err != nil {
		return err
	}
	return c.publishPolicyVerdicts(prs, entries, queueHeads, branch)
}

func (c *Client) revokeStatuses(heads map[string]bool) error {
	var failures []error
	for head := range heads {
		if err := c.status(head, "error", statusEvidenceError); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (c *Client) defaultBranch() (string, error) {
	var repo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := c.Get("", &repo); err != nil {
		return "", err
	}
	if repo.DefaultBranch == "" {
		return "", fmt.Errorf("%w: default branch missing", ErrMilestonePolicy)
	}
	return repo.DefaultBranch, nil
}

func queueGroupHeads(entries []queueEntry, eventHead string) (map[string]bool, error) {
	heads := map[string]bool{}
	for _, entry := range entries {
		if entry.Head != nil {
			heads[entry.Head.OID] = true
		}
	}
	if eventHead != "" && !heads[eventHead] {
		return heads, fmt.Errorf("%w: merge group is not present in the live queue; retry", ErrMilestonePolicy)
	}
	return heads, nil
}

func (c *Client) publishPolicyVerdicts(prs []ReleasePR, entries []queueEntry, queueHeads map[string]bool, branch string) error {
	milestones, err := c.ListMilestones()
	if err != nil {
		return err
	}
	baseline, baselineErr := c.LatestStable()
	if baselineErr == nil && requiresReleaseHistory(prs) {
		baselineErr = c.CheckReleaseHistory(milestones, baseline)
	}
	verdicts, headVerdicts := evaluatePRs(prs, milestones, baseline, baselineErr)
	for head := range queueHeads {
		headVerdicts[head] = errors.Join(headVerdicts[head], queueVerdict(head, entries, verdicts, branch))
	}
	return c.writeVerdictStatuses(headVerdicts)
}

func evaluatePRs(prs []ReleasePR, milestones []Milestone, baseline string, baselineErr error) (map[int]error, map[string]error) {
	verdicts := map[int]error{}
	headVerdicts := map[string]error{}
	for _, pr := range prs {
		verdict := CheckMilestonePR(&pr, milestones, baseline)
		label, labelErr := ReleaseLabel(&pr)
		if baselineErr != nil && (labelErr != nil || label != "no-release") {
			verdict = baselineErr
		}
		verdicts[pr.Number] = verdict
		// Two PRs sharing a head must both qualify. Never let an exemption on one
		// PR overwrite a failure on another PR at the same commit.
		headVerdicts[pr.Head.SHA] = errors.Join(headVerdicts[pr.Head.SHA], verdict)
	}
	return verdicts, headVerdicts
}

func (c *Client) writeVerdictStatuses(verdicts map[string]error) error {
	var failures []error
	for head, verdict := range verdicts {
		state, description := "success", "Milestone ordering and release-label compatibility verified"
		if verdict != nil {
			state, description = "failure", verdict.Error()
		}
		if err := c.status(head, state, description); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// Exempt-only batches must remain independent of release history availability.
func requiresReleaseHistory(prs []ReleasePR) bool {
	for _, pr := range prs {
		label, err := ReleaseLabel(&pr)
		if err == nil && label != "no-release" {
			return true
		}
	}
	return false
}
