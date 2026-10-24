package release

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/release/milestone"
)

type preparedRelease struct {
	Plan
	Tag   string `json:"tag"`
	RunID string `json:"run_id"`
}

// Prepare validates workflow inputs and reserves the immutable build identity.
// It creates a tag, never a release or release assets. Run only on trusted tooling.
func (Release) Prepare(repo string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client, err := newMilestoneClient(ctx, repo)
	if err != nil {
		return err
	}
	p, err := client.prepare(os.Getenv)
	if err != nil {
		return err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err = writePreparedPlan(os.Getenv("RELEASE_PLAN_FILE"), data); err != nil {
		return err
	}
	return releaseOutputs(map[string]string{"tag": p.Tag, "commit": p.Commit, "milestone": fmt.Sprint(p.Milestone.Number), "mode": p.Mode})
}

func (c *milestoneClient) authorize(env func(string) string) error {
	if env("ATMOS_GUARDED_RELEASES_ENABLED") != "true" {
		return fmt.Errorf("%w: guarded release cutover is not enabled", ErrMilestonePolicy)
	}
	scheduled := env("RELEASE_MODE") == releaseModeNightly && env("GITHUB_EVENT_NAME") == "schedule"
	if scheduled && env("GITHUB_RUN_ATTEMPT") == "1" {
		return nil
	}
	if !scheduled && !authorizedActorID(env("GITHUB_ACTOR_ID")) {
		return fmt.Errorf("%w: unauthorized release actor", ErrMilestonePolicy)
	}
	return c.authorizeTriggeringActor(env("GITHUB_TRIGGERING_ACTOR"))
}

// GitHub IDs and run IDs use canonical decimal strings, never signs or padding.
const (
	decimalBase  = 10
	githubIDBits = 64
	// Workflow plans and outputs contain internal release metadata.
	workflowFileMode = 0o600
)

func authorizedActorID(raw string) bool {
	id, err := strconv.ParseInt(raw, decimalBase, githubIDBits)
	return err == nil && authorizedReleaseID(id) && strconv.FormatInt(id, decimalBase) == raw
}

func (c *milestoneClient) authorizeTriggeringActor(actor string) error {
	// Reruns retain the original actor's privileges. Check their initiator separately.
	if actor == "" {
		return fmt.Errorf("%w: triggering actor missing", ErrMilestonePolicy)
	}
	var user struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	}
	if err := c.API.DoWithContext(c.Context, http.MethodGet, "users/"+url.PathEscape(actor), nil, &user); err != nil {
		return err
	}
	if user.Type != "User" || !authorizedReleaseID(user.ID) {
		return fmt.Errorf("%w: unauthorized rerun actor", ErrMilestonePolicy)
	}
	return nil
}

func (c *milestoneClient) prepare(env func(string) string) (*preparedRelease, error) {
	if err := c.authorize(env); err != nil {
		return nil, err
	}
	request := workflowReleaseRequest(env)
	plan, err := c.requestPlan(&request)
	if err != nil {
		return nil, err
	}
	if err = request.validatePlan(plan); err != nil {
		return nil, err
	}
	if !canonicalRunID(request.RunID) {
		return nil, fmt.Errorf("%w: invalid run identity", ErrMilestonePolicy)
	}
	plan.Mode = request.Mode
	result := &preparedRelease{Plan: *plan, RunID: request.RunID}
	if result.Mode == releaseModeCandidate {
		result.Tag, err = c.reserveCandidate(result)
	} else {
		result.Tag, err = expectedTag(result)
	}
	if err != nil {
		return nil, err
	}
	if result.Mode != releaseModeCandidate {
		if err = c.ensureTag(result.Tag, result.Commit); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (c *milestoneClient) requestPlan(request *releaseRequest) (*Plan, error) {
	if request.Mode == releaseModeFeature {
		return c.featurePlan(request.PullRequest)
	}
	mode := request.Mode
	if mode == releaseModeNightly {
		mode = releaseModeCandidate
	}
	return c.plan(mode, request.Commit)
}

func (c *milestoneClient) featurePlan(number string) (*Plan, error) {
	pr, err := c.featurePullRequest(number)
	if err != nil {
		return nil, err
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
	if err = milestone.CheckMilestonePR(&pr.ReleasePR, milestones, baseline); err != nil {
		return nil, err
	}
	notes, err := loadReleaseNotesSelection()
	if err != nil {
		return nil, err
	}
	version, _ := milestone.MilestoneVersion(next.Title)
	return &Plan{
		Repository:  c.Repo,
		Milestone:   *next,
		Version:     "v" + version.String(),
		Commit:      pr.Head.SHA,
		PreviousTag: baseline,
		Branch:      pr.Base.Ref,
		Mode:        releaseModeFeature,
		Entries:     []releasenotes.PREntry{notes.noteEntry(pr)},
	}, nil
}

func (c *milestoneClient) featurePullRequest(number string) (*releasePull, error) {
	n, err := strconv.Atoi(number)
	if err != nil || n <= 0 || strconv.Itoa(n) != number {
		return nil, fmt.Errorf("%w: invalid feature PR", ErrMilestonePolicy)
	}
	var pr releasePull
	if err = c.Get("pulls/"+number, &pr); err != nil {
		return nil, err
	}
	if pr.State != "open" || !githubapi.SHAPattern.MatchString(pr.Head.SHA) {
		return nil, fmt.Errorf("%w: feature PR must be open with a full SHA", ErrMilestonePolicy)
	}
	return &pr, nil
}

// Bound annotated-tag traversal to reject cycles and pathological chains.
const maxTagDepth = 8

func (c *milestoneClient) tagCommit(tag string) (string, error) {
	var ref struct {
		Object struct {
			Type string `json:"type"`
			SHA  string `json:"sha"`
		} `json:"object"`
	}
	if err := c.Get("git/ref/tags/"+url.PathEscape(tag), &ref); err != nil {
		return "", err
	}
	for depth := 0; ref.Object.Type == "tag" && depth < maxTagDepth; depth++ {
		if err := c.Get("git/tags/"+ref.Object.SHA, &ref); err != nil {
			return "", err
		}
	}
	if ref.Object.Type != "commit" || !githubapi.SHAPattern.MatchString(ref.Object.SHA) {
		return "", fmt.Errorf("%w: tag does not resolve to a commit", ErrMilestonePolicy)
	}
	return ref.Object.SHA, nil
}

func (c *milestoneClient) ensureTag(tag, commit string) error {
	actual, err := c.tagCommit(tag)
	if githubapi.NotFound(err) {
		if err = c.Write(http.MethodPost, "git/refs", map[string]string{"ref": "refs/tags/" + tag, "sha": commit}); err != nil {
			return err
		}
		actual, err = c.tagCommit(tag)
	}
	if err != nil {
		return err
	}
	if actual != commit {
		return fmt.Errorf("%w: tag %s conflicts with source %s", ErrMilestonePolicy, tag, commit)
	}
	return nil
}

// Draft creates notes for the already-tagged, verified source. No version is
// inferred from labels or an existing draft. Call after building the artifacts.
func (Release) Draft(repo, planFile string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client, err := newMilestoneClient(ctx, repo)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(planFile)
	if err != nil {
		return err
	}
	var p preparedRelease
	if err = json.Unmarshal(data, &p); err != nil {
		return err
	}
	id, err := client.draft(&p)
	if err != nil {
		return err
	}
	return releaseOutputs(map[string]string{"release-id": fmt.Sprint(id)})
}

func (c *milestoneClient) draft(p *preparedRelease) (int64, error) {
	if err := validatePrepared(p); err != nil {
		return 0, err
	}
	if p.Repository != c.Repo {
		return 0, fmt.Errorf("%w: invalid prepared release", ErrMilestonePolicy)
	}
	if err := c.verifyPreparedTag(p); err != nil {
		return 0, err
	}
	drafts, err := milestonePages[githubRelease](c, "releases?")
	if err != nil {
		return 0, err
	}
	for _, existing := range drafts {
		if existing.Tag != p.Tag {
			continue
		}
		if existing.Commit != p.Commit || !existing.Draft {
			return 0, fmt.Errorf("%w: existing release conflicts or is already published", ErrMilestonePolicy)
		}
		return existing.ID, nil
	}
	return c.createDraft(p)
}

// githubRelease is the identity-bearing subset of GitHub's release response.
type githubRelease struct {
	ID         int64  `json:"id"`
	Tag        string `json:"tag_name"`
	Commit     string `json:"target_commitish"`
	Draft      bool   `json:"draft"`
	Immutable  bool   `json:"immutable"`
	Prerelease bool   `json:"prerelease"`
}

func (c *milestoneClient) createDraft(p *preparedRelease) (int64, error) {
	// Start with the unformatted PR list, as release-drafter did. SummarizeNotes
	// fetches the original descriptions and renders details once; pre-rendered
	// notes would double-escape titles and summarize already-condensed text.
	// These categorized bullets remain usable if refinement is unavailable.
	body, err := releasenotes.RenderBody(p.Entries, make([]string, len(p.Entries)))
	if err != nil {
		return 0, err
	}
	payload, _ := json.Marshal(map[string]any{
		"tag_name":         p.Tag,
		"target_commitish": p.Commit,
		"name":             p.Tag,
		"body":             body,
		"draft":            true,
		"prerelease":       p.Mode != releaseModeCut,
		"make_latest":      "false",
	})
	var created githubRelease
	err = c.API.DoWithContext(c.Context, http.MethodPost, "repos/"+c.Repo+"/releases", bytes.NewReader(payload), &created)
	if err != nil {
		return 0, err
	}
	if created.ID <= 0 || created.Tag != p.Tag || created.Commit != p.Commit || !created.Draft {
		return 0, fmt.Errorf("%w: draft identity mismatch", ErrMilestonePolicy)
	}
	return created.ID, nil
}

func releaseOutputs(values map[string]string) error {
	output := os.Getenv("GITHUB_OUTPUT")
	if output == "" {
		return fmt.Errorf("%w: GITHUB_OUTPUT missing", ErrMilestonePolicy)
	}
	root, err := os.OpenRoot(filepath.Dir(output))
	if err != nil {
		return err
	}
	defer root.Close()
	file, err := root.OpenFile(filepath.Base(output), os.O_APPEND|os.O_WRONLY, workflowFileMode)
	if err != nil {
		return err
	}
	defer file.Close()
	for key, value := range values {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%w: invalid output value", ErrMilestonePolicy)
		}
		if _, err = fmt.Fprintf(file, "%s=%s\n", key, value); err != nil {
			return err
		}
	}
	return nil
}

func writePreparedPlan(path string, data []byte) error {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	return root.WriteFile(filepath.Base(path), data, workflowFileMode)
}

func readPreparedPlan(path string) (*preparedRelease, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var plan preparedRelease
	if err = json.Unmarshal(data, &plan); err != nil {
		return nil, err
	}
	if err = validatePrepared(&plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

// ValidatePlan reauthorizes the workflow and repeats eligibility checks before
// signing/publication. A prepared artifact is not permission to skip live checks.
func (Release) ValidatePlan(repo, planFile string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client, err := newMilestoneClient(ctx, repo)
	if err != nil {
		return err
	}
	plan, err := readPreparedPlan(planFile)
	if err != nil {
		return err
	}
	return client.validateLivePlan(plan, os.Getenv)
}

func (c *milestoneClient) validateLivePlan(p *preparedRelease, env func(string) string) error {
	if p.Repository != c.Repo || p.RunID != env("GITHUB_RUN_ID") || p.Mode != env("RELEASE_MODE") {
		return fmt.Errorf("%w: prepared plan belongs to another execution", ErrMilestonePolicy)
	}
	if err := c.authorize(env); err != nil {
		return err
	}
	if err := c.verifyLiveEligibility(p); err != nil {
		return err
	}
	if err := c.verifyCandidateReservation(p); err != nil {
		return err
	}
	actual, err := c.tagCommit(p.Tag)
	if err != nil {
		return err
	}
	if actual != p.Commit {
		return fmt.Errorf("%w: release tag moved", ErrMilestonePolicy)
	}
	return nil
}

func (c *milestoneClient) verifyLiveEligibility(p *preparedRelease) error {
	request := releaseRequest{Mode: p.Mode, Commit: p.Commit}
	if p.Mode == releaseModeFeature {
		request.PullRequest = fmt.Sprint(p.Entries[0].Number)
	}
	live, err := c.requestPlan(&request)
	if err != nil {
		return err
	}
	if live.Commit != p.Commit || live.Version != p.Version || live.Milestone.Number != p.Milestone.Number || live.PreviousTag != p.PreviousTag {
		return fmt.Errorf("%w: release plan changed", ErrMilestonePolicy)
	}
	return nil
}

// Complete closes the stable milestone only after successful immutable publication.
// Prereleases are a no-op. This target can retry bookkeeping without republishing.
func (Release) Complete(repo, planFile string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, err := newMilestoneClient(ctx, repo)
	if err != nil {
		return err
	}
	plan, err := readPreparedPlan(planFile)
	if err != nil {
		return err
	}
	if plan.Repository != repo {
		return fmt.Errorf("%w: prepared repository mismatch", ErrMilestonePolicy)
	}
	if plan.Mode != releaseModeCut {
		return nil
	}
	if err = client.verifyStablePublication(plan); err != nil {
		return err
	}
	return client.closePublishedMilestone(plan)
}

func (c *milestoneClient) verifyStablePublication(p *preparedRelease) error {
	var published githubRelease
	if err := c.Get("releases/tags/"+url.PathEscape(p.Tag), &published); err != nil {
		return err
	}
	if published.Tag != p.Tag || published.Draft || !published.Immutable || published.Prerelease {
		return fmt.Errorf("%w: stable immutable publication not verified", ErrMilestonePolicy)
	}
	actual, err := c.tagCommit(p.Tag)
	if err != nil {
		return err
	}
	if actual != p.Commit {
		return fmt.Errorf("%w: published tag mismatch", ErrMilestonePolicy)
	}
	return nil
}

func (c *milestoneClient) closePublishedMilestone(p *preparedRelease) error {
	var m Milestone
	if err := c.Get(fmt.Sprintf("milestones/%d", p.Milestone.Number), &m); err != nil {
		return err
	}
	if m.Title != p.Milestone.Title {
		return fmt.Errorf("%w: milestone changed after publication", ErrMilestonePolicy)
	}
	if m.State == "closed" {
		return nil
	}
	return c.Write(http.MethodPatch, fmt.Sprintf("milestones/%d", m.Number), map[string]string{"state": "closed"})
}
