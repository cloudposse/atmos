package workflow

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	mouseZone "github.com/lrstanley/bubblezone"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func newTestApp() *App {
	mouseZone.NewGlobal()
	workflows := map[string]schema.WorkflowManifest{
		"deploy.yaml": {
			Name: "deploy",
			Workflows: schema.WorkflowConfig{
				"deploy-all": {
					Description: "Deploy everything",
					Steps: []schema.WorkflowStep{
						{Name: "plan", Command: "atmos terraform plan vpc -s dev"},
						{Name: "apply", Command: "atmos terraform apply vpc -s dev"},
					},
				},
				"deploy-vpc": {
					Description: "Deploy VPC only",
					Steps: []schema.WorkflowStep{
						{Name: "plan-vpc", Command: "atmos terraform plan vpc -s dev"},
					},
				},
			},
		},
	}
	app := NewApp(workflows)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return app
}

func focusColumn(app *App, col int) {
	app.columnViews[app.columnPointer].Blur()
	app.columnPointer = col
	app.columnViews[col].Focus()
}

func activateFilter(t *testing.T, app *App) {
	t.Helper()
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	require.True(t, app.columnViews[app.columnPointer].list.SettingFilter(),
		"list should be in filtering mode after pressing /")
}

func TestVimKeysDelegatedDuringFilter(t *testing.T) {
	app := newTestApp()
	focusColumn(app, 1)
	activateFilter(t, app)

	startCol := app.columnPointer

	for _, r := range []rune{'h', 'j', 'k', 'l'} {
		app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		assert.Equal(t, startCol, app.columnPointer,
			"pressing '%c' during filter should not change focused column", r)
	}

	assert.True(t, app.columnViews[startCol].list.SettingFilter(),
		"should still be filtering after typing vim keys")
	assert.False(t, app.quit)
}

func TestVimKeysNavigateOutsideFilter(t *testing.T) {
	app := newTestApp()
	focusColumn(app, 1)

	assert.False(t, app.columnViews[1].list.SettingFilter())

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	assert.Equal(t, 0, app.columnPointer, "'h' should navigate left")

	focusColumn(app, 1)
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, 2, app.columnPointer, "'l' should navigate right")
}

func TestCtrlCQuitsDuringFilter(t *testing.T) {
	app := newTestApp()
	focusColumn(app, 1)
	activateFilter(t, app)

	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	assert.True(t, app.quit)
	assert.NotNil(t, cmd, "ctrl+c during filter should return quit cmd")
}

func TestEscapeClearsFilterThenQuits(t *testing.T) {
	app := newTestApp()
	focusColumn(app, 1)
	activateFilter(t, app)

	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.Equal(t, list.Unfiltered, app.columnViews[1].list.FilterState(),
		"first escape should clear the filter")
	assert.False(t, app.quit, "first escape should not quit")

	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.True(t, app.quit, "second escape should quit")
}
