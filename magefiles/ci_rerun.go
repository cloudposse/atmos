//go:build mage

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/magefile/mage/mg"

	"github.com/cloudposse/atmos/internal/ci/rerun"
)

// CI groups targets that CI workflows dispatch to for decisions that are
// easier to keep in typed, unit-tested Go than in workflow YAML.
type CI mg.Namespace

// stuckGapEnv optionally overrides rerun.DefaultStuckGap as a Go duration
// (for example "300s"); .github/actions/classify-infra-failures sets it from
// its stuck-gap-seconds input.
const stuckGapEnv = "ATMOS_CI_STUCK_GAP"

// githubOutputDelimiter closes the multi-line `table` value written to
// GITHUB_OUTPUT; job names never contain it.
const githubOutputDelimiter = "ATMOS_CLASSIFY_EOF"

// githubFileMode is used when GITHUB_OUTPUT/GITHUB_STEP_SUMMARY do not exist
// yet (only outside the Actions runner, e.g. in tests).
const githubFileMode = 0o644

var errInvalidStuckGap = errors.New("mage: " + stuckGapEnv + " must be a positive Go duration such as 300s")

// classifyConfig bundles classifyInfraFailures' optional outputs so the
// function itself stays within revive's argument-count limit.
type classifyConfig struct {
	run         rerun.RunRef
	outputPath  string
	stuckGap    string
	summaryPath string
}

// ClassifyInfraFailures fetches every job of the repo/runID/runAttempt run
// attempt (repos/{repo}/actions/runs/{runID}/attempts/{runAttempt}/jobs, live
// via the GitHub REST API), classifies the non-success ones as infrastructure
// zombies or real failures, prints one TSV line (`job<TAB>conclusion<TAB>class`)
// per job to stdout and the verdict to stderr. When GITHUB_OUTPUT is set it
// also records `verdict` (rerun|no-rerun|no-jobs), `reason` and the
// multi-line `table` for the calling workflow step. When GITHUB_STEP_SUMMARY
// is set it also appends a Markdown rendering of the same classification. See
// internal/ci/rerun for the rules.
func (CI) ClassifyInfraFailures(repo, runID, runAttempt string) error {
	client, err := api.DefaultRESTClient()
	if err != nil {
		return fmt.Errorf("mage: create GitHub REST client: %w", err)
	}
	return classifyInfraFailures(context.Background(), client, os.Stdout, os.Stderr, &classifyConfig{
		run:         rerun.RunRef{Repo: repo, RunID: runID, RunAttempt: runAttempt},
		outputPath:  os.Getenv("GITHUB_OUTPUT"),
		stuckGap:    os.Getenv(stuckGapEnv),
		summaryPath: os.Getenv("GITHUB_STEP_SUMMARY"),
	})
}

func classifyInfraFailures(ctx context.Context, client rerun.RESTClient, stdout, stderr io.Writer, cfg *classifyConfig) error {
	opts, err := stuckGapOptions(cfg.stuckGap)
	if err != nil {
		return err
	}
	jobs, err := rerun.FetchJobs(ctx, client, cfg.run.Repo, cfg.run.RunID, cfg.run.RunAttempt)
	if err != nil {
		return err
	}

	classified := rerun.Classify(jobs, opts)
	outcome, reason := rerun.Verdict(classified)
	var table bytes.Buffer
	if err := rerun.WriteTSV(&table, classified); err != nil {
		return err
	}
	if _, err := stdout.Write(table.Bytes()); err != nil {
		return fmt.Errorf("mage: write table: %w", err)
	}
	fmt.Fprintf(stderr, "verdict: %s (%s)\n", outcome, reason)

	if cfg.outputPath != "" {
		if err := appendGitHubOutput(cfg.outputPath, outcome, reason, table.String()); err != nil {
			return err
		}
	}
	if cfg.summaryPath == "" {
		return nil
	}
	return appendGitHubStepSummary(cfg.summaryPath, cfg.run, classified, outcome, reason)
}

func stuckGapOptions(value string) (rerun.Options, error) {
	if value == "" {
		return rerun.Options{}, nil
	}
	gap, err := time.ParseDuration(value)
	if err != nil || gap <= 0 {
		return rerun.Options{}, fmt.Errorf("%w: got %q", errInvalidStuckGap, value)
	}
	return rerun.Options{StuckGap: gap}, nil
}

func appendGitHubOutput(path string, outcome rerun.Outcome, reason, table string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, githubFileMode)
	if err != nil {
		return fmt.Errorf("mage: open GITHUB_OUTPUT: %w", err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "verdict=%s\nreason=%s\ntable<<%s\n%s%s\n", outcome, reason, githubOutputDelimiter, table, githubOutputDelimiter)
	if err != nil {
		return fmt.Errorf("mage: write GITHUB_OUTPUT: %w", err)
	}
	return nil
}

func appendGitHubStepSummary(path string, run rerun.RunRef, classified []rerun.Classified, outcome rerun.Outcome, reason string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, githubFileMode)
	if err != nil {
		return fmt.Errorf("mage: open GITHUB_STEP_SUMMARY: %w", err)
	}
	defer f.Close()
	if err := rerun.WriteMarkdownSummary(f, run, classified, outcome, reason); err != nil {
		return fmt.Errorf("mage: write GITHUB_STEP_SUMMARY: %w", err)
	}
	return nil
}

// rerunParams identifies the workflow_run this rerun decision is about;
// prNumbers is the space-separated PR_NUMBERS the calling workflow builds
// with `join(github.event.workflow_run.pull_requests.*.number, ' ')`.
type rerunParams struct {
	repo      string
	runID     string
	event     string
	headSHA   string
	prNumbers []string
}

// RerunInfraFailures re-runs the failed/cancelled jobs of a workflow run
// (repos/{repo}/actions/runs/{runID}/rerun-failed-jobs, mirroring
// `gh run rerun <runID> --failed`), but only after re-verifying - for
// pull_request runs - that the run's head SHA still matches every associated
// PR's current head, skipping the rerun otherwise. This is best-effort, not a
// hard guarantee: a new push can still land in the gap between that check and
// the rerun-request call below, so a just-superseded run can occasionally get
// rerun anyway. Workflow_run triggers only fire after the triggering run has
// already finished, so there's no live "cancel this if a newer run starts"
// concurrency group to attach here the way pull_request triggers get one -
// the wasted cost of that rare case is one extra CI run for an already-stale
// commit, not a correctness problem. A fork PR (whose pull_requests list
// GitHub always leaves empty) can't be verified this way and is never rerun
// either. Every decision is appended to GITHUB_STEP_SUMMARY when set; only a
// genuine rerun-request failure (not "the run hasn't finished yet") is
// returned as an error.
func (CI) RerunInfraFailures(repo, runID, event, headSHA, prNumbers string) error {
	client, err := api.DefaultRESTClient()
	if err != nil {
		return fmt.Errorf("mage: create GitHub REST client: %w", err)
	}
	return rerunInfraFailures(context.Background(), client, os.Stdout, os.Getenv("GITHUB_STEP_SUMMARY"), &rerunParams{
		repo:      repo,
		runID:     runID,
		event:     event,
		headSHA:   headSHA,
		prNumbers: strings.Fields(prNumbers),
	})
}

func rerunInfraFailures(ctx context.Context, client rerun.RESTClient, stdout io.Writer, summaryPath string, p *rerunParams) error {
	if p.event == "pull_request" {
		superseded, reason, err := checkPRHeadsCurrent(ctx, client, p)
		if err != nil {
			return err
		}
		if superseded {
			return appendSummaryLine(stdout, summaryPath, reason)
		}
	}
	return requestRerun(ctx, client, stdout, summaryPath, p)
}

// checkPRHeadsCurrent re-verifies that every PR associated with the run is
// still at the run's head SHA, and reports why not otherwise: superseded is
// true when the run must not be rerun (no PR to verify against, or any PR
// has since moved on), and reason explains why.
func checkPRHeadsCurrent(ctx context.Context, client rerun.RESTClient, p *rerunParams) (superseded bool, reason string, err error) {
	if len(p.prNumbers) == 0 {
		return true, fmt.Sprintf("Run %s has no associated pull request (fork PR?); cannot verify it is current, not rerunning.", p.runID), nil
	}
	for _, raw := range p.prNumbers {
		number, convErr := strconv.Atoi(raw)
		if convErr != nil {
			return false, "", fmt.Errorf("mage: invalid PR number %q: %w", raw, convErr)
		}
		head, headErr := rerun.PRHeadSHA(ctx, client, p.repo, number)
		if headErr != nil {
			return false, "", headErr
		}
		if head != p.headSHA {
			return true, fmt.Sprintf("Run %s was for %s but PR #%d is now at %s; superseded, not rerunning.", p.runID, p.headSHA, number, head), nil
		}
	}
	return false, "", nil
}

// maxRerunAttempts mirrors `.github/workflows/rerun-infra-failures.yml`'s
// job-level `run_attempt < 3` if-gate. That gate reads the triggering
// event's attempt count once, when this workflow starts; by the time
// requestRerun actually calls the rerun API - after harden-runner setup,
// checkout and classification - the run's real attempt count can have moved
// (another trigger of this same workflow, or a manual rerun in the UI).
// Re-checking live here, immediately before the call that would create the
// next attempt, closes that gap instead of just narrowing it.
const maxRerunAttempts = 3

func requestRerun(ctx context.Context, client rerun.RESTClient, stdout io.Writer, summaryPath string, p *rerunParams) error {
	attempt, err := rerun.RunAttempt(ctx, client, p.repo, p.runID)
	if err != nil {
		return err
	}
	if attempt >= maxRerunAttempts {
		return appendSummaryLine(stdout, summaryPath, fmt.Sprintf("Rerun skipped: run %s is already at attempt %d (cap %d).", p.runID, attempt, maxRerunAttempts))
	}

	stillRunning, err := rerun.RerunFailedJobs(ctx, client, p.repo, p.runID)
	if err != nil {
		fmt.Fprintf(stdout, "::error::rerun failed jobs for run %s: %s\n", p.runID, err)
		return err
	}
	if stillRunning {
		fmt.Fprintf(stdout, "::warning::Run %s is still finishing; skipping rerun.\n", p.runID)
		return appendSummaryLine(stdout, summaryPath, fmt.Sprintf("Rerun skipped: run %s is still finishing.", p.runID))
	}
	return appendSummaryLine(stdout, summaryPath, fmt.Sprintf("Rerun requested for run %s.", p.runID))
}

func appendSummaryLine(stdout io.Writer, summaryPath, line string) error {
	fmt.Fprintln(stdout, line)
	if summaryPath == "" {
		return nil
	}
	f, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, githubFileMode) //nolint:gosec // Path is GITHUB_STEP_SUMMARY, set by the Actions runner, not user input.
	if err != nil {
		return fmt.Errorf("mage: open GITHUB_STEP_SUMMARY: %w", err)
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, line); err != nil {
		return fmt.Errorf("mage: write GITHUB_STEP_SUMMARY: %w", err)
	}
	return nil
}
