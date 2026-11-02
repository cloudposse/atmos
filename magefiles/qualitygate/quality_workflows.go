package qualitygate

import (
	"fmt"

	"github.com/cloudposse/atmos/magefiles/bench"
	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

type qualityWorkflowRun struct {
	githubapi.Run
	CheckSuiteID int64 `json:"check_suite_id"`
}

const (
	releaseMilestonesWorkflow = ".github/workflows/release-milestones.yml"
	qualityReportWorkflow     = ".github/workflows/quality-report.yml"
	qualityReconcileWorkflow  = ".github/workflows/quality-reconcile.yml"
	qualitySignalWorkflow     = ".github/workflows/quality-signal.yml"
)

// reporterRun reports whether a run is the reporter's own plumbing rather than
// evidence about the PR.
//
// Repository reconciliation is replaceable background work. Live release policy
// and the current Release milestone commit status remain authoritative gates.
// The reporter's own runs are not evidence either: GitHub cancels a pending run
// replaced in its concurrency group, and the report it would have written is
// the one being evaluated.
//
// The Quality Gate Signal job only wakes the trusted coordinator. On fork PRs,
// GitHub holds its bot-triggered review runs for maintainer approval
// indefinitely, so counting them would block a green PR on a no-op job.
//
// Each path qualifies only at the events that workflow runs on; a PR-defined
// workflow cannot opt out of CI. A PR able to rename a failing workflow into the
// signal path could equally delete it; required checks defend against that,
// not this reporter.
func (r *qualityWorkflowRun) reporterRun() bool {
	switch r.Path {
	case releaseMilestonesWorkflow, qualityReportWorkflow, qualityReconcileWorkflow:
		switch r.Event {
		case "pull_request_target", "issues", "milestone", "release", "merge_group", "schedule", "workflow_dispatch":
			return true
		}
	case qualitySignalWorkflow:
		switch r.Event {
		case "pull_request", "pull_request_review", "pull_request_review_comment":
			return true
		}
	}
	return false
}

// workflowStates selects each workflow by creation time and ID, independent of
// API order. It also identifies obsolete Actions suites from the same response.
// Reporter runs never become a path's current run, so an older run of the same
// path from an event outside the reporter's own set remains evidence.
func (q *qualityClient) workflowStates(sha string, states map[string]string, superseded map[int64]bool) error {
	runs, err := q.workflowRuns(sha)
	if err != nil {
		return err
	}
	latest := map[string]qualityWorkflowSelection{}
	for i := range runs {
		run := &runs[i]
		if run.reporterRun() {
			continue
		}
		selection := latest[run.Path]
		selection.include(run)
		latest[run.Path] = selection
	}
	currentSuites := map[int64]bool{}
	for path, selection := range latest {
		run := selection.current()
		state := run.Status
		if state == "completed" {
			state = run.Conclusion
		}
		states["workflow: "+path] = state
		currentSuites[run.CheckSuiteID] = true
	}
	for i := range runs {
		run := &runs[i]
		selection := latest[run.Path]
		current := selection.current()
		// Suite identity, never a job name such as "enforce", proves which
		// checks belong to the background workflow. Unknown/shared suites stay.
		background := run.reporterRun() && run.ID > 0 && run.CheckSuiteID > 0 && !currentSuites[run.CheckSuiteID]
		if superseded != nil && (background || run.supersededBy(current, currentSuites)) {
			superseded[run.CheckSuiteID] = true
		}
	}
	return nil
}

// supersededBy requires enough metadata to prove that this suite is obsolete.
// Shared or missing identifiers must not hide a potentially current failure.
// A path with only reporter runs has no current run.
func (r *qualityWorkflowRun) supersededBy(current *qualityWorkflowRun, currentSuites map[int64]bool) bool {
	return current != nil && r.Path != "" && r.ID > 0 && current.ID > 0 && r.ID != current.ID &&
		r.CheckSuiteID > 0 && current.CheckSuiteID > 0 && !currentSuites[r.CheckSuiteID]
}

func (q *qualityClient) workflowRuns(sha string) ([]qualityWorkflowRun, error) {
	var runs []qualityWorkflowRun
	for page := 1; page <= githubapi.PageLimit; page++ {
		var response struct {
			Runs []qualityWorkflowRun `json:"workflow_runs"`
		}
		if err := q.get(fmt.Sprintf("actions/runs?head_sha=%s&per_page=100&page=%d", sha, page), &response); err != nil {
			return nil, err
		}
		for i := range response.Runs {
			run := &response.Runs[i]
			if run.HeadSHA == sha && run.Event != "workflow_run" {
				runs = append(runs, *run)
			}
		}
		if len(response.Runs) < githubapi.PageSize {
			return runs, nil
		}
	}
	return nil, bench.Error("workflow pagination limit exceeded")
}

// A missing timestamp switches the entire workflow group to IDs, avoiding an
// inconsistent pairwise comparison when a paginated response omits a date.
type qualityWorkflowSelection struct {
	byCreation, byID *qualityWorkflowRun
	missingCreation  bool
}

func (s *qualityWorkflowSelection) include(run *qualityWorkflowRun) {
	if s.byID == nil || newerWorkflowID(run, s.byID) {
		s.byID = run
	}
	if run.Created.IsZero() {
		s.missingCreation = true
	}
	if s.byCreation == nil || run.Created.After(s.byCreation.Created) ||
		(run.Created.Equal(s.byCreation.Created) && newerWorkflowID(run, s.byCreation)) {
		s.byCreation = run
	}
}

func (s *qualityWorkflowSelection) current() *qualityWorkflowRun {
	if s == nil || (s.byID == nil && s.byCreation == nil) {
		return nil
	}
	if s.missingCreation {
		return s.byID
	}
	return s.byCreation
}

func newerWorkflowID(a, b *qualityWorkflowRun) bool {
	return a.ID > b.ID || (a.ID == b.ID && a.Attempt > b.Attempt)
}
