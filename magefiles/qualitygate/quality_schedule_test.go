//go:build mage

package qualitygate

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
)

// scheduleCommentState describes the gate comment a PR has.
type scheduleCommentState int

const (
	scheduleCommentCurrent     scheduleCommentState = iota // Report for the PR's current head.
	scheduleCommentOldHead                                 // Report for a previous head.
	scheduleCommentUnavailable                             // Report for the current head with a gate whose evidence was unavailable.
	scheduleCommentPending                                 // Initial snapshot awaiting CI completion.
	scheduleCommentNone
	scheduleCommentError
)

// schedulePR is one open PR in the scheduled-selection fixtures.
type schedulePR struct {
	number    int
	labeled   bool
	updated   time.Time // GitHub's updated_at: advanced by bot comments and labels too.
	committed time.Time // The head commit date.
	comment   scheduleCommentState
	state     string
	base      string
	noUpdated bool
}

func scheduleSHA(number int) string { return fmt.Sprintf("%040x", number) }

// scheduleServer serves PR lists, gate comments and commit dates, and records
// which per-PR endpoints the selection touched.
type scheduleServer struct {
	prs      []schedulePR
	comments []int
	commits  []int
	pages    []int
}

var (
	schedulePagePattern    = regexp.MustCompile(`[&?]page=(\d+)`)
	scheduleCommentPattern = regexp.MustCompile(`issues/(\d+)/comments`)
	scheduleCommitPattern  = regexp.MustCompile(`commits/([0-9a-f]{40})$`)
)

func (s *scheduleServer) get(path string, response any) error {
	switch {
	case strings.Contains(path, "/actions/runs?"):
		return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{}}, response)
	case strings.Contains(path, "/pulls?"):
		page, _ := strconv.Atoi(schedulePagePattern.FindStringSubmatch(path)[1])
		s.pages = append(s.pages, page)
		var list []githubapi.PR
		start := (page - 1) * githubapi.PageSize
		for i := start; i < len(s.prs) && i < start+githubapi.PageSize; i++ {
			list = append(list, s.listed(&s.prs[i]))
		}
		return githubtest.Respond(list, response)
	case scheduleCommentPattern.MatchString(path):
		number, _ := strconv.Atoi(scheduleCommentPattern.FindStringSubmatch(path)[1])
		s.comments = append(s.comments, number)
		return s.commentsFor(number, response)
	case scheduleCommitPattern.MatchString(path):
		sha := scheduleCommitPattern.FindStringSubmatch(path)[1]
		number, _ := strconv.ParseInt(sha, 16, 64)
		s.commits = append(s.commits, int(number))
		for i := range s.prs {
			if s.prs[i].number == int(number) {
				date := s.prs[i].committed.Format(time.RFC3339Nano)
				return githubtest.Respond(map[string]any{"commit": map[string]any{
					"author":    map[string]string{"date": time.Time{}.Format(time.RFC3339)},
					"committer": map[string]string{"date": date},
				}}, response)
			}
		}
	}
	return fmt.Errorf("unexpected API path %s", path)
}

func (s *scheduleServer) listed(p *schedulePR) githubapi.PR {
	pr := githubtest.PR(githubtest.Run())
	pr.Number, pr.State, pr.Updated = p.number, "open", p.updated
	pr.Head.SHA = scheduleSHA(p.number)
	if p.noUpdated {
		pr.Updated = time.Time{}
	}
	if p.state != "" {
		pr.State = p.state
	}
	if p.base != "" {
		pr.Base.Ref = p.base
	}
	if p.labeled {
		pr.Labels = append(pr.Labels, struct {
			Name string `json:"name"`
		}{Name: qualityReady})
	}
	return pr
}

// commentsFor serves trusted report fixtures with revision markers or the requested missing-evidence state.
func (s *scheduleServer) commentsFor(number int, response any) error {
	for i := range s.prs {
		p := &s.prs[i]
		if p.number != number {
			continue
		}
		var body string
		switch p.comment {
		case scheduleCommentError:
			return errors.New("comments unavailable")
		case scheduleCommentCurrent:
			body = qualityMarker + "\n" + qualityHeadMarker(scheduleSHA(number)) + "\n"
		case scheduleCommentUnavailable:
			body = qualityMarker + "\n" + qualityHeadMarker(scheduleSHA(number)) + "\nAll CI checks and workflows " + qualityUnavailableProblem + "\n"
		case scheduleCommentPending:
			body = qualityMarker + "\n" + qualityHeadMarker(scheduleSHA(number)) + "\n" + qualityCIPending + "\n"
		case scheduleCommentOldHead:
			body = qualityMarker + "\n" + qualityHeadMarker(githubtest.Base) + "\n"
		case scheduleCommentNone:
			return githubtest.Respond([]githubapi.Comment{{ID: 1, Body: "unrelated"}}, response)
		}
		pr := s.listed(p)
		body += qualityBaseMarker(&pr) + "\n"
		comment := githubapi.Comment{ID: 17, Body: body}
		comment.User.Login, comment.User.Type = "github-actions[bot]", "Bot"
		return githubtest.Respond([]githubapi.Comment{comment}, response)
	}
	return fmt.Errorf("no fixture for PR #%d", number)
}

// TestQualityScheduledSelection checks recovery eligibility, ordering, and bounded metadata reads.
func TestQualityScheduledSelection(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "schedule")
	t.Setenv("GITHUB_EVENT_PATH", "")
	now := time.Date(2026, 10, 13, 1, 0, 0, 0, time.FixedZone("test", -5*60*60))
	cutoff := now.UTC().Add(-24 * time.Hour)
	old := cutoff.Add(-time.Hour)
	cases := []struct {
		name         string
		prs          []schedulePR
		want         []int
		wantComments []int
		wantCommits  []int
	}{
		{
			name: "bot activity alone does not select a PR",
			prs: []schedulePR{
				{number: 1, labeled: true, updated: now, committed: old, comment: scheduleCommentCurrent},
			},
			wantComments: []int{1},
			wantCommits:  []int{1},
		},
		{
			name: "recent head commit is selected, cutoff inclusive",
			prs: []schedulePR{
				{number: 1, labeled: true, updated: now, committed: now, comment: scheduleCommentCurrent},
				{number: 2, labeled: true, updated: now, committed: cutoff, comment: scheduleCommentCurrent},
				{number: 3, labeled: true, updated: now, committed: cutoff.Add(-time.Nanosecond), comment: scheduleCommentCurrent},
			},
			want:         []int{1, 2},
			wantComments: []int{1, 2, 3},
			wantCommits:  []int{1, 2, 3},
		},
		{
			name: "no report for current head is selected at any age and without a commit read",
			prs: []schedulePR{
				{number: 1, labeled: true, updated: old, committed: old, comment: scheduleCommentNone},
				{number: 2, labeled: true, updated: old, committed: old, comment: scheduleCommentOldHead},
			},
			want:         []int{1, 2},
			wantComments: []int{1, 2},
		},
		{
			name: "report with unavailable evidence stays eligible at any age without a commit read",
			prs: []schedulePR{
				{number: 1, labeled: true, updated: old, committed: old, comment: scheduleCommentUnavailable},
				{number: 2, labeled: true, updated: old, committed: old, comment: scheduleCommentCurrent},
			},
			want:         []int{1},
			wantComments: []int{1, 2},
		},
		{
			name: "pending snapshot stays eligible at any age without a commit read",
			prs: []schedulePR{
				{number: 1, labeled: true, updated: old, committed: old, comment: scheduleCommentPending},
				{number: 2, labeled: true, updated: old, committed: old, comment: scheduleCommentCurrent},
			},
			want:         []int{1},
			wantComments: []int{1, 2},
		},
		{
			name: "unlabeled PR was never published and needs no reads",
			prs: []schedulePR{
				{number: 1, updated: old, committed: old, comment: scheduleCommentCurrent},
			},
			want: []int{1},
		},
		{
			name: "clearly stale published PR costs one comment read and no commit read",
			prs: []schedulePR{
				{number: 1, labeled: true, updated: old, committed: old, comment: scheduleCommentCurrent},
			},
			wantComments: []int{1},
		},
		{
			name: "unpublished PRs come before recent commits",
			prs: []schedulePR{
				{number: 1, labeled: true, updated: now, committed: now, comment: scheduleCommentCurrent},
				{number: 2, labeled: true, updated: now, committed: old, comment: scheduleCommentCurrent},
				{number: 3, labeled: true, updated: old, committed: old, comment: scheduleCommentNone},
				{number: 4, updated: now, committed: now, comment: scheduleCommentCurrent},
				{number: 5, labeled: true, updated: now, committed: now, comment: scheduleCommentCurrent},
			},
			want:         []int{3, 4, 1, 5},
			wantComments: []int{1, 2, 3, 5},
			wantCommits:  []int{1, 2, 5},
		},
		{
			name: "open PRs include feature branch targets",
			prs: []schedulePR{
				{number: 1, updated: now, state: "closed"},
				{number: 2, updated: now, base: "feature/prerequisite"},
				{number: 3, updated: now},
			},
			want: []int{2, 3},
		},
		{
			name: "missing updated_at is not treated as stale",
			prs: []schedulePR{
				{number: 1, labeled: true, noUpdated: true, committed: now, comment: scheduleCommentCurrent},
			},
			want:         []int{1},
			wantComments: []int{1},
			wantCommits:  []int{1},
		},
		{
			name: "failed selection read includes the PR instead of hiding it",
			prs: []schedulePR{
				{number: 1, labeled: true, updated: now, committed: old, comment: scheduleCommentError},
				{number: 2, labeled: true, updated: now, committed: old, comment: scheduleCommentCurrent},
			},
			want:         []int{1},
			wantComments: []int{1, 2},
			wantCommits:  []int{2},
		},
		{name: "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &scheduleServer{prs: tc.prs}
			q := qualityTestClient(t, server.get)
			got, err := q.eventPRs(now)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantComments, server.comments)
			require.Equal(t, tc.wantCommits, server.commits)
		})
	}
}

func TestQualityScheduledSelectionPaginates(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "schedule")
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	// A full first page followed by a partial one: stale-by-age PRs on page two
	// must still be inspected, because an unpublished PR qualifies at any age.
	server := &scheduleServer{}
	var want []int
	for number := 1; number <= githubapi.PageSize+2; number++ {
		server.prs = append(server.prs, schedulePR{number: number, updated: old, committed: old})
		want = append(want, number)
	}
	q := qualityTestClient(t, server.get)
	got, err := q.eventPRs(now)
	require.NoError(t, err)
	// Unpublished candidates rotate per schedule slot: every PR is present once
	// and the order is a cyclic shift of the listing order.
	require.ElementsMatch(t, want, got)
	require.Equal(t, []int{1, 2}, server.pages)
	require.Len(t, got, githubapi.PageSize+2)
	for i := range got {
		require.Equal(t, (got[0]-1+i)%len(got)+1, got[i], "position %d", i)
	}
	require.Equal(t, []int{1, 2}, server.pages)
}

// TestQualityScheduledRotationVisitsEveryPR proves a PR that is always first in
// GitHub's listing (and, in production, hangs every run) does not keep the rest
// from leading a run: consecutive 15-minute slots start at different PRs and all
// of them lead within one full rotation.
func TestQualityScheduledRotationVisitsEveryPR(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "schedule")
	const count = 5
	start := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	led := map[int]bool{}
	var previous int
	for slot := range count {
		now := start.Add(time.Duration(slot) * qualityScheduleInterval)
		server := &scheduleServer{}
		for number := 1; number <= count; number++ {
			server.prs = append(server.prs, schedulePR{number: number, updated: now, committed: now})
		}
		q := qualityTestClient(t, server.get)
		got, err := q.eventPRs(now)
		require.NoError(t, err)
		require.Len(t, got, count)
		require.ElementsMatch(t, []int{1, 2, 3, 4, 5}, got)
		if slot > 0 {
			require.NotEqual(t, previous, got[0], "slot %d led with the same PR as the previous slot", slot)
		}
		previous = got[0]
		led[got[0]] = true
	}
	require.Len(t, led, count, "every PR must lead one run within a full rotation")
}

func TestRotatePRNumbers(t *testing.T) {
	cases := []struct {
		name    string
		numbers []int
		slot    int
		want    []int
	}{
		{name: "empty", slot: 3},
		{name: "single", numbers: []int{7}, slot: 5, want: []int{7}},
		{name: "slot zero keeps order", numbers: []int{1, 2, 3, 4}, slot: 0, want: []int{1, 2, 3, 4}},
		{name: "slot wraps by length", numbers: []int{1, 2, 3, 4}, slot: 6, want: []int{3, 4, 1, 2}},
		{name: "negative slot", numbers: []int{1, 2, 3, 4}, slot: -1, want: []int{4, 1, 2, 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := append([]int(nil), tc.numbers...)
			got := rotatePRNumbers(input, tc.slot)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.numbers, input, "input must not be mutated")
		})
	}
}

func TestQualityScheduledSelectionErrors(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "schedule")
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	q := qualityTestClient(t, func(_ string, _ any) error { return errors.New("API unavailable") })
	_, err := q.eventPRs(now)
	require.ErrorContains(t, err, "API unavailable")
	prs := make([]githubapi.PR, githubapi.PageSize)
	for i := range prs {
		prs[i] = githubtest.PR(githubtest.Run())
		prs[i].Updated = now
	}
	q = qualityTestClient(t, func(_ string, response any) error { return githubtest.Respond(prs, response) })
	_, err = q.eventPRs(now)
	require.ErrorContains(t, err, "pagination limit exceeded")
}

func TestQualityHeadCommitDate(t *testing.T) {
	author := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	committer := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name              string
		author, committer time.Time
		want              time.Time
		err               string
	}{
		{name: "rebase moves the committer date", author: author, committer: committer, want: committer},
		{name: "author date later than committer", author: committer, committer: author, want: committer},
		{name: "author only", author: author, want: author},
		{name: "no dates", err: "no author or committer date"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := qualityTestClient(t, func(path string, response any) error {
				require.Equal(t, "repos/"+githubtest.Repo+"/commits/"+githubtest.SHA, path)
				date := func(value time.Time) map[string]any {
					if value.IsZero() {
						return map[string]any{}
					}
					return map[string]any{"date": value}
				}
				return githubtest.Respond(map[string]any{"commit": map[string]any{"author": date(tc.author), "committer": date(tc.committer)}}, response)
			})
			got, err := q.headCommitDate(githubtest.SHA)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.True(t, tc.want.Equal(got), "got %s want %s", got, tc.want)
		})
	}
}
