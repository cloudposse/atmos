package release

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cloudposse/atmos/magefiles/release/milestone"
	"github.com/cloudposse/atmos/pkg/data"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// List displays all PRs assigned to a milestone, independently of release eligibility.
func (Release) List(repo, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	c, err := newMilestoneClient(ctx, repo)
	if err != nil {
		return err
	}
	m, prs, err := c.listRelease(version)
	if err != nil {
		return err
	}
	ioCtx, err := atmosio.NewContext()
	if err != nil {
		return err
	}
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)
	return data.Writeln(renderReleaseList(m, prs))
}

type listedReleasePR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Merged bool   `json:"merged"`
	Draft  bool   `json:"draft"`
}

func (c *milestoneClient) listRelease(version string) (*Milestone, []listedReleasePR, error) {
	milestones, err := c.ListMilestones()
	if err != nil {
		return nil, nil, err
	}
	m, err := selectReleaseMilestone(milestones, version)
	if err != nil {
		return nil, nil, err
	}
	items, err := milestonePages[milestoneItem](c, fmt.Sprintf("issues?state=all&milestone=%d", m.Number))
	if err != nil {
		return nil, nil, err
	}
	prs := make([]listedReleasePR, 0, len(items))
	for _, item := range items {
		if !item.isPullRequest() {
			continue
		}
		var pr listedReleasePR
		if err = c.Get(fmt.Sprintf("pulls/%d", item.Number), &pr); err != nil {
			return nil, nil, err
		}
		prs = append(prs, pr)
	}
	slices.SortFunc(prs, func(a, b listedReleasePR) int { return a.Number - b.Number })
	return m, prs, nil
}

func selectReleaseMilestone(milestones []Milestone, version string) (*Milestone, error) {
	if version == "" {
		return milestone.NextMilestone(milestones)
	}
	want, err := milestone.MilestoneVersion(version)
	if err != nil {
		return nil, err
	}
	var selected *Milestone
	for _, m := range milestones {
		v, parseErr := milestone.MilestoneVersion(m.Title)
		if parseErr != nil || !v.Equal(want) {
			continue
		}
		if selected != nil {
			return nil, fmt.Errorf("%w: duplicate milestones for %s", ErrMilestonePolicy, version)
		}
		selected = &m
	}
	if selected == nil {
		return nil, fmt.Errorf("%w: release milestone %s not found", ErrMilestonePolicy, version)
	}
	return selected, nil
}

func renderReleaseList(m *Milestone, prs []listedReleasePR) string {
	heading := "Release " + strings.TrimPrefix(m.Title, "v") + "\n\n"
	if len(prs) == 0 {
		return heading + "No pull requests assigned to this milestone."
	}
	rows := make([][]string, 0, len(prs))
	for _, pr := range prs {
		dot, state := releasePRStatus(pr)
		// Keep untrusted titles on one row without terminal control sequences.
		title := strings.Join(strings.Fields(ansi.Strip(pr.Title)), " ")
		// Normalize whitespace first so removing controls does not join words.
		title = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, title)
		rows = append(rows, []string{dot, fmt.Sprintf("#%d", pr.Number), title, state})
	}
	config := theme.TableConfig{Style: theme.TableStylePlain, StyleFunc: func(row, col int) lipgloss.Style {
		style := lipgloss.NewStyle().PaddingRight(2)
		if row >= 0 && col == 0 {
			return style.Inherit(releasePRStyle(prs[row]))
		}
		return style
	}}
	return heading + theme.CreateTable(&config, []string{"", "PR", "Title", "Status"}, rows)
}

func releasePRStatus(pr listedReleasePR) (string, string) {
	switch {
	case pr.Merged:
		return "●", "merged"
	case pr.State == "closed":
		return "○", "closed (unmerged)"
	case pr.Draft:
		return "○", "draft"
	default:
		return "○", "open"
	}
}

func releasePRStyle(pr listedReleasePR) lipgloss.Style {
	switch {
	case pr.Merged:
		return theme.GetSuccessStyle()
	case pr.State == "closed":
		return theme.GetErrorStyle()
	case pr.Draft:
		return theme.GetWarningStyle()
	default:
		return theme.GetInfoStyle()
	}
}
