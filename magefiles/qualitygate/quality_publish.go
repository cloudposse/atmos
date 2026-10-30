package qualitygate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
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
	var issue struct {
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := q.get(fmt.Sprintf("issues/%d", number), &issue); err != nil {
		return err
	}
	hasDesired, hasOpposite := false, false
	for _, label := range issue.Labels {
		hasDesired = hasDesired || label.Name == name
		hasOpposite = hasOpposite || label.Name == opposite
	}
	// Remove a stale positive label before attempting any other label update.
	if hasOpposite {
		err := q.write(http.MethodDelete, fmt.Sprintf("issues/%d/labels/%s", number, url.PathEscape(opposite)), nil)
		if err != nil && !githubapi.NotFound(err) {
			return err
		}
	}
	if hasDesired {
		return nil
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

// qualityBaseMarker records the evaluated target revision with a URL-encoded branch name.
func qualityBaseMarker(pr *githubapi.PR) string {
	return "<!-- atmos-quality-base:" + url.QueryEscape(pr.Base.Ref) + ":" + pr.Base.SHA + " -->"
}

// refresh evaluates and publishes current PR evidence while rejecting stale or superseded snapshots.
func (q *qualityClient) refresh(number int, performance string, run *githubapi.Run) error {
	// Scheduled and event reporters for one PR can run concurrently. Publish only
	// when the comment is unchanged since rendering; otherwise re-render from the
	// other writer's result so a stale snapshot never overwrites a fresher one.
	var stalePublished *githubapi.Comment
	for range qualityPublishAttempts {
		var pr githubapi.PR
		if err := q.get(fmt.Sprintf("pulls/%d", number), &pr); err != nil {
			return err
		}
		if !qualityEligible(&pr, q.repo) || (run != nil && !bench.PRMatches(&pr, run, q.repo)) {
			return nil
		}
		existing, err := q.existing(number)
		if err != nil {
			return err
		}
		if qualityObsolete(existing, run) {
			return nil
		}
		report := q.render(&pr, existing, performance, run)
		valid, err := q.stillCurrent(&pr, run)
		if err != nil {
			return err
		}
		if !q.ciIncomplete && report.releasePolicyPassed && len(report.unavailable) > 0 && qualityPublishedRevision(existing, &pr) && !qualitySameComment(existing, stalePublished) {
			if !valid {
				// Re-evaluate the new revision instead of retaining stale readiness,
				// including draft or merge changes that keep the same head and base.
				stalePublished = existing
				continue
			}
			// Missing evidence is not a verdict. Replacing a report for this head and base
			// would flip its label and comment on every transient API failure; keep the
			// last published state and let the failure surface for a retry.
			// Known incomplete CI is an exception: clear stale readiness even when
			// another gate's evidence is temporarily unavailable.
			return bench.Error("evidence unavailable for %s; kept the published report for %s", strings.Join(report.unavailable, ", "), pr.Head.SHA)
		}
		if !valid {
			return nil
		}
		latest, err := q.existing(number)
		if err != nil {
			return err
		}
		if !qualitySameComment(existing, latest) {
			continue
		}
		published, err := q.publish(number, existing, report.body, report.ready)
		if err != nil || published {
			return err
		}
	}
	// Another writer keeps publishing newer state; its report supersedes this one.
	return nil
}

// Bounds re-rendering when another reporter publishes during evaluation.
const qualityPublishAttempts = 3

func qualitySameComment(before, after *githubapi.Comment) bool {
	if before == nil || after == nil {
		return before == after
	}
	return before.ID == after.ID && before.Body == after.Body
}

func qualityObsolete(existing *githubapi.Comment, run *githubapi.Run) bool {
	return run != nil && existing != nil && qualityNewer(existing.Body, run)
}

// qualityEligible accepts open PRs targeting any named branch in the trusted repository.
func qualityEligible(pr *githubapi.PR, repo string) bool {
	return pr.State == "open" && pr.Base.Ref != "" && pr.Base.Repo.FullName == repo
}

// qualitySameRevision checks that the evaluated PR metadata still describes the current PR.
// A base update or retarget invalidates the evaluated mergeability and checks
// even when the PR head has not moved.
func qualitySameRevision(pr, current *githubapi.PR, repo string) bool {
	return qualityEligible(current, repo) && current.Head.SHA == pr.Head.SHA &&
		current.Draft == pr.Draft && current.MergeSHA == pr.MergeSHA &&
		current.Base.Ref == pr.Base.Ref && current.Base.SHA == pr.Base.SHA &&
		sameReleaseAssignment(pr, current)
}

// qualityPublishedRevision requires a report for both the current head and target revision.
func qualityPublishedRevision(existing *githubapi.Comment, pr *githubapi.PR) bool {
	return qualityPublishedFor(existing, pr.Head.SHA) && strings.Contains(existing.Body, qualityBaseMarker(pr))
}

func qualityPublishedFor(existing *githubapi.Comment, sha string) bool {
	return existing != nil && strings.Contains(existing.Body, qualityHeadMarker(sha))
}

type qualityRenderedReport struct {
	body        string
	ready       bool
	unavailable []string
	// Release eligibility depends on mutable repository metadata. A previous
	// green report cannot substitute for a passing live release-policy check.
	releasePolicyPassed bool
}

// render evaluates live evidence and records whether retaining an old report is safe.
func (q *qualityClient) render(pr *githubapi.PR, existing *githubapi.Comment, performance string, run *githubapi.Run) qualityRenderedReport {
	q.initReleaseEvidence()
	if q.ciIncomplete && qualityPublishedRevision(existing, pr) && strings.Contains(existing.Body, qualityCIPending) {
		// Keep the initial snapshot while jobs/workflows finish. refresh still
		// validates the revision and publish still repairs a stale ready label.
		return qualityRenderedReport{body: existing.Body}
	}
	if performance == "" {
		performance = qualityPreservePerformance(existing, pr.Head.SHA)
	}
	evaluator := *q
	evaluator.benchmarkReportFailed = strings.Contains(performance, qualityBenchmarkError)
	gates := evaluator.evaluate(pr)
	report := qualityRenderedReport{}
	for i := range gates {
		if gates[i].ID == "release-policy" {
			report.releasePolicyPassed = gates[i].Passed && !gates[i].Unavailable
		}
		if gates[i].Unavailable {
			report.unavailable = append(report.unavailable, gates[i].Name)
			fmt.Printf("PR #%d: %s: %s\n", pr.Number, gates[i].Name, gates[i].Detail)
		}
	}
	body, ready := qualityMarkdown(pr, gates, performance)
	if q.ciIncomplete {
		notice := qualityCIPending + "\n> CI is still running. This is an initial snapshot; the full report will refresh when all workflows finish.\n"
		body = strings.Replace(body, qualityMarker, qualityMarker+"\n"+notice, 1)
	}
	if run != nil {
		body += fmt.Sprintf("\n<!-- atmos-benchmark-run:%d:%d -->\n", run.ID, run.Attempt)
	} else if qualityPublishedFor(existing, pr.Head.SHA) {
		// Carry the run marker only when there is one; appending a bare newline pair
		// would make a never-benchmarked comment differ from itself and rewrite once.
		if marker := githubapi.CommentRunPattern.FindString(existing.Body); marker != "" {
			body += "\n" + marker + "\n"
		}
	}
	report.body, report.ready = body, ready
	return report
}

// Publish reports false without writing the comment when another writer changed
// it while labels were updated; the caller then re-renders from fresh state.
// GitHub has no conditional comment write, so the remaining race is one
// read-to-PATCH round trip, and the next reconcile repairs any lost update.
func (q *qualityClient) publish(number int, existing *githubapi.Comment, body string, ready bool) (bool, error) {
	if err := q.labels(number, ready); err != nil {
		return false, err
	}
	latest, err := q.existing(number)
	if err != nil || !qualitySameComment(existing, latest) {
		return false, err
	}
	if err = qualitySummary(body); err != nil {
		return false, err
	}
	if existing != nil && qualitySameReport(existing.Body, body) {
		return true, nil
	}
	method, path := http.MethodPost, fmt.Sprintf("issues/%d/comments", number)
	if existing != nil {
		method, path = http.MethodPatch, fmt.Sprintf("issues/comments/%d", existing.ID)
	}
	return true, q.write(method, path, map[string]string{"body": body})
}

var qualitySummaryLink = regexp.MustCompile(`(?m)^\[View job summary\]\(https://github\.com/[^/\s]+/[^/\s]+/actions/runs/[0-9]+#summary\)$`)

// A new reporter run alone is not new evidence. Keep the published summary link
// until the report changes, while still writing this invocation's job summary.
func qualitySameReport(before, after string) bool {
	return qualitySummaryLink.ReplaceAllString(before, "[View job summary]") ==
		qualitySummaryLink.ReplaceAllString(after, "[View job summary]")
}

// stillCurrent rechecks the PR revision and optional benchmark attempt before publication.
func (q *qualityClient) stillCurrent(pr *githubapi.PR, run *githubapi.Run) (bool, error) {
	var current githubapi.PR
	if err := q.get(fmt.Sprintf("pulls/%d", pr.Number), &current); err != nil {
		return false, err
	}
	if !qualitySameRevision(pr, &current, q.repo) {
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

// qualityMarkdown renders gate evidence, revision markers, and performance data with the readiness verdict.
func qualityMarkdown(pr *githubapi.PR, gates []qualityGate, performance string) (string, bool) {
	var out strings.Builder
	ready := true
	for i := range gates {
		gate := &gates[i]
		ready = ready && (gate.Passed || gate.Advisory)
	}
	status, badge, label := "not ready", "Not_ready-d93f0b", qualityNotReady
	if ready {
		status, badge, label = "passed", "Passed-0e8a16", qualityReady
	}
	fmt.Fprintf(&out, "%s\n<!-- coderabbit do not respond to this -->\n<h2>Quality Gate &nbsp; <a href=\"https://github.com/%s/pull/%d/checks\"><img height=\"28\" align=\"top\" alt=\"Quality Gate %s\" src=\"https://img.shields.io/badge/%s?style=for-the-badge\" /></a></h2>\n\n", qualityMarker, pr.Base.Repo.FullName, pr.Number, status, badge)
	qualityGateList(&out, gates)
	if summaryURL := qualitySummaryURL(pr.Base.Repo.FullName); summaryURL != "" {
		fmt.Fprintf(&out, "[View job summary](%s)\n\n", summaryURL)
	}
	fmt.Fprintf(&out, "<sub>Current commit: <code>%s</code> · <code>%s</code></sub>\n%s\n\n", pr.Head.SHA, label, qualityHeadMarker(pr.Head.SHA)+"\n"+qualityBaseMarker(pr))
	fmt.Fprintln(&out, qualityStatusBlockersMarker(pr, gates))
	fmt.Fprintln(&out, qualityReporterRunsExcluded)
	qualityDetails(&out, gates)
	out.WriteString(qualityGuidance(pr, gates, performance))
	if performance == "Performance measurements pending for this commit." {
		performance = ""
	}
	performance = strings.TrimSpace(strings.TrimPrefix(performance, bench.Marker))
	budget := bench.CommentLimit - out.Len()
	performance = bench.TruncateReport(performance, budget)
	fmt.Fprintf(&out, "%s\n%s\n%s\n", qualityPerformanceStart, performance, qualityPerformanceEnd)
	return out.String(), ready
}

func qualityGateList(out *strings.Builder, gates []qualityGate) {
	sections := []string{"Readiness", "Reviews", "Measures"}
	grouped := make(map[string][]qualityGate)
	for i := range gates {
		gate := &gates[i]
		section := gate.Section
		if section == "" {
			section = "Other checks"
		}
		if _, exists := grouped[section]; !exists && section != "Readiness" && section != "Reviews" && section != "Measures" {
			sections = append(sections, section)
		}
		grouped[section] = append(grouped[section], *gate)
	}
	for _, section := range sections {
		if len(grouped[section]) == 0 {
			continue
		}
		fmt.Fprintf(out, "### %s\n\n", section)
		qualityGateItems(out, grouped[section])
	}
}

func qualityGateItems(out *strings.Builder, gates []qualityGate) {
	for i := range gates {
		gate := &gates[i]
		icon := "⚠️"
		if gate.Passed {
			icon = "✅"
		}
		fmt.Fprintf(out, "%s [%s](%s)", icon, gate.Name, gate.URL)
		if !gate.Passed && gate.Problem != "" {
			fmt.Fprintf(out, " — %s", bench.Escape(gate.Problem))
		}
		fmt.Fprint(out, "\n\n")
	}
}

func qualityDetails(out *strings.Builder, gates []qualityGate) {
	fmt.Fprint(out, "<details><summary>Quality Gate details</summary>\n\n| Gate | Status | Evidence |\n|---|---|---|\n")
	for i := range gates {
		gate := &gates[i]
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

// Link only when this invocation writes an Actions job summary. Local reports
// must not advertise a summary that GitHub will never receive.
func qualitySummaryURL(repo string) string {
	if os.Getenv("GITHUB_STEP_SUMMARY") == "" {
		return ""
	}
	runID, err := strconv.ParseInt(os.Getenv("GITHUB_RUN_ID"), bench.Decimal, bench.FloatBits)
	if err != nil || runID <= 0 {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/actions/runs/%d#summary", repo, runID)
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
