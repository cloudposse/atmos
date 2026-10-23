//go:build mage

package release

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

func TestSelectReleaseMilestone(t *testing.T) {
	milestones := []Milestone{{Number: 1, Title: "1.233.2", State: "closed"}, {Number: 3, Title: "1.235.0", State: "open"}, {Number: 2, Title: "v1.234.0", State: "open"}, {Number: 4, Title: "Backlog", State: "open"}}
	for _, tc := range []struct {
		version string
		number  int
	}{{"", 2}, {"1.233.2", 1}, {"v1.233.2", 1}, {"1.234.0", 2}} {
		m, err := selectReleaseMilestone(milestones, tc.version)
		require.NoError(t, err)
		require.Equal(t, tc.number, m.Number)
	}
	for _, version := range []string{"1.233.9", "invalid", "1.233.2-rc.1"} {
		_, err := selectReleaseMilestone(milestones, version)
		require.Error(t, err)
	}
	_, err := selectReleaseMilestone(append(milestones, Milestone{Number: 5, Title: "v1.233.2", State: "open"}), "1.233.2")
	require.ErrorContains(t, err, "duplicate")
	_, err = selectReleaseMilestone(nil, "")
	require.ErrorContains(t, err, "no open")
}

func TestReleaseListPaginationAndStates(t *testing.T) {
	prs := map[int]listedReleasePR{
		20: {Number: 20, Title: "Merged fix", State: "closed", Merged: true},
		5:  {Number: 5, Title: "Open change", State: "open"},
		10: {Number: 10, Title: "Draft change", State: "open", Draft: true},
		15: {Number: 15, Title: "Abandoned change", State: "closed"},
	}
	pages := []string{}
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method, "list must be read-only")
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(planPrefix, "/")), "/")
		switch path {
		case "milestones":
			githubtest.JSON(t, w, []Milestone{{Number: 13, Title: "1.233.2", State: "open"}})
		case "issues":
			require.Equal(t, "all", r.URL.Query().Get("state"))
			require.Equal(t, "13", r.URL.Query().Get("milestone"))
			page := r.URL.Query().Get("page")
			pages = append(pages, page)
			items := []map[string]any{{"number": 10, "pull_request": map[string]string{"url": "unused"}}, {"number": 15, "pull_request": map[string]string{"url": "unused"}}}
			if page == "1" {
				items = make([]map[string]any, 100)
				for i := range items {
					items[i] = map[string]any{"number": 100 + i}
				}
				items[0] = map[string]any{"number": 20, "pull_request": map[string]string{"url": "unused"}}
				items[99] = map[string]any{"number": 5, "pull_request": map[string]string{"url": "unused"}}
			}
			githubtest.JSON(t, w, items)
		default:
			var number int
			_, err := fmt.Sscanf(path, "pulls/%d", &number)
			require.NoError(t, err)
			pr, ok := prs[number]
			require.True(t, ok, "issues must not be fetched as PRs")
			githubtest.JSON(t, w, pr)
		}
	})
	m, got, err := c.listRelease("")
	require.NoError(t, err)
	require.Equal(t, 13, m.Number)
	require.Equal(t, []listedReleasePR{prs[5], prs[10], prs[15], prs[20]}, got)
	require.Equal(t, []string{"1", "2"}, pages)
}

func TestReleaseListUsesAlignedColumnsWithoutColor(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	prs := []listedReleasePR{
		{Number: 1, Title: "Short", Merged: true},
		{Number: 2222, Title: "Longer Unicode title 界", State: "open"},
		{Number: 3, Title: "Draft\nwith\ttabs", State: "open", Draft: true},
		{Number: 4, Title: "\x1b[31mClosed", State: "closed"},
	}
	output := renderReleaseList(&Milestone{Title: "v1.233.2"}, prs)
	require.NotContains(t, output, "\x1b")
	require.Contains(t, output, "Release 1.233.2")
	positions := []int{}
	for _, line := range strings.Split(output, "\n") {
		for _, status := range []string{"merged", "open", "draft", "closed (unmerged)"} {
			if strings.HasSuffix(strings.TrimSpace(line), status) {
				index := strings.LastIndex(line, status)
				positions = append(positions, lipgloss.Width(line[:index]))
				if status == "merged" {
					require.Contains(t, line, "●")
				} else {
					require.Contains(t, line, "○")
				}
				break
			}
		}
	}
	require.Len(t, positions, 4)
	for _, position := range positions {
		require.Equal(t, positions[0], position, "status column must align by display width")
	}
	require.Contains(t, output, "Draft with tabs")
	require.Contains(t, renderReleaseList(&Milestone{Title: "1.233.2"}, nil), "No pull requests assigned")
}

func TestReleaseListDoesNotHideAPIFailures(t *testing.T) {
	for _, failure := range []string{"milestones", "issues", "pulls/42"} {
		t.Run(failure, func(t *testing.T) {
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(planPrefix, "/")), "/")
				if path == failure {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				if path == "milestones" {
					githubtest.JSON(t, w, []Milestone{{Number: 13, Title: "1.233.2", State: "open"}})
					return
				}
				githubtest.JSON(t, w, []map[string]any{{"number": 42, "pull_request": map[string]string{"url": "unused"}}})
			})
			_, prs, err := c.listRelease("")
			require.Error(t, err)
			require.Nil(t, prs)
		})
	}
}

func TestReleaseListColorsOnlyStatusDots(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	theme.InvalidateStyleCache()
	t.Cleanup(func() { lipgloss.SetColorProfile(profile); theme.InvalidateStyleCache() })
	prs := []listedReleasePR{
		{Number: 1, Title: "Merged", Merged: true},
		{Number: 2, Title: "Open", State: "open"},
		{Number: 3, Title: "Draft", State: "open", Draft: true},
		{Number: 4, Title: "Closed", State: "closed"},
	}
	colors := []lipgloss.Style{theme.GetSuccessStyle(), theme.GetInfoStyle(), theme.GetWarningStyle(), theme.GetErrorStyle()}
	output := renderReleaseList(&Milestone{Title: "1.233.2"}, prs)
	for i, pr := range prs {
		dot, _ := releasePRStatus(pr)
		require.Equal(t, colors[i].GetForeground(), releasePRStyle(pr).GetForeground())
		require.Contains(t, output, colors[i].Render(dot))
	}
}
