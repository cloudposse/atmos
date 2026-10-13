package qualitygate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/magefiles/bench"
	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

func (q *qualityClient) tests(pr *githubapi.PR) (qualityGate, error) {
	states := map[string]string{}
	if err := q.workflowStates(pr.Head.SHA, states); err != nil {
		return qualityGate{}, err
	}
	if err := q.allCheckStates(pr.Head.SHA, states); err != nil {
		return qualityGate{}, err
	}
	if err := q.allStatusStates(pr.Head.SHA, states); err != nil {
		return qualityGate{}, err
	}
	if err := q.mergeStates(pr, states); err != nil {
		return qualityGate{}, err
	}
	return qualityCIGate(states), nil
}

func qualityCIGate(states map[string]string) qualityGate {
	gate := qualityGate{Passed: len(states) > 0, Detail: fmt.Sprintf("%d current PR CI results passed or were skipped/neutral", len(states))}
	var pending []string
	for _, name := range sortedQualityKeys(states) {
		state := states[name]
		if state != "success" && state != "skipped" && state != "neutral" {
			gate.Passed = false
			pending = append(pending, name+": "+state)
		}
	}
	if len(states) == 0 {
		gate.Detail = "Pending: no CI results for this commit"
	} else if len(pending) > 0 {
		gate.Detail = strings.Join(pending, "; ")
	}
	qualityCIGuidance(&gate, states)
	return gate
}

func qualityCIGuidance(gate *qualityGate, states map[string]string) {
	gate.Signals = qualityBlockingSignals(states, true)
	if gate.Passed {
		return
	}
	if len(states) == 0 {
		gate.Problem = "No CI results have been reported for this commit."
		gate.NextAction = "Wait for the PR workflows to start. If none are scheduled, inspect the workflow triggers and Actions permissions."
		return
	}
	waiting := qualityPendingSignals(gate.Signals)
	failed := len(gate.Signals) - waiting
	gate.Problem = fmt.Sprintf("%d CI results are still pending; no failed results are currently reported.", waiting)
	gate.NextAction = "Wait for the listed checks and workflows to finish, then re-evaluate this head."
	if failed > 0 {
		gate.Problem = fmt.Sprintf("%d CI results are unsuccessful; %d are still pending.", failed, waiting)
		gate.NextAction = "Open the logs for the unsuccessful results listed below. Fix the reported cause and rerun those jobs; for cancelled or infrastructure-failed jobs, verify the cause before retrying."
	}
}

func qualityBlockingSignals(states map[string]string, allowNeutral bool) []qualitySignal {
	var failed, pending []qualitySignal
	for _, name := range sortedQualityKeys(states) {
		state := states[name]
		if state == "success" || (allowNeutral && (state == "skipped" || state == "neutral")) {
			continue
		}
		signal := qualitySignal{Name: name, State: state}
		if qualityPendingState(state) {
			pending = append(pending, signal)
		} else {
			failed = append(failed, signal)
		}
	}
	return append(failed, pending...)
}

func qualityPendingState(state string) bool {
	switch state {
	case "pending", "queued", "in_progress", "waiting", "requested", "pending_deployment":
		return true
	default:
		return false
	}
}

func qualityPendingSignals(signals []qualitySignal) int {
	count := 0
	for _, signal := range signals {
		if qualityPendingState(signal.State) {
			count++
		}
	}
	return count
}

// Some PR jobs attach their checks to GitHub's current synthetic merge commit.
// Include those signals for CI, while Codecov approval remains head-only.
func (q *qualityClient) mergeStates(pr *githubapi.PR, states map[string]string) error {
	if pr.MergeSHA == "" || pr.MergeSHA == pr.Head.SHA {
		return nil
	}
	merged := map[string]string{}
	if err := q.allCheckStates(pr.MergeSHA, merged); err != nil {
		return err
	}
	if err := q.allStatusStates(pr.MergeSHA, merged); err != nil {
		return err
	}
	for key, value := range merged {
		states["merge: "+key] = value
	}
	return nil
}

func (q *qualityClient) workflowStates(sha string, states map[string]string) error {
	for page := 1; page <= githubapi.PageLimit; page++ {
		var response struct {
			Runs []githubapi.Run `json:"workflow_runs"`
		}
		if err := q.get(fmt.Sprintf("actions/runs?head_sha=%s&per_page=100&page=%d", sha, page), &response); err != nil {
			return err
		}
		for i := range response.Runs {
			run := &response.Runs[i]
			if run.HeadSHA != sha || run.Event == "workflow_run" {
				continue
			}
			// API order is newest first; only the latest run/attempt of each workflow
			// represents this commit, including a queued or failed rerun.
			key := "workflow: " + run.Path
			if _, seen := states[key]; seen {
				continue
			}
			state := run.Status
			if state == "completed" {
				state = run.Conclusion
			}
			states[key] = state
		}
		if len(response.Runs) < githubapi.PageSize {
			return nil
		}
	}
	return bench.Error("workflow pagination limit exceeded")
}

func (q *qualityClient) allCheckStates(sha string, states map[string]string) error {
	for page := 1; page <= githubapi.PageLimit; page++ {
		var response struct {
			Checks []qualityCheck `json:"check_runs"`
		}
		if err := q.get(fmt.Sprintf("commits/%s/check-runs?filter=latest&per_page=100&page=%d", sha, page), &response); err != nil {
			return err
		}
		for _, check := range response.Checks {
			if check.HeadSHA != sha {
				continue
			}
			state := check.Status
			if state == "completed" {
				state = check.Conclusion
			}
			states["check: "+check.App.Slug+"/"+check.Name] = state
		}
		if len(response.Checks) < githubapi.PageSize {
			return nil
		}
	}
	return bench.Error("check pagination limit exceeded")
}

func (q *qualityClient) allStatusStates(sha string, states map[string]string) error {
	for page := 1; page <= githubapi.PageLimit; page++ {
		var statuses []struct {
			Context string `json:"context"`
			State   string `json:"state"`
		}
		if err := q.get(fmt.Sprintf("commits/%s/statuses?per_page=100&page=%d", sha, page), &statuses); err != nil {
			return err
		}
		for _, status := range statuses {
			key := "status: " + status.Context
			if _, seen := states[key]; !seen {
				states[key] = status.State
			}
		}
		if len(statuses) < githubapi.PageSize {
			return nil
		}
	}
	return bench.Error("status pagination limit exceeded")
}

func sortedQualityKeys(states map[string]string) []string {
	keys := make([]string, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type qualityCheck struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HeadSHA    string `json:"head_sha"`
	App        struct {
		Slug string `json:"slug"`
	} `json:"app"`
}

func (q *qualityClient) coverage(pr *githubapi.PR) (qualityGate, error) {
	states := map[string]string{}
	if err := q.coverageChecks(pr.Head.SHA, states); err != nil {
		return qualityGate{}, err
	}
	if err := q.coverageStatuses(pr.Head.SHA, states); err != nil {
		return qualityGate{}, err
	}
	return qualityCoverageGate(states), nil
}

func (q *qualityClient) coverageChecks(sha string, states map[string]string) error {
	for page := 1; page <= githubapi.PageLimit; page++ {
		var response struct {
			Checks []qualityCheck `json:"check_runs"`
		}
		if err := q.get(fmt.Sprintf("commits/%s/check-runs?filter=latest&per_page=100&page=%d", sha, page), &response); err != nil {
			return err
		}
		for _, check := range response.Checks {
			if check.App.Slug != "codecov" || check.HeadSHA != sha {
				continue
			}
			state := check.Status
			if state == "completed" {
				state = check.Conclusion
			}
			states[check.Name] = state
		}
		if len(response.Checks) < githubapi.PageSize {
			return nil
		}
	}
	return bench.Error("Codecov check pagination limit exceeded")
}

func qualityCoverageGate(states map[string]string) qualityGate {
	gate := qualityGate{Passed: len(states) > 0, Detail: "Pending: no Codecov result for this commit"}
	var details []string
	for _, name := range sortedQualityKeys(states) {
		state := states[name]
		details = append(details, name+": "+state)
		if state != "success" {
			gate.Passed = false
		}
	}
	if len(details) > 0 {
		gate.Detail = strings.Join(details, "; ")
	}
	qualityCoverageGuidance(&gate, states)
	return gate
}

func qualityCoverageGuidance(gate *qualityGate, states map[string]string) {
	gate.Signals = qualityBlockingSignals(states, false)
	if gate.Passed {
		return
	}
	switch {
	case len(states) == 0:
		gate.Problem = "Codecov has not reported a result for this commit; its coverage verdict is unknown."
		gate.NextAction = "Wait for this commit's coverage uploads, then recheck Codecov. If uploads have already finished, inspect the upload logs for the missing result."
	case qualityPendingSignals(gate.Signals) == len(gate.Signals):
		gate.Problem = "Codecov is still processing this commit's coverage; no failing coverage result is currently reported."
		gate.NextAction = "Wait for the pending Codecov results below to complete."
	default:
		gate.Problem = "Codecov has returned a non-passing result for this commit."
		gate.NextAction = "Open the non-passing Codecov results below. For failed coverage, identify the uncovered changed lines and add tests for those behaviors using the existing repository thresholds. For a skipped or neutral result, restore a completed coverage evaluation."
	}
}

func (q *qualityClient) coverageStatuses(sha string, states map[string]string) error {
	seen := map[string]bool{}
	for page := 1; page <= githubapi.PageLimit; page++ {
		var statuses []struct {
			Context string `json:"context"`
			State   string `json:"state"`
			SHA     string `json:"sha"`
			Creator struct {
				Login string `json:"login"`
			} `json:"creator"`
		}
		if err := q.get(fmt.Sprintf("commits/%s/statuses?per_page=100&page=%d", sha, page), &statuses); err != nil {
			return err
		}
		for _, status := range statuses {
			if !strings.HasPrefix(status.Context, "codecov/") || seen[status.Context] || (status.Creator.Login != "codecov[bot]" && status.Creator.Login != "codecov") {
				continue
			}
			seen[status.Context] = true
			// Keep check-run and legacy-status results independently: a pending
			// provider signal must not be hidden by a successful other signal.
			states[status.Context+" (status)"] = status.State
		}
		if len(statuses) < githubapi.PageSize {
			return nil
		}
	}
	return bench.Error("Codecov status pagination limit exceeded")
}

func (q *qualityClient) review(pr *githubapi.PR) (qualityGate, error) {
	// Automated PRs (Renovate, Dependabot, the github-actions token, release apps) do not require CodeRabbit approval.
	if pr.User.Type == "Bot" {
		return qualityGate{Passed: true, Detail: fmt.Sprintf("Not required: PR opened by automation (%s)", pr.User.Login)}, nil
	}
	gate := qualityGate{Detail: "Pending: CodeRabbit has not approved this commit"}
	for page := 1; page <= githubapi.PageLimit; page++ {
		var reviews []struct {
			State  string `json:"state"`
			Commit string `json:"commit_id"`
			User   struct {
				Login string `json:"login"`
			} `json:"user"`
		}
		if err := q.get(fmt.Sprintf("pulls/%d/reviews?per_page=100&page=%d", pr.Number, page), &reviews); err != nil {
			return gate, err
		}
		for _, review := range reviews {
			if review.User.Login != "coderabbitai[bot]" && review.User.Login != "coderabbitai" {
				continue
			}
			if review.Commit != pr.Head.SHA || review.State == "COMMENTED" || review.State == "PENDING" {
				continue
			}
			gate.Passed = review.State == "APPROVED"
			gate.Detail = review.State
		}
		if len(reviews) < githubapi.PageSize {
			qualityReviewGuidance(&gate)
			return gate, nil
		}
	}
	return gate, bench.Error("review pagination limit exceeded")
}

func qualityReviewGuidance(gate *qualityGate) {
	if gate.Passed {
		return
	}
	switch gate.Detail {
	case "CHANGES_REQUESTED":
		gate.Problem = "CodeRabbit requested changes on this commit."
		gate.NextAction = "Address the findings in that review or explain a justified disagreement, then request another review using the commands documented at https://docs.coderabbit.ai/guides/commands#manually-request-code-reviews."
	case "DISMISSED":
		gate.Problem = "CodeRabbit's review of this commit was dismissed."
		gate.NextAction = "Request a new CodeRabbit review on this head using the commands documented at https://docs.coderabbit.ai/guides/commands#manually-request-code-reviews."
	default:
		gate.Problem = "CodeRabbit has not approved this commit."
		gate.NextAction = "Wait for CodeRabbit's current-head review. If no review is running, request one using the commands documented at https://docs.coderabbit.ai/guides/commands#manually-request-code-reviews."
	}
}

func (q *qualityClient) graph(query string, variables map[string]any, response any) error {
	data, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err = q.api.DoWithContext(q.ctx, http.MethodPost, "https://api.github.com/graphql", bytes.NewReader(data), &envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		return bench.Error("GitHub GraphQL: %s", envelope.Errors[0].Message)
	}
	return json.Unmarshal(envelope.Data, response)
}

func (q *qualityClient) variables(pr *githubapi.PR, cursor *string) map[string]any {
	owner, name, _ := strings.Cut(q.repo, "/")
	return map[string]any{"owner": owner, "name": name, "number": pr.Number, "cursor": cursor}
}

type qualityPageInfo struct {
	HasNext bool   `json:"hasNextPage"`
	End     string `json:"endCursor"`
}

func (q *qualityClient) signatures(pr *githubapi.PR) (qualityGate, error) {
	const query = `query($owner:String!,$name:String!,$number:Int!,$cursor:String){repository(owner:$owner,name:$name){pullRequest(number:$number){headRefOid commits(first:100,after:$cursor){nodes{commit{oid signature{isValid}}}pageInfo{hasNextPage endCursor}}}}}`
	total, invalid := 0, 0
	var cursor *string
	for page := 0; page < githubapi.PageLimit; page++ {
		var response struct {
			Repository struct {
				PR struct {
					Head    string `json:"headRefOid"`
					Commits struct {
						Nodes []struct {
							Commit struct {
								Signature *struct {
									Valid bool `json:"isValid"`
								} `json:"signature"`
							} `json:"commit"`
						} `json:"nodes"`
						Page qualityPageInfo `json:"pageInfo"`
					} `json:"commits"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		if err := q.graph(query, q.variables(pr, cursor), &response); err != nil {
			return qualityGate{}, err
		}
		result := response.Repository.PR
		if result.Head != pr.Head.SHA {
			return qualityGate{}, bench.Error("PR head changed during signature inspection")
		}
		for _, node := range result.Commits.Nodes {
			total++
			if node.Commit.Signature == nil || !node.Commit.Signature.Valid {
				invalid++
			}
		}
		if !result.Commits.Page.HasNext {
			return qualityGate{Passed: total > 0 && invalid == 0, Detail: fmt.Sprintf("%d/%d commits have GitHub-verified signatures", total-invalid, total), Problem: fmt.Sprintf("GitHub verified %d of %d commit signatures; %d are missing or invalid.", total-invalid, total, invalid)}, nil
		}
		cursor = &result.Commits.Page.End
	}
	return qualityGate{}, bench.Error("commit pagination limit exceeded")
}

func (q *qualityClient) threads(pr *githubapi.PR) (qualityGate, error) {
	const query = `query($owner:String!,$name:String!,$number:Int!,$cursor:String){repository(owner:$owner,name:$name){pullRequest(number:$number){headRefOid reviewThreads(first:100,after:$cursor){nodes{isResolved comments(first:1){nodes{pullRequestReview{state}}}}pageInfo{hasNextPage endCursor}}}}}`
	unresolved := 0
	var cursor *string
	for page := 0; page < githubapi.PageLimit; page++ {
		var response struct {
			Repository struct {
				PR struct {
					Head    string `json:"headRefOid"`
					Threads struct {
						Nodes []struct {
							Resolved bool `json:"isResolved"`
							Comments struct {
								Nodes []struct {
									Review *struct {
										State string `json:"state"`
									} `json:"pullRequestReview"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
						Page qualityPageInfo `json:"pageInfo"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		if err := q.graph(query, q.variables(pr, cursor), &response); err != nil {
			return qualityGate{}, err
		}
		result := response.Repository.PR
		if result.Head != pr.Head.SHA {
			return qualityGate{}, bench.Error("PR head changed during thread inspection")
		}
		for _, thread := range result.Threads.Nodes {
			if thread.Resolved {
				continue
			}
			dismissed := len(thread.Comments.Nodes) > 0 && thread.Comments.Nodes[0].Review != nil && thread.Comments.Nodes[0].Review.State == "DISMISSED"
			if !dismissed {
				unresolved++
			}
		}
		if !result.Threads.Page.HasNext {
			return qualityGate{Passed: unresolved == 0, Detail: fmt.Sprintf("%d unresolved review threads (outdated threads still count)", unresolved), Problem: fmt.Sprintf("%d review threads remain unresolved, including outdated threads.", unresolved)}, nil
		}
		cursor = &result.Threads.Page.End
	}
	return qualityGate{}, bench.Error("thread pagination limit exceeded")
}
