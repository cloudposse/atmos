package release

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/release/milestone"
)

const taggedBuildWorkflow = ".github/workflows/release-tag-build.yml"

// taggedSigning applies to milestone versions, including their RCs, but never
// to feature/nightly builds whose workflow source is intentionally trusted main.
func taggedSigning(p *preparedRelease) bool {
	if p.Mode != releaseModeCut && p.Mode != releaseModeCandidate {
		return false
	}
	v, err := milestone.MilestoneVersion(p.Version)
	minimum, _ := milestone.MilestoneVersion("1.235.0")
	return err == nil && !v.LessThan(minimum)
}

type buildRun struct {
	ID              int64      `json:"id"`
	Title           string     `json:"display_title"`
	Path            string     `json:"path"`
	HeadSHA         string     `json:"head_sha"`
	HeadBranch      string     `json:"head_branch"`
	Event           string     `json:"event"`
	Status          string     `json:"status"`
	Conclusion      string     `json:"conclusion"`
	Attempt         int64      `json:"run_attempt"`
	Actor           buildActor `json:"actor"`
	TriggeringActor buildActor `json:"triggering_actor"`
	Repository      struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

type buildActor struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

type buildArtifact struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Expired     bool   `json:"expired"`
	WorkflowRun struct {
		ID      int64  `json:"id"`
		HeadSHA string `json:"head_sha"`
	} `json:"workflow_run"`
}

func tagBuildRequest(p *preparedRelease, artifact string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join([]string{p.Repository, p.RunID, artifact, p.Tag, p.Commit}, "\n"))))
}

func tagBuildTitle(p *preparedRelease, artifact string) string {
	return "Atmos tagged build " + p.Tag + " [" + tagBuildRequest(p, artifact) + "]"
}

// TagBuild dispatches once, follows the exact child run, and returns immutable
// artifact coordinates. A retry reuses the existing run, including its failures;
// it never silently creates another signing run after an uncertain dispatch.
func (Release) TagBuild(repo, planFile, artifact string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 190*time.Minute)
	defer cancel()
	c, err := newMilestoneClient(ctx, repo)
	if err != nil {
		return err
	}
	p, err := readPreparedPlan(planFile)
	if err != nil {
		return err
	}
	if err = c.validateLivePlan(p, os.Getenv); err != nil {
		return err
	}
	if !taggedSigning(p) {
		return fmt.Errorf("%w: release does not use tag signing", ErrReleaseVerification)
	}
	if _, err = c.checkBuildPlanArtifact(p, artifact); err != nil {
		return err
	}
	run, err := c.followTagBuild(p, artifact, os.Getenv("GITHUB_RUN_ATTEMPT"), waitReleasePoll)
	if err != nil {
		return err
	}
	asset, err := c.tagBuildArtifact(run)
	if err != nil {
		return err
	}
	return releaseOutputs(map[string]string{
		"run-id": fmt.Sprint(run.ID), "artifact": fmt.Sprint(asset.ID),
		"signing-workflow-ref": repo + "/" + taggedBuildWorkflow + "@refs/tags/" + p.Tag,
		"signing-workflow-sha": p.Commit,
	})
}

func (c *milestoneClient) followTagBuild(p *preparedRelease, artifact, attempt string, pause func(context.Context) error) (*buildRun, error) {
	run, err := c.findTagBuild(p, artifact)
	if err != nil {
		return nil, err
	}
	if run == nil {
		if err = c.dispatchTagBuild(p, artifact, attempt); err != nil {
			return nil, err
		}
	}
	for {
		// Search on every poll so duplicate request identities cannot be hidden by
		// selecting the first matching run or by pagination.
		run, err = c.findTagBuild(p, artifact)
		if err != nil {
			return nil, err
		}
		done, checkErr := c.checkTagBuildResult(p, run)
		if checkErr != nil {
			return nil, checkErr
		}
		if done {
			return run, nil
		}
		if err = pause(c.Context); err != nil {
			return nil, fmt.Errorf("tagged build may still be running; do not redispatch blindly: %w", err)
		}
	}
}

func (c *milestoneClient) dispatchTagBuild(p *preparedRelease, artifact, attempt string) error {
	if attempt != "1" {
		return fmt.Errorf("%w: no tagged build found on retry; inspect the original dispatch before starting a new release", ErrReleaseVerification)
	}
	inputs := map[string]string{"parent-run": p.RunID, "plan-artifact": artifact, "request-id": tagBuildRequest(p, artifact), "guarded-releases-enabled": "true"}
	if err := c.Write(http.MethodPost, "actions/workflows/release-tag-build.yml/dispatches", map[string]any{"ref": p.Tag, "inputs": inputs}); err != nil {
		return fmt.Errorf("tagged build dispatch uncertain for %s; inspect Actions before retrying: %w", inputs["request-id"], err)
	}
	return nil
}

func (c *milestoneClient) checkTagBuildResult(p *preparedRelease, run *buildRun) (bool, error) {
	if run == nil {
		return false, nil
	}
	if err := validateTagBuildRun(p, run); err != nil {
		return false, err
	}
	if run.Status != "completed" {
		return false, nil
	}
	if run.Conclusion != "success" {
		return false, fmt.Errorf("%w: tagged build %s finished %s; rerun that child, then retry the parent job", ErrReleaseVerification, c.releaseRunURL(run.ID), run.Conclusion)
	}
	fmt.Fprintln(os.Stdout, "Verified tagged build:", c.releaseRunURL(run.ID))
	return true, nil
}

func (c *milestoneClient) findTagBuild(p *preparedRelease, artifact string) (*buildRun, error) {
	var found *buildRun
	for page := 1; page <= githubapi.PageLimit; page++ {
		var listing struct {
			Runs []buildRun `json:"workflow_runs"`
		}
		if err := c.Get(fmt.Sprintf("actions/workflows/release-tag-build.yml/runs?event=workflow_dispatch&per_page=100&page=%d", page), &listing); err != nil {
			return nil, err
		}
		for i := range listing.Runs {
			run := &listing.Runs[i]
			if run.Title != tagBuildTitle(p, artifact) {
				continue
			}
			if found != nil && found.ID != run.ID {
				return nil, fmt.Errorf("%w: duplicate tagged build request", ErrReleaseVerification)
			}
			found = run
		}
		if len(listing.Runs) < githubapi.PageSize {
			return found, nil
		}
	}
	return nil, fmt.Errorf("%w: tagged build search pagination limit", ErrReleaseVerification)
}

func validateTagBuildRun(p *preparedRelease, run *buildRun) error {
	if run.ID <= 0 || run.Attempt <= 0 || run.Event != "workflow_dispatch" || run.Path != taggedBuildWorkflow ||
		run.HeadSHA != p.Commit || run.HeadBranch != p.Tag || run.Repository.FullName != p.Repository {
		return fmt.Errorf("%w: tagged build workflow, repository, tag or SHA mismatch", ErrReleaseVerification)
	}
	return nil
}

func validateBuildParent(p *preparedRelease, parent *buildRun) error {
	if fmt.Sprint(parent.ID) != p.RunID || parent.Repository.FullName != p.Repository || parent.Event != "workflow_dispatch" ||
		parent.Path != ".github/workflows/publish-release.yml" || parent.HeadBranch != p.Branch ||
		!githubapi.SHAPattern.MatchString(parent.HeadSHA) || !authorizedReleaseID(parent.Actor.ID) || !authorizedReleaseID(parent.TriggeringActor.ID) {
		return fmt.Errorf("%w: untrusted parent release run", ErrReleaseVerification)
	}
	return nil
}

func (c *milestoneClient) checkBuildPlanArtifact(p *preparedRelease, id string) (*buildRun, error) {
	if !canonicalRunID(id) {
		return nil, fmt.Errorf("%w: invalid plan artifact ID", ErrReleaseVerification)
	}
	var parent buildRun
	if err := c.Get("actions/runs/"+p.RunID, &parent); err != nil {
		return nil, err
	}
	if err := validateBuildParent(p, &parent); err != nil {
		return nil, err
	}
	var artifact buildArtifact
	if err := c.Get("actions/artifacts/"+id, &artifact); err != nil {
		return nil, err
	}
	prefix := "release-plan-" + p.RunID + "-"
	attempt := strings.TrimPrefix(artifact.Name, prefix)
	if fmt.Sprint(artifact.ID) != id || !validBuildArtifact(&artifact, &parent) ||
		!strings.HasPrefix(artifact.Name, prefix) || !canonicalRunID(attempt) {
		return nil, fmt.Errorf("%w: plan artifact does not belong to the parent workflow", ErrReleaseVerification)
	}
	return &parent, nil
}

func validBuildArtifact(artifact *buildArtifact, run *buildRun) bool {
	return artifact.ID > 0 && !artifact.Expired && artifact.WorkflowRun.ID == run.ID && artifact.WorkflowRun.HeadSHA == run.HeadSHA
}

func (c *milestoneClient) tagBuildArtifact(run *buildRun) (*buildArtifact, error) {
	var found *buildArtifact
	name := fmt.Sprintf("release-build-%d-%d", run.ID, run.Attempt)
	for page := 1; page <= githubapi.PageLimit; page++ {
		var listing struct {
			Artifacts []buildArtifact `json:"artifacts"`
		}
		if err := c.Get(fmt.Sprintf("actions/runs/%d/artifacts?per_page=100&page=%d", run.ID, page), &listing); err != nil {
			return nil, err
		}
		for _, artifact := range listing.Artifacts {
			if artifact.Name != name {
				continue
			}
			if found != nil || !validBuildArtifact(&artifact, run) {
				return nil, fmt.Errorf("%w: ambiguous or invalid tagged build artifact", ErrReleaseVerification)
			}
			copy := artifact
			found = &copy
		}
		if len(listing.Artifacts) < githubapi.PageSize {
			if found == nil {
				return nil, fmt.Errorf("%w: tagged build artifact missing", ErrReleaseVerification)
			}
			return found, nil
		}
	}
	return nil, fmt.Errorf("%w: build artifact pagination limit", ErrReleaseVerification)
}

// ValidateTagBuild authorizes the child against the original parent's plan. It
// does not reserve another tag or replace the plan's parent run identity.
func (Release) ValidateTagBuild(repo, planFile, artifact, parentRun, request string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	c, err := newMilestoneClient(ctx, repo)
	if err != nil {
		return err
	}
	p, err := readPreparedPlan(planFile)
	if err != nil {
		return err
	}
	if err = c.validateTagBuild(p, artifact, parentRun, request, os.Getenv); err != nil {
		return err
	}
	return releaseOutputs(map[string]string{"tag": p.Tag, "commit": p.Commit, "mode": p.Mode})
}

func (c *milestoneClient) validateTagBuild(p *preparedRelease, artifact, parentRun, request string, env func(string) string) error {
	if p.Repository != c.Repo || p.RunID != parentRun || !taggedSigning(p) || request != tagBuildRequest(p, artifact) {
		return fmt.Errorf("%w: tagged build request does not match the prepared identity", ErrReleaseVerification)
	}
	if err := validateTaggedWorkflow(p, env); err != nil {
		return err
	}
	parent, err := c.checkBuildPlanArtifact(p, artifact)
	if err != nil {
		return err
	}
	if env("GITHUB_RUN_ATTEMPT") != "1" {
		if err = c.authorizeTriggeringActor(env("GITHUB_TRIGGERING_ACTOR")); err != nil {
			return err
		}
	}
	// The App/GITHUB_TOKEN dispatch actor is not a human release approver.
	// Reauthorize the parent approver and live eligibility through the existing
	// checks without relaxing ValidatePlan for any other caller.
	return c.validateLivePlan(p, buildParentEnvironment(p, parent, env))
}

func buildParentEnvironment(p *preparedRelease, parent *buildRun, env func(string) string) func(string) string {
	return func(key string) string {
		switch key {
		case "GITHUB_RUN_ID":
			return p.RunID
		case "GITHUB_ACTOR_ID":
			return strconv.FormatInt(parent.Actor.ID, decimalBase)
		case "GITHUB_TRIGGERING_ACTOR":
			return parent.TriggeringActor.Login
		case "RELEASE_MODE":
			return p.Mode
		default:
			return env(key)
		}
	}
}

func validateTaggedWorkflow(p *preparedRelease, env func(string) string) error {
	if env("GITHUB_EVENT_NAME") != "workflow_dispatch" || env("GITHUB_REF") != "refs/tags/"+p.Tag ||
		env("GITHUB_SHA") != p.Commit || env("GITHUB_WORKFLOW_SHA") != p.Commit ||
		env("GITHUB_WORKFLOW_REF") != p.Repository+"/"+taggedBuildWorkflow+"@refs/tags/"+p.Tag {
		return fmt.Errorf("%w: tagged workflow ref or SHA differs from the prepared identity", ErrReleaseVerification)
	}
	return nil
}
