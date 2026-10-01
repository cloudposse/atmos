package testreport

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/ui/tree"
)

func TestTreeCountsAndConnectivity(t *testing.T) {
	var output bytes.Buffer
	r := New("Checks", []*Node{{ID: "group", Name: "Parallel", Children: []*Node{{ID: "a", Name: "First"}, {ID: "b", Name: "Second"}}}, {ID: "c", Name: "Last"}}, &output)
	r.Update("b", "failed", time.Second, "first line\nsecond line")
	r.Update("a", "passed", time.Second, "")
	r.Update("c", "skipped", 0, "")
	view := ansi.Strip(r.View(100, "", true))
	assert.Empty(t, tree.Violations(strings.Split(strings.Split(view, "\n\n")[0], "\n")))
	assert.Contains(t, view, "3/3")
	assert.Equal(t, 3, r.Counts()["total"])
	assert.NotContains(t, view, "first line")
	assert.NoError(t, r.Finish())
	assert.Equal(t, 1, strings.Count(output.String(), "first line"))
	assert.Regexp(t, `3/3 .* · [0-9]+\.[0-9]s elapsed`, ansi.Strip(output.String()))
	assert.NotContains(t, ansi.Strip(r.View(25, "", false)), "\x1b")
}

func TestLiveReporterCompletionDoesNotDeadlock(t *testing.T) {
	var output bytes.Buffer
	r := New("Checks", []*Node{{ID: "a", Name: "Check"}}, &output)
	r.Start(true, func() {})
	done := make(chan error, 1)
	go func() {
		r.Update("a", "running", 0, "")
		r.Update("a", "failed", time.Second, strings.Repeat("failure detail\n", 200))
		done <- r.Finish()
	}()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		r.program.Kill()
		t.Fatal("live reporter deadlocked")
	}
	assert.Regexp(t, `1/1 .* · [0-9]+\.[0-9]s elapsed`, ansi.Strip(output.String()))
	assert.Equal(t, 200, strings.Count(output.String(), "failure detail"))
}

func TestReporterResizeAndInterruption(t *testing.T) {
	var output bytes.Buffer
	canceled := false
	r := New("Checks", []*Node{{ID: "a", Name: strings.Repeat("long-check-name", 10)}}, &output)
	r.Start(false, func() { canceled = true })
	m := &model{report: r}
	for _, width := range []int{120, 32, 80} {
		m.Update(tea.WindowSizeMsg{Width: width})
		view := ansi.Strip(m.View())
		assert.Contains(t, view, "…")
		for _, line := range strings.Split(view, "\n") {
			if strings.Contains(line, "└──") {
				assert.LessOrEqual(t, ansi.StringWidth(line), width)
			}
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.True(t, canceled)
	r.Update("a", Canceled, time.Second, "")
	m.Update(finishMsg{})
	assert.Empty(t, m.View())
	assert.Contains(t, r.View(80, "", true), "1 canceled")
	assert.NoError(t, r.Finish())
	// Forced color is valid in static output, but cursor and other terminal controls are not.
	plain := regexp.MustCompile(`\x1b\[[0-9;:]*m`).ReplaceAllString(output.String(), "")
	assert.NotContains(t, plain, "\x1b")
}

func TestLiveReporterPreservesTallTree(t *testing.T) {
	var output bytes.Buffer
	nodes := make([]*Node, 80)
	for i := range nodes {
		nodes[i] = &Node{ID: fmt.Sprintf("case-%03d", i), Name: fmt.Sprintf("case-%03d", i), Status: Passed}
	}
	r := New("Long suite", nodes, &output)
	r.Start(true, func() {})
	r.program.Send(tea.WindowSizeMsg{Width: 80, Height: 12})
	assert.NoError(t, r.Finish())
	for _, node := range nodes {
		assert.Contains(t, output.String(), node.Name)
	}
	assert.Contains(t, output.String(), "80/80")
}

func TestReporterFailureLabels(t *testing.T) {
	for _, tc := range []struct{ name, id, heading, label string }{
		{"suite setup", "", "Post-deployment tests", "Setup"},
		{"known case", "check", "Health", "Health"},
		{"unknown case", "unknown", "unknown", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			r := New("Post-deployment tests", []*Node{{ID: "check", Name: "Health"}}, &output)
			r.Update(tc.id, Failed, 0, "setup failed")
			plain := ansi.Strip(output.String())
			assert.Contains(t, plain, "     "+tc.heading+"\n")
			assert.Contains(t, plain, "└── "+tc.label+"\n")
			assert.Equal(t, 1, strings.Count(plain, "setup failed"))
		})
	}
}
