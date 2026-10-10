package qualitygate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/magefiles/bench"
	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

// Keep URL construction for readiness labels in one place; delete only the
// opposite managed label, preserving every unrelated label on the PR.
func (q *qualityClient) labels(number int, ready bool) error {
	name, opposite, color := qualityNotReady, qualityReady, "D93F0B"
	if ready {
		name, opposite, color = qualityReady, qualityNotReady, "0E8A16"
	}
	var label any
	if err := q.get("labels/"+url.PathEscape(name), &label); err != nil {
		if !githubapi.NotFound(err) {
			return err
		}
		if err = q.write(http.MethodPost, "labels", map[string]string{"name": name, "color": color, "description": "Atmos quality gates for the current PR head"}); err != nil {
			return err
		}
	}
	// Remove a stale positive label before attempting any other label update.
	err := q.write(http.MethodDelete, fmt.Sprintf("issues/%d/labels/%s", number, url.PathEscape(opposite)), nil)
	if err != nil && !githubapi.NotFound(err) {
		return err
	}
	return q.write(http.MethodPost, fmt.Sprintf("issues/%d/labels", number), map[string]any{"labels": []string{name}})
}

func (q *qualityClient) write(method, path string, value any) error {
	var body io.Reader
	if value != nil {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	return q.api.DoWithContext(q.ctx, method, "repos/"+q.repo+"/"+path, body, nil)
}

func qualityHeadMarker(sha string) string { return "<!-- atmos-quality-head:" + sha + " -->" }

func (q *qualityClient) refresh(number int, performance string, run *githubapi.Run) error {
	var pr githubapi.PR
	if err := q.get(fmt.Sprintf("pulls/%d", number), &pr); err != nil {
		return err
	}
	if !qualityEligible(&pr, q.repo) {
		return nil
	}
	if run != nil && !bench.PRMatches(&pr, run, q.repo) {
		return nil
	}
	existing, err := q.existing(number)
	if err != nil {
		return err
	}
	if qualityObsolete(existing, run) {
		return nil
	}
	body, ready := q.render(&pr, existing, performance, run)
	valid, err := q.stillCurrent(&pr, run)
	if err != nil || !valid {
		return err
	}
	return q.publish(number, existing, body, ready)
}

func qualityObsolete(existing *githubapi.Comment, run *githubapi.Run) bool {
	return run != nil && existing != nil && qualityNewer(existing.Body, run)
}

func qualityEligible(pr *githubapi.PR, repo string) bool {
	return pr.State == "open" && pr.Base.Ref == "main" && pr.Base.Repo.FullName == repo
}

func (q *qualityClient) render(pr *githubapi.PR, existing *githubapi.Comment, performance string, run *githubapi.Run) (string, bool) {
	if performance == "" {
		performance = qualityPreservePerformance(existing, pr.Head.SHA)
	}
	gates := q.evaluate(pr)
	body, ready := qualityMarkdown(pr, gates, performance)
	if run != nil {
		body += fmt.Sprintf("\n<!-- atmos-benchmark-run:%d:%d -->\n", run.ID, run.Attempt)
	} else if existing != nil && strings.Contains(existing.Body, qualityHeadMarker(pr.Head.SHA)) {
		body += "\n" + githubapi.CommentRunPattern.FindString(existing.Body) + "\n"
	}
	return body, ready
}

func (q *qualityClient) publish(number int, existing *githubapi.Comment, body string, ready bool) error {
	if err := qualitySummary(body); err != nil {
		return err
	}
	if err := q.labels(number, ready); err != nil {
		return err
	}
	if existing != nil && existing.Body == body {
		return nil
	}
	method, path := http.MethodPost, fmt.Sprintf("issues/%d/comments", number)
	if existing != nil {
		method, path = http.MethodPatch, fmt.Sprintf("issues/comments/%d", existing.ID)
	}
	return q.write(method, path, map[string]string{"body": body})
}

func (q *qualityClient) stillCurrent(pr *githubapi.PR, run *githubapi.Run) (bool, error) {
	var current githubapi.PR
	if err := q.get(fmt.Sprintf("pulls/%d", pr.Number), &current); err != nil {
		return false, err
	}
	if current.Head.SHA != pr.Head.SHA || current.State != "open" || current.Draft != pr.Draft || current.MergeSHA != pr.MergeSHA {
		return false, nil
	}
	if run != nil {
		var latest githubapi.Run
		if err := q.get(fmt.Sprintf("actions/runs/%d", run.ID), &latest); err != nil {
			return false, err
		}
		if latest.Attempt != run.Attempt || latest.Status != "completed" {
			return false, nil
		}
	}
	return true, nil
}

func (q *qualityClient) existing(number int) (*githubapi.Comment, error) {
	for page := 1; page <= githubapi.PageLimit; page++ {
		var comments []githubapi.Comment
		if err := q.get(fmt.Sprintf("issues/%d/comments?per_page=100&page=%d", number, page), &comments); err != nil {
			return nil, err
		}
		for _, comment := range comments {
			if comment.User.Login == "github-actions[bot]" && comment.User.Type == "Bot" && strings.HasPrefix(comment.Body, qualityMarker) {
				return &comment, nil
			}
		}
		if len(comments) < githubapi.PageSize {
			return nil, nil
		}
	}
	return nil, bench.Error("comment pagination limit exceeded")
}

func qualityNewer(body string, run *githubapi.Run) bool {
	matches := githubapi.CommentRunPattern.FindStringSubmatch(body)
	if len(matches) != 3 {
		return false
	}
	id, _ := strconv.ParseInt(matches[1], bench.Decimal, bench.FloatBits)
	attempt, _ := strconv.Atoi(matches[2])
	return id > run.ID || (id == run.ID && attempt > run.Attempt)
}

func qualityPreservePerformance(existing *githubapi.Comment, sha string) string {
	if existing == nil || !strings.Contains(existing.Body, qualityHeadMarker(sha)) {
		return "Performance measurements pending for this commit."
	}
	_, tail, ok := strings.Cut(existing.Body, qualityPerformanceStart)
	if !ok {
		return "Performance measurements pending for this commit."
	}
	section, _, ok := strings.Cut(tail, qualityPerformanceEnd)
	if !ok {
		return "Performance report unavailable."
	}
	return strings.TrimSpace(section)
}

func qualityMarkdown(pr *githubapi.PR, gates []qualityGate, performance string) (string, bool) {
	var out strings.Builder
	ready := !pr.Draft
	for _, gate := range gates {
		ready = ready && (gate.Passed || gate.Advisory)
	}
	status, badge, label := "not ready", "Not_ready-orange", qualityNotReady
	if ready {
		status, badge, label = "passed", "Passed-brightgreen", qualityReady
	}
	fmt.Fprintf(&out, "%s\n## Quality Gate %s\n\n![Quality Gate %s](https://img.shields.io/badge/Quality_Gate-%s)\n\n", qualityMarker, status, status, badge)
	for _, gate := range gates {
		icon := "⏳"
		if gate.Advisory && !gate.Passed {
			icon = "⚠️"
		}
		if gate.Passed {
			icon = "✅"
		}
		fmt.Fprintf(&out, "%s [%s](%s)\n\n", icon, gate.Name, gate.URL)
	}
	fmt.Fprintf(&out, "Current commit: `%s` · `%s`\n%s\n\n", pr.Head.SHA, label, qualityHeadMarker(pr.Head.SHA))
	if pr.Draft {
		fmt.Fprint(&out, "Draft PRs are not ready for merge.\n\n")
	}
	qualityDetails(&out, gates)
	out.WriteString(qualityGuidance(pr, gates, performance))
	fmt.Fprint(&out, "### Additional information\n\nPerformance comparisons are advisory and do not affect Quality Gate readiness.\n\n")
	performance = strings.TrimSpace(strings.TrimPrefix(performance, bench.Marker))
	budget := bench.CommentLimit - out.Len()
	performance = bench.TruncateReport(performance, budget)
	fmt.Fprintf(&out, "%s\n%s\n%s\n", qualityPerformanceStart, performance, qualityPerformanceEnd)
	return out.String(), ready
}

func qualityDetails(out *strings.Builder, gates []qualityGate) {
	fmt.Fprint(out, "<details><summary>Quality Gate details</summary>\n\n| Gate | Status | Evidence |\n|---|---|---|\n")
	for _, gate := range gates {
		state := "Not ready"
		if gate.Advisory {
			state = "Advisory warning"
		}
		if gate.Passed {
			state = "Passed"
		}
		fmt.Fprintf(out, "| %s | %s | %s |\n", gate.Name, state, bench.Escape(qualityShortDetail(gate.Detail)))
	}
	fmt.Fprint(out, "\n</details>\n\n")
}

func qualityShortDetail(detail string) string {
	const limit = 1500
	if len(detail) > limit {
		return detail[:limit] + " … (see CI checks for full details)"
	}
	return detail
}

func qualitySummary(body string) error {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, bench.PrivateFile) // #nosec G703 -- GitHub supplies the summary file path.
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, body)
	return err
}
