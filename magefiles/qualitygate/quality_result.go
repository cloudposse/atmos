package qualitygate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/cloudposse/atmos/magefiles/bench"
	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

const (
	qualityResultPrefix  = "<!-- atmos-quality-result:"
	qualityReportHeader  = qualityMarker + "\n<!-- coderabbit do not respond to this -->\n"
	qualityResultPending = "pending"
	qualityResultReady   = "ready"
	qualityResultBlocked = "blocked"
)

// qualityResult is a versioned published snapshot, not an Actions run conclusion.
// No timestamp: an identical evaluation must not rewrite the sticky comment.
type qualityResult struct {
	SchemaVersion int    `json:"schema_version"`
	Repository    string `json:"repository"`
	PRNumber      int    `json:"pr_number"`
	HeadSHA       string `json:"head_sha"`
	BaseSHA       string `json:"base_sha"`
	BaseRef       string `json:"base_ref"`
	Revision      string `json:"revision"`
	State         string `json:"state"`
	Reason        string `json:"reason,omitempty"`
}

func qualitySnapshot(pr *githubapi.PR, gates []qualityGate, pending bool) qualityResult {
	result := qualityResult{
		SchemaVersion: 1, Repository: pr.Base.Repo.FullName, PRNumber: pr.Number,
		HeadSHA: pr.Head.SHA, BaseSHA: pr.Base.SHA, BaseRef: pr.Base.Ref,
		Revision: qualityRevision(pr), State: qualityResultReady,
	}
	for i := range gates {
		gate := &gates[i]
		if !gate.Advisory && !gate.Passed {
			result.State, result.Reason = qualityResultBlocked, "gates_failed"
		}
	}
	for i := range gates {
		gate := &gates[i]
		if !gate.Advisory && gate.Unavailable {
			result.State, result.Reason = qualityResultPending, "evidence_unavailable"
		}
	}
	if qualityAwaitingSignals(gates) {
		result.State, result.Reason = qualityResultPending, "checks_pending"
	}
	if pending || len(gates) == 0 {
		result.State, result.Reason = qualityResultPending, "ci_incomplete"
	}
	return result
}

func qualityAwaitingSignals(gates []qualityGate) bool {
	for i := range gates {
		if !gates[i].Advisory && qualityPendingSignals(gates[i].Signals) > 0 {
			return true
		}
	}
	return false
}

// Include mutable PR policy metadata as well as the head/base. Automation's
// own readiness labels and updated_at are deliberately excluded.
func qualityRevision(pr *githubapi.PR) string {
	data, _ := json.Marshal(struct {
		Head, Base, Ref, Merge string
		Draft                  bool
		Milestone              *githubapi.PRMilestone
		Labels                 []string
	}{pr.Head.SHA, pr.Base.SHA, pr.Base.Ref, pr.MergeSHA, pr.Draft, pr.Milestone, releaseLabels(pr)})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func qualityResultMarker(result *qualityResult) string {
	data, _ := json.Marshal(result)
	return qualityResultPrefix + string(data) + " -->"
}

func qualityReadResult(body string, pr *githubapi.PR) (qualityResult, bool) {
	tail, ok := strings.CutPrefix(body, qualityReportHeader+qualityResultPrefix)
	encoded, _, closed := strings.Cut(tail, " -->\n")
	var result qualityResult
	if !ok || !closed || json.Unmarshal([]byte(encoded), &result) != nil {
		return result, false
	}
	expected := qualitySnapshot(pr, nil, true)
	expected.State, expected.Reason = result.State, result.Reason
	validState := result.State == qualityResultPending || result.State == qualityResultReady || result.State == qualityResultBlocked
	return result, validState && result == expected
}

// Status prints one JSON object for the trusted published snapshot of a PR.
// It never refreshes or writes GitHub state. Missing/stale reports are pending;
// API errors fail the command rather than emitting a potentially green result.
func (Quality) Status(repo, number string) error {
	pr, err := strconv.Atoi(number)
	if err != nil || pr <= 0 || strconv.Itoa(pr) != number {
		return bench.Error("invalid PR number: use a positive decimal integer")
	}
	if !githubapi.RepoPattern.MatchString(repo) {
		return bench.Error("invalid repository")
	}
	client, err := api.DefaultRESTClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), qualityPRTimeout)
	defer cancel()
	q := qualityClient{ctx: ctx, api: client, repo: repo}
	result, err := q.result(pr)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func (q *qualityClient) result(number int) (qualityResult, error) {
	var pr githubapi.PR
	if err := q.get(fmt.Sprintf("pulls/%d", number), &pr); err != nil {
		return qualityResult{}, err
	}
	result := qualitySnapshot(&pr, nil, true)
	result.Reason = "report_missing_or_stale"
	if !qualityEligible(&pr, q.repo) {
		result.Reason = "pr_not_open_or_eligible"
		return result, nil
	}
	comment, err := q.existing(number)
	if err != nil {
		return qualityResult{}, err
	}
	if comment != nil {
		if published, valid := qualityReadResult(comment.Body, &pr); valid {
			result = published
		}
	}
	if err = q.validateResultCI(&pr, &result); err != nil {
		return qualityResult{}, err
	}
	var current githubapi.PR
	if err = q.get(fmt.Sprintf("pulls/%d", number), &current); err != nil {
		return qualityResult{}, err
	}
	if !qualitySameRevision(&pr, &current, q.repo) {
		result = qualitySnapshot(&current, nil, true)
		result.Reason = "pr_changed_during_read"
	}
	return result, nil
}

// Validate live CI before exposing a stored green snapshot. Reporter runs use
// the same exclusions and supersession rules as normal gate evaluation.
func (q *qualityClient) validateResultCI(pr *githubapi.PR, result *qualityResult) error {
	complete, err := q.reportingComplete(pr)
	if err != nil {
		return err
	}
	if !complete {
		result.State, result.Reason = qualityResultPending, "ci_incomplete"
	}
	if result.State != qualityResultReady {
		return nil
	}
	ci, err := q.tests(pr)
	if err != nil {
		return err
	}
	if ci.Passed {
		return nil
	}
	result.State, result.Reason = qualityResultBlocked, "ci_changed"
	if qualityPendingSignals(ci.Signals) > 0 {
		result.State = qualityResultPending
	}
	return nil
}
