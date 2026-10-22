package qualitygate

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cloudposse/atmos/magefiles/bench"
	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

// qualityScheduleInterval is the cron period of the scheduled reconcile. The
// unpublished candidates rotate once per interval so a PR that hangs or fails
// every run cannot keep the PRs behind it from ever being visited.
const qualityScheduleInterval = 15 * time.Minute

// qualitySearchLimit bounds the PRs verified after a SHA search. A commit is
// the head of at most a handful of open PRs, so a small page is enough.
const qualitySearchLimit = 10

// Why a scheduled run reconciles a PR. Unpublished PRs come first so recovery
// of missed events is never starved by the busier recent-commit set.
const (
	qualityReasonNone        = ""
	qualityReasonUnpublished = "no report for current head"
	qualityReasonBaseChanged = "no report for current base"
	qualityReasonUnavailable = "report has unavailable evidence"
	qualityReasonPending     = "report awaits CI completion"
	qualityReasonRecent      = "recent head commit"
	qualityReasonUnknown     = "selection check failed"
)

// prNumbers lists eligible open PRs from a paginated pulls endpoint. When headSHA is
// set, only PRs whose head is exactly that commit qualify, so a PR whose head
// moved after the event is never reconciled for the stale commit.
func (q *qualityClient) prNumbers(path, headSHA string) ([]int, error) {
	prs, err := q.openPRs(path, headSHA)
	if err != nil {
		return nil, err
	}
	return prNumbersOf(prs), nil
}

func prNumbersOf(prs []githubapi.PR) []int {
	var numbers []int
	for i := range prs {
		numbers = append(numbers, prs[i].Number)
	}
	return numbers
}

// unlistedPRNumbers resolves PRs that the commit association endpoint does not
// list, which includes every PR whose head commit exists only in a fork. The
// head-branch lookup is exact and strongly consistent; the SHA search covers
// events that do not carry the fork owner and branch. Both verify the head SHA.
func (q *qualityClient) unlistedPRNumbers(sha, owner, branch string) ([]int, error) {
	if owner != "" && branch != "" {
		path := "pulls?state=open&head=" + url.QueryEscape(owner+":"+branch)
		numbers, err := q.prNumbers(path, sha)
		if err != nil || len(numbers) > 0 {
			return numbers, err
		}
	}
	return q.searchPRNumbers(sha)
}

// searchPRNumbers verifies SHA search results against each eligible PR's current head.
func (q *qualityClient) searchPRNumbers(sha string) ([]int, error) {
	query := url.Values{
		"q":        {fmt.Sprintf("repo:%s type:pr state:open sha:%s", q.repo, sha)},
		"per_page": {fmt.Sprint(qualitySearchLimit)},
	}
	var result struct {
		Items []struct {
			Number int `json:"number"`
		} `json:"items"`
	}
	if err := githubapi.Get(q.ctx, q.api, "search/issues?"+query.Encode(), &result); err != nil {
		return nil, err
	}
	var numbers []int
	for _, item := range result.Items {
		// Search matches any commit in the PR and lags behind pushes, so the
		// current PR is the only authority on its head.
		var pr githubapi.PR
		if err := q.get(fmt.Sprintf("pulls/%d", item.Number), &pr); err != nil {
			return nil, err
		}
		if qualityEligible(&pr, q.repo) && pr.Head.SHA == sha {
			numbers = append(numbers, item.Number)
		}
	}
	return numbers, nil
}

// recentPRNumbers selects open PRs that need a scheduled reconcile: those
// with no final gate report for their current head and base or a report with
// unavailable evidence (any age) and those whose head commit is newer than the cutoff.
//
// GitHub's updated_at is not an activity signal: the gate's own labels and
// comments, Mergify, CodeRabbit and Codecov all advance it, which previously
// selected nearly every open PR. It is used only as an upper bound: a push
// always advances updated_at, so a PR last updated before the cutoff cannot
// have a newer head commit, and its commit date is never fetched.
//
// A PR that is stale, labeled, and has a report for its current head and base is
// skipped even if review threads resolved since; those need the head-change or
// review events to reach the gate, since the schedule no longer sees them.
func (q *qualityClient) recentPRNumbers(cutoff time.Time) ([]int, error) {
	prs, err := q.openPRs("pulls?state=open&sort=updated&direction=desc", "")
	if err != nil {
		return nil, err
	}
	var unpublished, recent []int
	for i := range prs {
		pr := &prs[i]
		reason, checkErr := q.scheduledReason(pr, cutoff)
		if checkErr != nil {
			// Reconcile reports the same API failure if it persists; skipping the
			// PR would hide it.
			fmt.Printf("PR #%d: %s, including: %v\n", pr.Number, qualityReasonUnknown, checkErr)
		}
		switch reason {
		case qualityReasonNone:
		case qualityReasonRecent:
			recent = append(recent, pr.Number)
		default:
			unpublished = append(unpublished, pr.Number)
		}
	}
	fmt.Printf("Scheduled quality recovery: %d open PRs, %d needing a report, %d with a recent head commit\n", len(prs), len(unpublished), len(recent))
	unpublished = rotatePRNumbers(unpublished, int(cutoff.Unix()/int64(qualityScheduleInterval/time.Second)))
	return append(unpublished, recent...), nil
}

// rotatePRNumbers starts the list at slot modulo its length, keeping the order
// otherwise. The run has no cursor: successive schedule slots start at different
// PRs, so a persistent timeout at the head cannot starve the rest indefinitely.
func rotatePRNumbers(numbers []int, slot int) []int {
	if len(numbers) == 0 {
		return numbers
	}
	offset := slot % len(numbers)
	if offset < 0 {
		offset += len(numbers)
	}
	return append(append(make([]int, 0, len(numbers)), numbers[offset:]...), numbers[:offset]...)
}

// openPRs accepts an API path with a query string, retaining repository and
// optional current-head checks for every page regardless of the target branch.
func (q *qualityClient) openPRs(path, headSHA string) ([]githubapi.PR, error) {
	var all []githubapi.PR
	for page := 1; page <= githubapi.PageLimit; page++ {
		var prs []githubapi.PR
		pagePath := fmt.Sprintf("%s&per_page=%d&page=%d", path, githubapi.PageSize, page)
		if err := q.get(pagePath, &prs); err != nil {
			return nil, err
		}
		for i := range prs {
			if qualityEligible(&prs[i], q.repo) && (headSHA == "" || prs[i].Head.SHA == headSHA) {
				all = append(all, prs[i])
			}
		}
		if len(prs) < githubapi.PageSize {
			return all, nil
		}
	}
	return nil, bench.Error("PR pagination limit exceeded")
}

// scheduledReason answers why a PR is reconciled, spending at most one comment
// read and, for PRs updated since the cutoff, one commit read.
func (q *qualityClient) scheduledReason(pr *githubapi.PR, cutoff time.Time) (string, error) {
	if !qualityHasGateLabel(pr) {
		return qualityReasonUnpublished, nil
	}
	existing, err := q.existing(pr.Number)
	if err != nil {
		return qualityReasonUnknown, err
	}
	if existing == nil || !strings.Contains(existing.Body, qualityHeadMarker(pr.Head.SHA)) {
		return qualityReasonUnpublished, nil
	}
	// A prerequisite can advance or the PR can be retargeted without a new head.
	// Legacy reports without a base marker also need one fresh evaluation.
	if !strings.Contains(existing.Body, qualityBaseMarker(pr)) {
		return qualityReasonBaseChanged, nil
	}
	// A report that could not read some evidence is not final: retry it at any
	// age so it is replaced once the evidence recovers.
	if strings.Contains(existing.Body, qualityUnavailableProblem) {
		return qualityReasonUnavailable, nil
	}
	// A pending snapshot is not a final report, even after the recent-head window.
	if strings.Contains(existing.Body, qualityCIPending) {
		return qualityReasonPending, nil
	}
	if !pr.Updated.IsZero() && pr.Updated.Before(cutoff) {
		return qualityReasonNone, nil
	}
	committed, err := q.headCommitDate(pr.Head.SHA)
	if err != nil {
		return qualityReasonUnknown, err
	}
	if committed.Before(cutoff) {
		return qualityReasonNone, nil
	}
	return qualityReasonRecent, nil
}

func qualityHasGateLabel(pr *githubapi.PR) bool {
	for _, label := range pr.Labels {
		if label.Name == qualityReady || label.Name == qualityNotReady {
			return true
		}
	}
	return false
}

// headCommitDate returns when the commit was last created. A rebase or amend
// moves the committer date while keeping the author date, so the later one wins.
func (q *qualityClient) headCommitDate(sha string) (time.Time, error) {
	var commit struct {
		Commit struct {
			Author    struct{ Date time.Time } `json:"author"`
			Committer struct{ Date time.Time } `json:"committer"`
		} `json:"commit"`
	}
	if err := q.get("commits/"+sha, &commit); err != nil {
		return time.Time{}, err
	}
	date := commit.Commit.Committer.Date
	if commit.Commit.Author.Date.After(date) {
		date = commit.Commit.Author.Date
	}
	if date.IsZero() {
		return time.Time{}, bench.Error("commit %s has no author or committer date", sha)
	}
	return date, nil
}
