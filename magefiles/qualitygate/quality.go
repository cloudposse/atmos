package qualitygate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/magefile/mage/mg"

	"github.com/cloudposse/atmos/magefiles/bench"
	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	iolayer "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Quality maintains the composite PR quality report and readiness labels.
type Quality mg.Namespace

const (
	qualityMarker           = "<!-- atmos-quality -->"
	qualityReady            = "quality:ready"
	qualityNotReady         = "quality:not-ready"
	qualityPerformanceStart = "<!-- atmos-quality-performance:start -->"
	qualityPerformanceEnd   = "<!-- atmos-quality-performance:end -->"
	qualityBenchmarkError   = "<!-- atmos-benchmark-report-error -->"
	qualityCIPending        = "<!-- atmos-quality-ci-pending -->"
)

type qualityGate struct {
	ID, Name, Detail, URL, Remediation string
	Section                            string
	Problem, NextAction                string
	Signals                            []qualitySignal
	Advisory                           bool
	Passed                             bool
	// Unavailable marks a gate whose evidence could not be read. It is missing
	// evidence, not a verdict, so it must not replace a report for the same revision.
	Unavailable bool
}

// Signals are evidence from GitHub, never instructions for an agent.
type qualitySignal struct {
	Name, State string
}

type qualityClient struct {
	ctx                   context.Context
	api                   githubapi.API
	repo                  string
	benchmarkReportFailed bool
	releaseEvidence       *releaseEvidenceCache
	// ciIncomplete publishes one pending snapshot, then waits for the final CI report.
	ciIncomplete bool
	// prTimeout overrides qualityPRTimeout for each PR's reconcile.
	prTimeout time.Duration
	// trueUp permits selecting every open PR for an event that names none.
	trueUp bool
}

func (q *qualityClient) get(path string, response any) error {
	return githubapi.Get(q.ctx, q.api, "repos/"+q.repo+"/"+path, response)
}

// Report refreshes only the PRs associated with the current Actions event, such
// as a label change or a CI completion for their head. Repository-wide events
// belong to Reconcile, so a per-PR run never waits behind a true-up.
func (Quality) Report(repo string) error {
	if qualityRepositoryEvent() {
		fmt.Println("Quality report: repository-wide event; quality:reconcile handles it")
		return nil
	}
	return runQuality(repo, false)
}

// Reconcile trues up every PR a repository-wide event can affect. Scheduled runs
// reconcile PRs with a recent head commit or no report for their current head and
// base; merges, milestone and release changes, and manual runs reconcile all open
// PRs. One failing PR never blocks the rest.
func (Quality) Reconcile(repo string) error {
	if !qualityRepositoryEvent() {
		return bench.Error("quality:reconcile handles repository-wide events only; use quality:report")
	}
	return runQuality(repo, true)
}

func runQuality(repo string, trueUp bool) error {
	started := time.Now().UTC()
	if !githubapi.RepoPattern.MatchString(repo) {
		return bench.Error("invalid repository")
	}
	client, err := api.DefaultRESTClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), qualityRunBudget)
	defer cancel()
	q := qualityClient{ctx: ctx, api: client, repo: repo, trueUp: trueUp}
	prs, err := q.eventPRs(started)
	if err != nil {
		return err
	}
	return q.reconcileAll(prs)
}

// qualityRepositoryEvent reports whether the current event changes state shared
// by every open PR: time-based recovery, a merge, release metadata, or a manual
// true-up. An unreadable event payload is left to eventPRs to report.
func qualityRepositoryEvent() bool {
	switch os.Getenv("GITHUB_EVENT_NAME") {
	case "schedule", "workflow_dispatch", "milestone", "release":
		return true
	case "pull_request_target":
		var event struct {
			Action string `json:"action"`
		}
		data, err := os.ReadFile(os.Getenv("GITHUB_EVENT_PATH")) // #nosec G703 -- GitHub supplies the event file path.
		return err == nil && json.Unmarshal(data, &event) == nil && event.Action == "closed"
	default:
		return false
	}
}

// eventPRs resolves event-associated PRs, scheduled recovery candidates, or all eligible open PRs.
//
//nolint:revive // Explicit event precedence handles reviews, external checks, and workflow runs.
func (q *qualityClient) eventPRs(now time.Time) ([]int, error) {
	if os.Getenv("GITHUB_EVENT_NAME") == "schedule" {
		cutoff := now.UTC().Add(-24 * time.Hour)
		fmt.Printf("Scheduled quality recovery: cutoff %s\n", cutoff.Format(time.RFC3339Nano))
		prs, err := q.recentPRNumbers(cutoff)
		if err == nil {
			fmt.Printf("Scheduled quality recovery: selected %d PRs\n", len(prs))
		}
		return prs, err
	}
	var event struct {
		Action string `json:"action"`
		Number int    `json:"number"`
		Issue  struct {
			Number      int             `json:"number"`
			PullRequest json.RawMessage `json:"pull_request"`
		} `json:"issue"`
		WorkflowRun githubapi.Run `json:"workflow_run"`
		CheckRun    struct {
			HeadSHA string `json:"head_sha"`
		} `json:"check_run"`
		SHA string `json:"sha"`
	}
	if path := os.Getenv("GITHUB_EVENT_PATH"); path != "" {
		data, err := os.ReadFile(path) // #nosec G703 -- GitHub supplies the event file path.
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
	}
	// A merged PR changes release viability for every remaining open PR.
	if os.Getenv("GITHUB_EVENT_NAME") == "pull_request_target" && event.Action == "closed" {
		return q.prNumbers("pulls?state=open", "")
	}
	if event.Issue.Number > 0 && len(event.Issue.PullRequest) > 0 {
		return []int{event.Issue.Number}, nil
	}
	if event.Number > 0 {
		return []int{event.Number}, nil
	}
	return q.eventCommitPRs(event.SHA, event.CheckRun.HeadSHA, &event.WorkflowRun)
}

func (q *qualityClient) eventCommitPRs(eventSHA, checkSHA string, run *githubapi.Run) ([]int, error) {
	sha := eventSHA
	if checkSHA != "" {
		sha = checkSHA
	}
	if run.HeadSHA != "" {
		sha = run.HeadSHA
	}
	if sha == "" {
		if !q.trueUp {
			return nil, bench.Error("event names no pull request or commit; use quality:reconcile")
		}
		return q.prNumbers("pulls?state=open", "")
	}
	if !githubapi.SHAPattern.MatchString(sha) {
		return nil, bench.Error("invalid event SHA")
	}
	numbers, err := q.prNumbers("commits/"+sha+"/pulls?state=open", "")
	if err != nil || len(numbers) > 0 {
		return numbers, err
	}
	// Commits that exist only in a fork are not listed by the commit association
	// endpoint, so resolve them through the fork's head branch or a SHA search.
	// Only workflow_run payloads carry the head repository and branch.
	owner, branch := "", ""
	if run.HeadSHA == sha {
		owner, _, _ = strings.Cut(run.HeadRepository.FullName, "/")
		branch = run.Branch
	}
	return q.unlistedPRNumbers(sha, owner, branch)
}

func (q *qualityClient) evaluate(pr *githubapi.PR) []qualityGate {
	registry, err := defaultQualityRegistry()
	if err != nil {
		return []qualityGate{{Name: "Quality gate configuration", Detail: err.Error(), Remediation: "Repair the trusted quality gate registry before evaluating readiness."}}
	}
	return registry.evaluate(q, pr)
}

const qualityFileLimit = 150

func qualityFileCount(pr *githubapi.PR) (qualityGate, error) {
	if pr.ChangedFiles == nil || *pr.ChangedFiles < 0 {
		return qualityGate{Detail: "Changed-file count unavailable", Problem: "GitHub did not return a changed-file count.", NextAction: "Retry the PR metadata lookup before changing the PR's scope."}, nil
	}
	gate := qualityGate{Passed: *pr.ChangedFiles < qualityFileLimit, Detail: fmt.Sprintf("%d changed files; maximum 149", *pr.ChangedFiles)}
	if !gate.Passed {
		gate.Problem = fmt.Sprintf("This PR changes %d files, exceeding CodeRabbit's review limit of 149.", *pr.ChangedFiles)
		gate.NextAction = fmt.Sprintf("Move at least %d changed files into a separate PR to reach at most 149 files, then request review again.", *pr.ChangedFiles-qualityFileLimit+1)
	}
	return gate, nil
}

// Recover benchmark reports missed by events or a full Actions concurrency queue.
// The schedule retries publication even during a busy CI burst.
func (q *qualityClient) reconcile(number int) error {
	var pr githubapi.PR
	if err := q.get(fmt.Sprintf("pulls/%d", number), &pr); err != nil {
		return err
	}
	if !qualityEligible(&pr, q.repo) {
		return nil
	}
	complete, err := q.reportingComplete(&pr)
	if err != nil {
		return err
	}
	if !complete {
		ui.New(iolayer.UI).Writef("PR #%d: ensuring a pending report; deferring further updates until CI completes\n", number)
		pending := *q
		pending.ciIncomplete = true
		return pending.refresh(number, "", nil)
	}
	var response struct {
		Runs []githubapi.Run `json:"workflow_runs"`
	}
	path := fmt.Sprintf("actions/workflows/%s/runs?head_sha=%s&event=pull_request&per_page=1", bench.Workflow, pr.Head.SHA)
	if err := q.get(path, &response); err != nil {
		return err
	}
	existing, err := q.existing(number)
	if err != nil {
		return err
	}
	if len(response.Runs) > 0 {
		run := &response.Runs[0]
		if bench.PRMatches(&pr, run, q.repo) && run.Status == "completed" && !qualityReported(existing, &pr, run) {
			return q.recoverBenchmarkReport(number, run)
		}
	}
	return q.refresh(number, "", nil)
}

// Keep evidence fresh after a reporting failure without marking the run handled.
// Refresh retains the original run so stale heads and newer attempts cannot be overwritten.
func (q *qualityClient) recoverBenchmarkReport(number int, run *githubapi.Run) error {
	published := 0
	err := bench.ReportRun(q.ctx, q.api, q.repo, run.ID, func(n int, body string, reported *githubapi.Run) error {
		if n == number {
			published++
		}
		return q.refresh(n, body, reported)
	})
	if err == nil {
		if published > 0 {
			return nil
		}
		// ReportRun found no PR to publish for. Never leave the gate unpublished
		// because of an association gap: publish with performance pending.
		fmt.Printf("::warning title=Quality gate benchmark recovery::PR #%d: benchmark run %d produced no report; publishing the quality gate without performance results\n", number, run.ID)
		return q.refresh(number, "", nil)
	}
	performance := qualityBenchmarkError + "\nComparison unavailable: benchmark report processing failed. See the reporter job logs; reconciliation will retry."
	refreshErr := q.refresh(number, performance, run)
	return errors.Join(fmt.Errorf("benchmark report: %w", err), refreshErr)
}

func qualityReported(existing *githubapi.Comment, pr *githubapi.PR, run *githubapi.Run) bool {
	if existing == nil || !strings.Contains(existing.Body, qualityHeadMarker(pr.Head.SHA)) {
		return false
	}
	if qualityNewer(existing.Body, run) {
		return true
	}
	marker := fmt.Sprintf("<!-- atmos-benchmark-run:%d:%d -->", run.ID, run.Attempt)
	return strings.Contains(existing.Body, marker) && !strings.Contains(existing.Body, qualityBenchmarkError)
}
