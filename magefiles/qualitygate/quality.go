package qualitygate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/magefile/mage/mg"

	"github.com/cloudposse/atmos/magefiles/bench"
	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

// Quality maintains the composite PR quality report and readiness labels.
type Quality mg.Namespace

const (
	qualityMarker           = "<!-- atmos-quality -->"
	qualityReady            = "quality:ready"
	qualityNotReady         = "quality:not-ready"
	qualityPerformanceStart = "<!-- atmos-quality-performance:start -->"
	qualityPerformanceEnd   = "<!-- atmos-quality-performance:end -->"
)

type qualityGate struct {
	ID, Name, Detail, URL, Remediation string
	Advisory                           bool
	Passed                             bool
}

type qualityClient struct {
	ctx  context.Context
	api  githubapi.API
	repo string
}

func (q *qualityClient) get(path string, response any) error {
	return githubapi.Get(q.ctx, q.api, "repos/"+q.repo+"/"+path, response)
}

// Report refreshes the PRs associated with the current Actions event. Scheduled
// and manual runs reconcile every open PR, including fork review resolutions.
func (Quality) Report(repo string) error {
	if !githubapi.RepoPattern.MatchString(repo) {
		return bench.Error("invalid repository")
	}
	client, err := api.DefaultRESTClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	q := qualityClient{ctx: ctx, api: client, repo: repo}
	prs, err := q.eventPRs()
	if err != nil {
		return err
	}
	for _, number := range prs {
		if err = q.reconcile(number); err != nil {
			return err
		}
	}
	return nil
}

//nolint:revive // Explicit event precedence handles reviews, external checks, and workflow runs.
func (q *qualityClient) eventPRs() ([]int, error) {
	var event struct {
		Number int `json:"number"`
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
	if event.Issue.Number > 0 && len(event.Issue.PullRequest) > 0 {
		return []int{event.Issue.Number}, nil
	}
	if event.Number > 0 {
		return []int{event.Number}, nil
	}
	sha := event.SHA
	if event.CheckRun.HeadSHA != "" {
		sha = event.CheckRun.HeadSHA
	}
	if event.WorkflowRun.HeadSHA != "" {
		sha = event.WorkflowRun.HeadSHA
	}
	path := "pulls?state=open&base=main"
	if sha != "" {
		if !githubapi.SHAPattern.MatchString(sha) {
			return nil, bench.Error("invalid event SHA")
		}
		path = "commits/" + sha + "/pulls?state=open"
	}
	return q.prNumbers(path)
}

func (q *qualityClient) prNumbers(path string) ([]int, error) {
	var numbers []int
	for page := 1; page <= githubapi.PageLimit; page++ {
		var prs []githubapi.PR
		if err := q.get(fmt.Sprintf("%s&per_page=100&page=%d", path, page), &prs); err != nil {
			return nil, err
		}
		for i := range prs {
			pr := &prs[i]
			if pr.State == "open" && pr.Base.Ref == "main" {
				numbers = append(numbers, pr.Number)
			}
		}
		if len(prs) < githubapi.PageSize {
			return numbers, nil
		}
	}
	return nil, bench.Error("PR pagination limit exceeded")
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
		return qualityGate{Detail: "Changed-file count unavailable"}, nil
	}
	return qualityGate{Passed: *pr.ChangedFiles < qualityFileLimit, Detail: fmt.Sprintf("%d changed files; maximum 149", *pr.ChangedFiles)}, nil
}

// Recover benchmark reports displaced from Actions' single pending concurrency
// slot. The schedule makes publication eventual even during a busy CI burst.
func (q *qualityClient) reconcile(number int) error {
	var pr githubapi.PR
	if err := q.get(fmt.Sprintf("pulls/%d", number), &pr); err != nil {
		return err
	}
	if !qualityEligible(&pr, q.repo) {
		return nil
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
			return bench.ReportRun(q.ctx, q.api, q.repo, run.ID, q.refresh)
		}
	}
	return q.refresh(number, "", nil)
}

func qualityReported(existing *githubapi.Comment, pr *githubapi.PR, run *githubapi.Run) bool {
	if existing == nil || !strings.Contains(existing.Body, qualityHeadMarker(pr.Head.SHA)) {
		return false
	}
	marker := fmt.Sprintf("<!-- atmos-benchmark-run:%d:%d -->", run.ID, run.Attempt)
	return strings.Contains(existing.Body, marker) || qualityNewer(existing.Body, run)
}
