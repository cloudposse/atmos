// Package rerun classifies the non-success jobs of a GitHub Actions
// workflow-run attempt as infrastructure zombies or real failures, and turns
// that classification into a rerun verdict. It backs the
// `go tool mage ci:classifyInfraFailures` and `ci:rerunInfraFailures` targets
// used by .github/workflows/rerun-infra-failures.yml (via
// .github/actions/classify-infra-failures), which re-runs a `Tests` run only
// when nothing in it genuinely failed. See
// docs/fixes/2026-09-03-rerun-infra-cancelled-ci-runs.md.
//
// classify.go is deliberately HTTP-free: Classify/Verdict/WriteTSV/
// WriteMarkdownSummary are pure functions of a decoded job list, which keeps
// the classification rules unit-testable against recorded API payloads
// (testdata/run-*.json) with no network or mocking involved. The network
// calls this package's callers need - listing a run attempt's jobs, reading a
// pull request's head SHA, and requesting a rerun - live in fetch.go and
// actions.go behind the small RESTClient interface, so those too are testable
// without a live GitHub API.
package rerun

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Class is the verdict for one non-success job.
type Class string

const (
	// ClassRunnerStuckAfterComplete is a cancelled job whose every step
	// succeeded (or was skipped) but whose job-level completion landed at
	// least Options.StuckGap after its last step: the runner finished all its
	// work, never reported completion, and GitHub reaped it. On Windows this
	// is harden-runner's post step killing its agent mid DNS-restore.
	ClassRunnerStuckAfterComplete Class = "runner-stuck-after-complete"
	// ClassRunnerLost is a failed job with no failed step: the runner vanished.
	ClassRunnerLost Class = "runner-lost"
	// ClassCheckCascade is a failed job whose only failed steps are aggregator
	// steps named in checkResultStepNames (test.yml's test-required, k3s and
	// terraform-registry-cache verdict jobs). Those steps only inspect other
	// jobs' conclusions and never run tests, so they fail as a consequence of
	// a zombie upstream job. Coupled to test.yml's step-naming convention.
	ClassCheckCascade Class = "check-cascade"
	// ClassSuperseded is any other cancellation: a step was interrupted or the
	// job never started, because a newer run replaced it or a human cancelled.
	ClassSuperseded Class = "superseded"
	// ClassRealFailure is everything else: a step genuinely failed.
	ClassRealFailure Class = "real-failure"
)

// Outcome is the rerun verdict for a whole run attempt.
type Outcome string

const (
	// OutcomeRerun means every non-success job is a zombie or a cascade of one.
	OutcomeRerun Outcome = "rerun"
	// OutcomeNoRerun means at least one job vetoes a rerun, or nothing is a zombie.
	OutcomeNoRerun Outcome = "no-rerun"
	// OutcomeNoJobs means the attempt has no non-success jobs at all.
	OutcomeNoJobs Outcome = "no-jobs"
)

// DefaultStuckGap is the minimum delay between a cancelled job's last step
// and its job-level completion for it to count as runner-stuck-after-complete.
// Ordinary cancellations land within seconds; reaped zombies take 30 minutes
// or more.
const DefaultStuckGap = 5 * time.Minute

const (
	conclusionSuccess   = "success"
	conclusionSkipped   = "skipped"
	conclusionFailure   = "failure"
	conclusionCancelled = "cancelled"
	statusCompleted     = "completed"
)

var errInvalidJobsJSON = errors.New("invalid jobs JSON")

// checkResultStepNames is the exact, closed set of aggregator step names
// test.yml defines today. A closed set - rather than a broad "Check .*
// result" pattern this package used before - narrows, but cannot fully
// close, a real gap: a pull_request-triggered `Tests` run executes the PR's
// own copy of test.yml, so a same-repo PR could in principle name one of its
// own (always-failing) steps to match one of these exact names, gaming
// check-cascade into tolerating it. Accepted rather than solved outright:
// GitHub's jobs API carries no signal distinguishing "this step name came
// from main" from "this step name came from the PR branch" for a
// pull_request trigger, and the blast radius stays small regardless - at
// most a couple of extra reruns of that PR's own run
// (rerun-infra-failures.yml's run_attempt < 3 cap), never another PR's run,
// and a same-repo PR can already run arbitrary code in this CI either way.
var checkResultStepNames = map[string]bool{
	"Check per-OS test matrix result":       true,
	"Check terraform-registry-cache result": true,
	"Check k3s matrix result":               true,
}

// Step is one step of a job as returned by the GitHub jobs API.
type Step struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	// CompletedAt is nil when the API returns null. encoding/json parses
	// RFC 3339 timestamps with or without fractional seconds.
	CompletedAt *time.Time `json:"completed_at"`
}

// Job is one job of a workflow-run attempt as returned by the GitHub jobs API.
type Job struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	CompletedAt *time.Time `json:"completed_at"`
	Steps       []Step     `json:"steps"`
}

// Classified is the class assigned to one non-success job.
type Classified struct {
	Job        string
	Conclusion string
	Class      Class
}

// Options tunes classification.
type Options struct {
	// StuckGap overrides DefaultStuckGap when positive.
	StuckGap time.Duration
}

// DecodeJobs reads the jobs of a run attempt from r. It accepts the raw
// `{"jobs":[...]}` response, several such pages concatenated (the shape of
// `gh api ... --paginate` output) or a bare array of jobs.
func DecodeJobs(r io.Reader) ([]Job, error) {
	defer perf.Track(nil, "rerun.DecodeJobs")()

	var jobs []Job
	decoder := json.NewDecoder(r)
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				return jobs, nil
			}
			return nil, fmt.Errorf("%w: %w", errInvalidJobsJSON, err)
		}
		page, err := decodePage(raw)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, page...)
	}
}

func decodePage(raw json.RawMessage) ([]Job, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%w: empty document", errInvalidJobsJSON)
	}
	switch trimmed[0] {
	case '{':
		var page struct {
			Jobs []Job `json:"jobs"`
		}
		if err := json.Unmarshal(trimmed, &page); err != nil {
			return nil, fmt.Errorf("%w: %w", errInvalidJobsJSON, err)
		}
		return page.Jobs, nil
	case '[':
		var jobs []Job
		if err := json.Unmarshal(trimmed, &jobs); err != nil {
			return nil, fmt.Errorf("%w: %w", errInvalidJobsJSON, err)
		}
		return jobs, nil
	default:
		return nil, fmt.Errorf("%w: expected an object or array, got %q", errInvalidJobsJSON, string(trimmed[:1]))
	}
}

// Classify returns one Classified entry per job whose conclusion is not
// success, skipped or empty (still running), in input order.
func Classify(jobs []Job, opts Options) []Classified {
	defer perf.Track(nil, "rerun.Classify")()

	gap := opts.StuckGap
	if gap <= 0 {
		gap = DefaultStuckGap
	}
	var out []Classified
	for i := range jobs {
		job := &jobs[i]
		switch job.Conclusion {
		case "", conclusionSuccess, conclusionSkipped:
			continue
		}
		out = append(out, Classified{Job: job.Name, Conclusion: job.Conclusion, Class: classify(job, gap)})
	}
	return out
}

func classify(job *Job, gap time.Duration) Class {
	switch job.Conclusion {
	case conclusionCancelled:
		if allStepsOK(job.Steps) && gapAfterLastStep(job) >= gap {
			return ClassRunnerStuckAfterComplete
		}
		return ClassSuperseded
	case conclusionFailure:
		failed := failedSteps(job.Steps)
		if len(failed) == 0 {
			return ClassRunnerLost
		}
		if allCheckResultSteps(failed) {
			return ClassCheckCascade
		}
		return ClassRealFailure
	default:
		return ClassRealFailure
	}
}

func allStepsOK(steps []Step) bool {
	if len(steps) == 0 {
		return false
	}
	for _, step := range steps {
		if step.Status != statusCompleted {
			return false
		}
		if step.Conclusion != conclusionSuccess && step.Conclusion != conclusionSkipped {
			return false
		}
	}
	return true
}

// gapAfterLastStep is how long after its last step the job itself completed,
// or -1 when either timestamp is missing.
func gapAfterLastStep(job *Job) time.Duration {
	if len(job.Steps) == 0 || job.CompletedAt == nil {
		return -1
	}
	last := job.Steps[len(job.Steps)-1].CompletedAt
	if last == nil {
		return -1
	}
	return job.CompletedAt.Sub(*last)
}

func failedSteps(steps []Step) []Step {
	var failed []Step
	for _, step := range steps {
		if step.Conclusion == conclusionFailure {
			failed = append(failed, step)
		}
	}
	return failed
}

func allCheckResultSteps(steps []Step) bool {
	for _, step := range steps {
		if !checkResultStepNames[step.Name] {
			return false
		}
	}
	return true
}

// Verdict decides whether a run attempt should be rerun: only when at least
// one job is runner-stuck-after-complete or runner-lost and every other one is
// check-cascade. Any real-failure or superseded job vetoes the rerun. The
// reason is a short human-readable justification for the log.
func Verdict(classified []Classified) (Outcome, string) {
	defer perf.Track(nil, "rerun.Verdict")()

	if len(classified) == 0 {
		return OutcomeNoJobs, "no non-success jobs"
	}
	zombies, vetoes := 0, 0
	for _, entry := range classified {
		switch entry.Class {
		case ClassRunnerStuckAfterComplete, ClassRunnerLost:
			zombies++
		case ClassCheckCascade:
		default:
			vetoes++
		}
	}
	switch {
	case zombies == 0:
		return OutcomeNoRerun, "no runner-stuck-after-complete or runner-lost jobs"
	case vetoes != 0:
		return OutcomeNoRerun, fmt.Sprintf("%d job(s) are real-failure or superseded", vetoes)
	default:
		return OutcomeRerun, fmt.Sprintf("all %d non-success job(s) are infrastructure zombies or cascades of one", len(classified))
	}
}

// WriteTSV writes one `job<TAB>conclusion<TAB>class` line per entry.
func WriteTSV(w io.Writer, classified []Classified) error {
	defer perf.Track(nil, "rerun.WriteTSV")()

	var b strings.Builder
	for _, entry := range classified {
		fmt.Fprintf(&b, "%s\t%s\t%s\n", entry.Job, entry.Conclusion, entry.Class)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// RunRef identifies the run/attempt a WriteMarkdownSummary header links to.
type RunRef struct {
	Repo       string
	RunID      string
	RunAttempt string
}

// url is the GitHub URL for the run/attempt this ref points to.
func (r RunRef) url() string {
	return fmt.Sprintf("https://github.com/%s/actions/runs/%s/attempts/%s", r.Repo, r.RunID, r.RunAttempt)
}

// WriteMarkdownSummary renders a classification as a GitHub Actions
// step-summary block: a header linking to the run, a Markdown table of
// non-success jobs, and the verdict line.
func WriteMarkdownSummary(w io.Writer, run RunRef, classified []Classified, outcome Outcome, reason string) error {
	defer perf.Track(nil, "rerun.WriteMarkdownSummary")()

	var b strings.Builder
	fmt.Fprintf(&b, "### Infra-failure classification: run [%s attempt %s](%s)\n\n", run.RunID, run.RunAttempt, run.url())
	fmt.Fprintf(&b, "| Job | Conclusion | Class |\n|---|---|---|\n")
	for _, entry := range classified {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", entry.Job, entry.Conclusion, entry.Class)
	}
	fmt.Fprintf(&b, "\nVerdict: **%s** (%s)\n", outcome, reason)
	_, err := io.WriteString(w, b.String())
	return err
}
