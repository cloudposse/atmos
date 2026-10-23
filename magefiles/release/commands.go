package release

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Check validates the stable-release plan and prints a concise operator summary.
func (Release) Check(repo string) error { return releaseCommand(repo, "check") }

// PlanJSON returns the exact plan for the native confirmation flow to capture.
func (Release) PlanJSON(repo string) error { return releaseCommand(repo, "plan-json") }

// Cut dispatches the exact plan confirmed by the Atmos UI, then follows its run.
func (Release) Cut(repo string) error { return releaseCommand(repo, releaseModeCut) }

// Candidate dispatches a prerelease without requiring a completed milestone.
func (Release) Candidate(repo string) error { return releaseCommand(repo, releaseModeCandidate) }

func releaseCommand(repo, mode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)
	defer cancel()
	client, err := newMilestoneClient(ctx, repo)
	if err != nil {
		return err
	}
	return client.command(mode, os.Stdout, waitReleasePoll)
}

func waitReleasePoll(ctx context.Context) error {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Stable numeric identities are the authorization boundary. Usernames document
// the owners; they are not matched as strings or regular expressions.
const (
	ostermanGitHubUserID int64 = 52489   // Erik Osterman (@osterman).
	aknyshGitHubUserID   int64 = 7356997 // Andriy Knysh (@aknysh).
)

func authorizedReleaseID(id int64) bool {
	return id == ostermanGitHubUserID || id == aknyshGitHubUserID
}

func (c *milestoneClient) command(mode string, out io.Writer, pause func(context.Context) error) error {
	p, err := c.commandPlan(mode, os.Getenv("RELEASE_CONFIRMED"), os.Getenv("RELEASE_APPROVED_PLAN"))
	if err != nil {
		return err
	}
	if mode == "plan-json" {
		return json.NewEncoder(out).Encode(p)
	}
	if err = writePlanSummary(out, p); err != nil {
		return err
	}
	if mode == "check" {
		return nil
	}
	request, err := c.dispatchRelease(p, mode)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Dispatched request %s for %s at %s\n", request, p.Version, p.Commit)
	runID, err := c.followRun(request, out, pause)
	if err != nil {
		return err
	}
	return c.summarizeRelease(p, runID, out)
}

func writePlanSummary(out io.Writer, p *Plan) error {
	ioCtx, err := atmosio.NewContext()
	if err != nil {
		return err
	}
	ui.InitFormatter(ioCtx)
	if _, err = fmt.Fprintf(out, "Release %s\n\n  Source    %s @ %s\n  Previous  %s\n\n", strings.TrimPrefix(p.Version, "v"), p.Branch, p.Commit, p.PreviousTag); err != nil {
		return err
	}
	ui.New(out).Success("Milestone and source checks passed.")
	return nil
}

// validateApprovedPlan binds the human's confirmation to the next live release.
// This is a local UX check, not authorization; Actions repeats the policy checks.
func validateApprovedPlan(raw string, current *Plan) error {
	var approved Plan
	if err := json.Unmarshal([]byte(raw), &approved); err != nil {
		return fmt.Errorf("%w: confirmed release plan missing or invalid; run atmos release cut again", ErrMilestonePolicy)
	}
	if approved.Mode != releaseModeCut || current.Mode != releaseModeCut || approved.identity() != current.identity() {
		return fmt.Errorf("%w: release plan changed after confirmation; run atmos release cut again", ErrMilestonePolicy)
	}
	return nil
}

// followRun discovers only this dispatch, then polls that run until completion.
// An interrupted wait never submits a replacement workflow.
func (c *milestoneClient) followRun(request string, out io.Writer, pause func(context.Context) error) (int64, error) {
	var runID int64
	for {
		if err := c.Context.Err(); err != nil {
			return runID, fmt.Errorf("%s continues remotely; inspect its run before retrying: %w", c.releaseRunContext(request, runID), err)
		}
		if runID == 0 {
			found, err := c.findDispatchedRun(request)
			if err != nil {
				return 0, err
			}
			runID = found
			if runID != 0 {
				fmt.Fprintln(out, c.releaseRunURL(runID))
			}
		}
		complete, err := c.releaseRunComplete(runID)
		if err != nil {
			return runID, err
		}
		if complete {
			fmt.Fprintf(out, "Release workflow succeeded: %s\n", c.releaseRunURL(runID))
			return runID, nil
		}
		if err = pause(c.Context); err != nil {
			return runID, fmt.Errorf("%s remains dispatched: %w", c.releaseRunContext(request, runID), err)
		}
	}
}

type dispatchedRun struct {
	ID    int64  `json:"id"`
	Title string `json:"display_title"`
	Event string `json:"event"`
}

func (c *milestoneClient) findDispatchedRun(request string) (int64, error) {
	for page := 1; page <= githubapi.PageLimit; page++ {
		var listing struct {
			Runs []dispatchedRun `json:"workflow_runs"`
		}
		err := c.Get(fmt.Sprintf("actions/workflows/publish-release.yml/runs?event=workflow_dispatch&per_page=100&page=%d", page), &listing)
		if err != nil {
			return 0, fmt.Errorf("cannot follow request %s (do not redispatch blindly): %w", request, err)
		}
		id, err := matchingRunID(request, listing.Runs)
		if err != nil || id != 0 {
			return id, err
		}
		if len(listing.Runs) < githubapi.PageSize {
			return 0, nil
		}
	}
	return 0, nil
}

func matchingRunID(request string, runs []dispatchedRun) (int64, error) {
	var id int64
	for _, run := range runs {
		if run.Title != "Atmos release "+request || run.Event != "workflow_dispatch" {
			continue
		}
		if id != 0 && id != run.ID {
			return 0, fmt.Errorf("%w: duplicate dispatch identity", ErrMilestonePolicy)
		}
		id = run.ID
	}
	return id, nil
}

func (c *milestoneClient) releaseRunComplete(runID int64) (bool, error) {
	if runID == 0 {
		return false, nil
	}
	var run struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}
	if err := c.Get(fmt.Sprintf("actions/runs/%d", runID), &run); err != nil {
		return false, fmt.Errorf("cannot follow %s; inspect it before retrying: %w", c.releaseRunURL(runID), err)
	}
	if run.Status != "completed" {
		return false, nil
	}
	if run.Conclusion != "success" {
		return false, fmt.Errorf("%w: release run %d finished %s: %s", ErrMilestonePolicy, runID, run.Conclusion, c.releaseRunURL(runID))
	}
	return true, nil
}

func (c *milestoneClient) releaseRunURL(runID int64) string {
	return fmt.Sprintf("https://github.com/%s/actions/runs/%d", c.Repo, runID)
}

func (c *milestoneClient) releaseRunContext(request string, runID int64) string {
	if runID == 0 {
		return "request " + request
	}
	return "request " + request + " (" + c.releaseRunURL(runID) + ")"
}

// summarizeRelease reports publication only after reading the exact immutable
// release produced by the correlated run. A successful unrelated run or draft
// must never produce a published-release confirmation.
func (c *milestoneClient) summarizeRelease(p *Plan, runID int64, out io.Writer) error {
	tag := p.Version
	if p.Mode == releaseModeCandidate {
		_, reserved, err := c.candidateReservations(&preparedRelease{Plan: *p, RunID: fmt.Sprint(runID)})
		if err != nil {
			return fmt.Errorf("cannot verify RC for workflow %s: %w", c.releaseRunURL(runID), err)
		}
		if reserved == "" {
			return fmt.Errorf("%w: workflow %s has no reserved RC", ErrReleaseVerification, c.releaseRunURL(runID))
		}
		tag = reserved
	}
	var published struct {
		Tag        string `json:"tag_name"`
		Commit     string `json:"target_commitish"`
		Draft      bool   `json:"draft"`
		Immutable  bool   `json:"immutable"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := c.Get("releases/tags/"+url.PathEscape(tag), &published); err != nil {
		return fmt.Errorf("workflow %s succeeded, but published release %s could not be verified: %w", c.releaseRunURL(runID), tag, err)
	}
	if published.Tag != tag || published.Commit != p.Commit || published.Draft || !published.Immutable || published.Prerelease != (p.Mode == releaseModeCandidate) {
		return fmt.Errorf("%w: workflow %s succeeded, but published release %s does not match the selected identity", ErrReleaseVerification, c.releaseRunURL(runID), tag)
	}
	_, err := fmt.Fprintf(out, "Published release: %s\nVersion: %s\nCommit: %s\nRelease: https://github.com/%s/releases/tag/%s\nWorkflow: %s\n", tag, strings.TrimPrefix(tag, "v"), p.Commit, c.Repo, tag, c.releaseRunURL(runID))
	return err
}

const dispatchIDBytes = 16

func (c *milestoneClient) dispatchRelease(p *Plan, mode string) (string, error) {
	var user struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	}
	if err := c.API.DoWithContext(c.Context, http.MethodGet, "user", nil, &user); err != nil {
		return "", err
	}
	if user.Type != "User" || !authorizedReleaseID(user.ID) {
		return "", fmt.Errorf("%w: only Erik or Andriy may dispatch releases", ErrMilestonePolicy)
	}
	random := make([]byte, dispatchIDBytes)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	request := hex.EncodeToString(random)
	inputs := map[string]string{"milestone": fmt.Sprint(p.Milestone.Number), "version": p.Version, "commit": p.Commit, "mode": mode, "request-id": request}
	if err := c.Write(http.MethodPost, "actions/workflows/publish-release.yml/dispatches", map[string]any{"ref": p.Branch, "inputs": inputs}); err != nil {
		return "", fmt.Errorf("dispatch result uncertain for request %s: inspect workflow runs before retrying: %w", request, err)
	}
	return request, nil
}

// Only release identity is confirmed; rendered prose is allowed to change.
type confirmedReleaseIdentity struct {
	repository      string
	milestoneNumber int
	milestoneTitle  string
	version         string
	commit          string
	previousTag     string
	branch          string
}

func (plan *Plan) identity() confirmedReleaseIdentity {
	return confirmedReleaseIdentity{
		repository:      plan.Repository,
		milestoneNumber: plan.Milestone.Number,
		milestoneTitle:  plan.Milestone.Title,
		version:         plan.Version,
		commit:          plan.Commit,
		previousTag:     plan.PreviousTag,
		branch:          plan.Branch,
	}
}

// commandPlan repeats local preflight and binds a cut to its confirmed identity.
func (c *milestoneClient) commandPlan(mode, confirmed, approved string) (*Plan, error) {
	if mode == releaseModeCut && confirmed != "true" {
		return nil, fmt.Errorf("%w: release was not confirmed; run atmos release cut", ErrMilestonePolicy)
	}
	planMode := mode
	if mode == "check" || mode == "plan-json" {
		planMode = releaseModeCut
	}
	p, err := c.plan(planMode, "")
	if err != nil {
		return nil, err
	}
	if mode == releaseModeCut {
		if err = validateApprovedPlan(approved, p); err != nil {
			return nil, err
		}
	}
	return p, nil
}
