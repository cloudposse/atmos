package release

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/release/milestone"
)

// releaseRecovery binds confirmation to an existing execution and its saved plan.
type releaseRecovery struct {
	RunID      int64 `json:"run_id"`
	Attempt    int   `json:"attempt"`
	ArtifactID int64 `json:"artifact_id"`
}

type recoveryRun struct {
	githubapi.Run
	Title string `json:"display_title"`
}

type recoveryArtifact struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Expired bool   `json:"expired"`
}

// recoveryPlan reads the original source from the reserved tag, never today's main.
// The saved preparation artifact proves which execution owns that release plan.
func (c *milestoneClient) recoveryPlan() (*Plan, error) {
	milestones, err := c.ListMilestones()
	if err != nil {
		return nil, err
	}
	next, err := milestone.NextMilestone(milestones)
	if err != nil {
		return nil, err
	}
	version, err := milestone.MilestoneVersion(next.Title)
	if err != nil {
		return nil, err
	}
	tag := "v" + version.String()
	commit, err := c.tagCommit(tag)
	if err != nil {
		return nil, fmt.Errorf("cannot resume %s without its original reserved tag: %w", tag, err)
	}
	if err = c.requireUnpublished(tag, commit); err != nil {
		return nil, err
	}
	p, err := c.plan(releaseModeCut, commit)
	if err != nil {
		return nil, err
	}
	recovery, err := c.findRecovery(p)
	if err != nil {
		return nil, err
	}
	p.Resume = recovery
	return p, nil
}

// requireUnpublished finds the reserved tag's release through the releases list:
// GitHub's tag endpoint returns published releases only, so it cannot see the
// draft that recovery must identity-check. The list includes drafts for callers
// with push access, which the authorized release operators have.
func (c *milestoneClient) requireUnpublished(tag, commit string) error {
	releases, err := milestonePages[githubRelease](c, "releases?")
	if err != nil {
		return err
	}
	release, err := releaseForTag(releases, tag)
	if err != nil {
		return err
	}
	if release == nil {
		return nil // Preparation/build may have failed before creating the draft.
	}
	if !release.Draft {
		return fmt.Errorf("%w: %s is already published; --resume never modifies a published release", ErrMilestonePolicy, tag)
	}
	if release.Commit != commit || release.Prerelease || release.Immutable {
		return fmt.Errorf("%w: draft identity differs from the reserved production release", ErrMilestonePolicy)
	}
	return nil
}

// releaseForTag returns the only release with the tag, or nil when none exists.
func releaseForTag(releases []githubRelease, tag string) (*githubRelease, error) {
	var found *githubRelease
	for i := range releases {
		if releases[i].Tag != tag {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("%w: multiple releases exist for %s", ErrMilestonePolicy, tag)
		}
		found = &releases[i]
	}
	return found, nil
}

func (c *milestoneClient) findRecovery(p *Plan) (*releaseRecovery, error) {
	var found *releaseRecovery
	for page := 1; page <= githubapi.PageLimit; page++ {
		var listing struct {
			Runs []recoveryRun `json:"workflow_runs"`
		}
		path := fmt.Sprintf("actions/workflows/publish-release.yml/runs?event=workflow_dispatch&per_page=100&page=%d", page)
		if err := c.Get(path, &listing); err != nil {
			return nil, err
		}
		for i := range listing.Runs {
			run := &listing.Runs[i]
			if !strings.HasPrefix(run.Title, "Atmos release "+p.Version+" [") || !releaseRunTitle.MatchString(run.Title) {
				continue
			}
			recovery, err := c.recoveryForRun(run, p)
			if err != nil {
				return nil, err
			}
			if found != nil {
				return nil, fmt.Errorf("%w: multiple executions match %s; inspect their runs before resuming", ErrMilestonePolicy, p.Version)
			}
			found = recovery
		}
		if len(listing.Runs) < githubapi.PageSize {
			if found == nil {
				return nil, fmt.Errorf("%w: no original release workflow found for %s", ErrMilestonePolicy, p.Version)
			}
			return found, nil
		}
	}
	return nil, fmt.Errorf("%w: release workflow pagination limit exceeded", ErrMilestonePolicy)
}

func (c *milestoneClient) recoveryForRun(run *recoveryRun, p *Plan) (*releaseRecovery, error) {
	if err := validateRecoveryRun(run, p); err != nil {
		return nil, err
	}
	artifact, err := c.recoveryArtifact(run)
	if err != nil {
		return nil, err
	}
	prepared, err := c.downloadRecoveryPlan(artifact.ID)
	if err != nil {
		return nil, err
	}
	if prepared.Resume != nil || prepared.Mode != releaseModeCut || prepared.Tag != p.Version ||
		prepared.RunID != fmt.Sprint(run.ID) || prepared.identity() != p.identity() {
		return nil, fmt.Errorf("%w: workflow %d preparation does not match the reserved release", ErrMilestonePolicy, run.ID)
	}
	return &releaseRecovery{RunID: run.ID, Attempt: run.Attempt, ArtifactID: artifact.ID}, nil
}

func validateRecoveryRun(run *recoveryRun, p *Plan) error {
	if run.ID <= 0 || run.Attempt <= 0 || run.Event != "workflow_dispatch" ||
		run.Path != ".github/workflows/publish-release.yml" || run.Branch != p.Branch ||
		run.HeadRepository.FullName != p.Repository {
		return fmt.Errorf("%w: original release workflow identity could not be verified", ErrMilestonePolicy)
	}
	if run.Status != "completed" {
		switch run.Status {
		case "queued", "in_progress", "waiting", "pending", "requested":
			return nil
		}
	} else {
		switch run.Conclusion {
		case "failure", "cancelled", "timed_out", "startup_failure", "action_required":
			return nil
		}
	}
	return fmt.Errorf("%w: workflow %d is %s/%s, not an unfinished release", ErrMilestonePolicy, run.ID, run.Status, run.Conclusion)
}

func (c *milestoneClient) recoveryArtifact(run *recoveryRun) (*recoveryArtifact, error) {
	var found *recoveryArtifact
	latest := 0
	for page := 1; page <= githubapi.PageLimit; page++ {
		var listing struct {
			Artifacts []recoveryArtifact `json:"artifacts"`
		}
		if err := c.Get(fmt.Sprintf("actions/runs/%d/artifacts?per_page=100&page=%d", run.ID, page), &listing); err != nil {
			return nil, err
		}
		var err error
		found, latest, err = selectRecoveryArtifact(run, listing.Artifacts, found, latest)
		if err != nil {
			return nil, err
		}
		if len(listing.Artifacts) < githubapi.PageSize {
			if found == nil || found.Expired {
				return nil, fmt.Errorf("%w: original preparation artifact missing or expired for %s; automatic resume is unavailable", ErrMilestonePolicy, c.releaseRunURL(run.ID))
			}
			return found, nil
		}
	}
	return nil, fmt.Errorf("%w: preparation artifact pagination limit exceeded", ErrMilestonePolicy)
}

func selectRecoveryArtifact(run *recoveryRun, artifacts []recoveryArtifact, found *recoveryArtifact, latest int) (*recoveryArtifact, int, error) {
	prefix := fmt.Sprintf("release-plan-%d-", run.ID)
	for _, artifact := range artifacts {
		if !strings.HasPrefix(artifact.Name, prefix) {
			continue
		}
		attempt, err := strconv.Atoi(strings.TrimPrefix(artifact.Name, prefix))
		if err != nil || attempt < 1 || attempt > run.Attempt || artifact.ID <= 0 {
			return nil, 0, fmt.Errorf("%w: invalid preparation artifact identity", ErrMilestonePolicy)
		}
		if attempt == latest {
			return nil, 0, fmt.Errorf("%w: duplicate preparation artifacts", ErrMilestonePolicy)
		}
		if attempt > latest {
			copy := artifact
			found, latest = &copy, attempt
		}
	}
	return found, latest, nil
}

const recoveryPlanLimit = 8 * 1024 * 1024

func (c *milestoneClient) downloadRecoveryPlan(id int64) (*preparedRelease, error) {
	response, err := c.API.RequestWithContext(c.Context, http.MethodGet, fmt.Sprintf("repos/%s/actions/artifacts/%d/zip", c.Repo, id), nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: preparation artifact download returned HTTP %d", ErrMilestonePolicy, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, recoveryPlanLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > recoveryPlanLimit {
		return nil, fmt.Errorf("%w: preparation archive too large", ErrMilestonePolicy)
	}
	return decodeRecoveryPlan(data)
}

func decodeRecoveryPlan(data []byte) (*preparedRelease, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(archive.File) != 1 || archive.File[0].Name != "release-plan.json" || !archive.File[0].Mode().IsRegular() {
		return nil, fmt.Errorf("%w: unexpected preparation archive contents", ErrMilestonePolicy)
	}
	reader, err := archive.File[0].Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	contents, err := io.ReadAll(io.LimitReader(reader, recoveryPlanLimit+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > recoveryPlanLimit {
		return nil, fmt.Errorf("%w: preparation plan too large", ErrMilestonePolicy)
	}
	var prepared preparedRelease
	if err = json.Unmarshal(contents, &prepared); err != nil {
		return nil, err
	}
	if err = validatePrepared(&prepared); err != nil {
		return nil, err
	}
	return &prepared, nil
}

func (c *milestoneClient) resumeRelease(p *Plan, out io.Writer, pause func(context.Context) error) error {
	if err := c.authorizeOperator(); err != nil {
		return err
	}
	if err := c.requireUnpublished(p.Version, p.Commit); err != nil {
		return err
	}
	run, err := c.confirmedRecoveryRun(p)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Resuming %s at %s\n%s\n", p.Version, p.Commit, c.releaseRunURL(run.ID))
	if run.Status == "completed" {
		if err := c.requestRecovery(run.ID); err != nil {
			return fmt.Errorf("resume result uncertain; inspect %s before retrying: %w", c.releaseRunURL(run.ID), err)
		}
		if err := c.waitRecoveryAttempt(run, pause); err != nil {
			return err
		}
	}
	if err := c.watchRun(run.ID, out); err != nil {
		return err
	}
	return c.summarizeRelease(p, run.ID, out)
}

func (c *milestoneClient) confirmedRecoveryRun(p *Plan) (*recoveryRun, error) {
	var run recoveryRun
	if err := c.Get(fmt.Sprintf("actions/runs/%d", p.Resume.RunID), &run); err != nil {
		return nil, err
	}
	if err := validateRecoveryRun(&run, p); err != nil {
		return nil, err
	}
	if run.ID != p.Resume.RunID || run.Attempt != p.Resume.Attempt {
		return nil, fmt.Errorf("%w: workflow changed after confirmation; run atmos release cut --resume again", ErrMilestonePolicy)
	}
	return &run, nil
}

func (c *milestoneClient) requestRecovery(runID int64) error {
	path := fmt.Sprintf("repos/%s/actions/runs/%d/rerun-failed-jobs", c.Repo, runID)
	response, err := c.API.RequestWithContext(c.Context, http.MethodPost, path, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	// GitHub returns an empty 201 response, not JSON.
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("%w: release rerun returned HTTP %d", ErrMilestonePolicy, response.StatusCode)
	}
	return nil
}

func (c *milestoneClient) waitRecoveryAttempt(previous *recoveryRun, pause func(context.Context) error) error {
	const maxRerunPolls = 12
	for range maxRerunPolls {
		var run recoveryRun
		if err := c.Get(fmt.Sprintf("actions/runs/%d", previous.ID), &run); err != nil {
			return fmt.Errorf("rerun requested; inspect %s: %w", c.releaseRunURL(previous.ID), err)
		}
		if run.ID == previous.ID && run.Attempt > previous.Attempt {
			return nil
		}
		if err := pause(c.Context); err != nil {
			return fmt.Errorf("rerun requested; reconnect to %s: %w", c.releaseRunURL(previous.ID), err)
		}
	}
	return fmt.Errorf("%w: rerun requested but not yet observed; inspect %s before retrying", ErrMilestonePolicy, c.releaseRunURL(previous.ID))
}
