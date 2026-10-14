//go:build mage

package qualitygate

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func TestQualityScheduledSelection(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "schedule")
	t.Setenv("GITHUB_EVENT_PATH", "")
	now := time.Date(2026, 10, 13, 1, 0, 0, 0, time.FixedZone("test", -5*60*60))
	cutoff := now.UTC().Add(-24 * time.Hour)
	pr := func(number int, updated time.Time) githubapi.PR {
		value := githubtest.PR(githubtest.Run())
		value.Number, value.Updated = number, updated
		return value
	}
	full := make([]githubapi.PR, githubapi.PageSize)
	var all []int
	for i := range full {
		full[i] = pr(i+1, now)
		all = append(all, i+1)
	}
	stopsWithinPage := append([]githubapi.PR(nil), full...)
	stopsWithinPage[1] = pr(2, cutoff.Add(-time.Nanosecond))
	closed, otherBase := pr(2, now), pr(3, now)
	closed.State, otherBase.Base.Ref = "closed", "release"
	cases := []struct {
		name  string
		pages [][]githubapi.PR
		want  []int
		err   string
	}{
		{name: "recent and inclusive cutoff", pages: [][]githubapi.PR{{pr(1, now), pr(2, cutoff), pr(3, cutoff.Add(-time.Nanosecond))}}, want: []int{1, 2}},
		{name: "empty", pages: [][]githubapi.PR{{}}},
		{name: "all old", pages: [][]githubapi.PR{{pr(1, cutoff.Add(-time.Hour))}}},
		{name: "missing timestamp", pages: [][]githubapi.PR{{pr(1, now), pr(2, time.Time{})}}, err: "PR #2: updated_at missing"},
		{name: "only open main PRs", pages: [][]githubapi.PR{{pr(1, now), closed, otherBase}}, want: []int{1}},
		{name: "stop within full page", pages: [][]githubapi.PR{stopsWithinPage}, want: []int{1}},
		{name: "stop on next page", pages: [][]githubapi.PR{full, {pr(101, cutoff), pr(102, cutoff.Add(-time.Second))}}, want: append(append([]int(nil), all...), 101)},
		{name: "full page then empty", pages: [][]githubapi.PR{full, {}}, want: all},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			q := qualityTestClient(t, func(path string, response any) error {
				calls++
				require.LessOrEqual(t, calls, len(tc.pages), "must not fetch pages beyond the cutoff")
				require.Equal(t, fmt.Sprintf("repos/%s/pulls?state=open&base=main&sort=updated&direction=desc&per_page=100&page=%d", githubtest.Repo, calls), path)
				return githubtest.Respond(tc.pages[calls-1], response)
			})
			got, err := q.eventPRs(now)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				require.Nil(t, got, "do not reconcile a partial selection on error")
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
			require.Equal(t, len(tc.pages), calls)
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
