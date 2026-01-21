package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInitModel(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("mycomponent", "mystack", "init", "", reader)

	assert.NotNil(t, m)
	assert.Equal(t, "mycomponent", m.component)
	assert.Equal(t, "mystack", m.stack)
	assert.Equal(t, "init", m.subCommand)
	assert.Empty(t, m.workspace)
	assert.NotNil(t, m.scanner)
	assert.NotNil(t, m.clock)
	assert.False(t, m.done)
	assert.Equal(t, 0, m.exitCode)
}

func TestNewInitModel_WithWorkspace(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("mycomponent", "mystack", "workspace", "dev", reader)

	assert.Equal(t, "workspace", m.subCommand)
	assert.Equal(t, "dev", m.workspace)
}

func TestNewInitModel_WithClock(t *testing.T) {
	reader := strings.NewReader("")
	clock := newTestClock()
	m := NewInitModel("comp", "stack", "init", "", reader, WithInitClock(clock))

	assert.Equal(t, clock, m.clock)
	assert.Equal(t, clock.Now(), m.startTime)
}

func TestInitModel_Update_KeyMsg_Quit(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyMsg
	}{
		{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}},
		{"q key", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader("")
			m := NewInitModel("comp", "stack", "init", "", reader)

			updated, cmd := m.Update(tt.key)
			_ = updated.(InitModel)

			assert.NotNil(t, cmd) // Should return tea.Quit.
		})
	}
}

func TestInitModel_Update_SpinnerTickMsg(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("comp", "stack", "init", "", reader)

	updated, cmd := m.Update(spinner.TickMsg{})
	_ = updated.(InitModel)

	assert.NotNil(t, cmd) // Spinner returns its own tick cmd.
}

func TestInitModel_Update_InitLineMsg_Initializing(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("comp", "stack", "init", "", reader)

	updated, cmd := m.Update(initLineMsg{line: "Initializing the backend..."})
	model := updated.(InitModel)

	assert.Equal(t, "Initializing the backend...", model.currentOp)
	assert.Empty(t, model.lines)
	assert.NotNil(t, cmd)
}

func TestInitModel_Update_InitLineMsg_Provider(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("comp", "stack", "init", "", reader)

	updated, cmd := m.Update(initLineMsg{line: "- Installing hashicorp/aws v5.0.0..."})
	model := updated.(InitModel)

	require.Len(t, model.lines, 1)
	assert.Equal(t, "- Installing hashicorp/aws v5.0.0...", model.lines[0])
	assert.NotNil(t, cmd)
}

func TestInitModel_Update_InitLineMsg_Module(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("comp", "stack", "init", "", reader)

	updated, cmd := m.Update(initLineMsg{line: "* module.vpc in .terraform/modules"})
	model := updated.(InitModel)

	require.Len(t, model.lines, 1)
	assert.Equal(t, "* module.vpc in .terraform/modules", model.lines[0])
	assert.NotNil(t, cmd)
}

func TestInitModel_Update_InitLineMsg_Success(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("comp", "stack", "init", "", reader)

	updated, _ := m.Update(initLineMsg{line: "Terraform has been successfully initialized!"})
	model := updated.(InitModel)

	assert.Equal(t, "Initialized successfully", model.currentOp)
}

func TestInitModel_Update_InitLineMsg_MaxLines(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("comp", "stack", "init", "", reader)

	// Add more than initMaxLines (6) lines.
	var model InitModel
	for i := 0; i < 10; i++ {
		updated, _ := m.Update(initLineMsg{line: "- provider_" + string(rune('a'+i))})
		model = updated.(InitModel)
		// Update the model for the next iteration.
		m.lines = model.lines
	}

	// Should only keep the last 6 lines.
	assert.LessOrEqual(t, len(model.lines), initMaxLines)
	assert.Equal(t, initMaxLines, len(model.lines))
}

func TestInitModel_Update_InitLineMsg_EmptyLine(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("comp", "stack", "init", "", reader)

	updated, cmd := m.Update(initLineMsg{line: ""})
	model := updated.(InitModel)

	// Empty lines should not add to lines.
	assert.Empty(t, model.lines)
	assert.NotNil(t, cmd)
}

func TestInitModel_Update_InitLineMsg_PlainText(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("comp", "stack", "init", "", reader)

	updated, cmd := m.Update(initLineMsg{line: "Some random text that doesn't match patterns"})
	model := updated.(InitModel)

	// Plain text shouldn't update currentOp or add to lines.
	assert.Empty(t, model.currentOp)
	assert.Empty(t, model.lines)
	assert.NotNil(t, cmd)
}

func TestInitModel_Update_InitDoneMsg_Success(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("comp", "stack", "init", "", reader)

	updated, cmd := m.Update(initDoneMsg{exitCode: 0, err: nil})
	model := updated.(InitModel)

	assert.True(t, model.done)
	assert.Equal(t, 0, model.exitCode)
	assert.Nil(t, model.err)
	assert.NotNil(t, cmd) // Should return tea.Quit.
}

func TestInitModel_Update_InitDoneMsg_Error(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("comp", "stack", "init", "", reader)

	updated, cmd := m.Update(initDoneMsg{exitCode: 1, err: assert.AnError})
	model := updated.(InitModel)

	assert.True(t, model.done)
	assert.Equal(t, 1, model.exitCode)
	assert.Equal(t, assert.AnError, model.err)
	assert.NotNil(t, cmd)
}

func TestInitModel_View_InProgress(t *testing.T) {
	clock := newTestClock()
	reader := strings.NewReader("")
	m := NewInitModel("myapp", "dev", "init", "", reader, WithInitClock(clock))

	clock.advance(3 * time.Second)

	view := m.View()

	assert.Contains(t, view, "Init")
	assert.Contains(t, view, "dev")
	assert.Contains(t, view, "myapp")
	assert.Contains(t, view, "3.0s")
}

func TestInitModel_View_InProgress_WithCurrentOp(t *testing.T) {
	clock := newTestClock()
	reader := strings.NewReader("")
	m := NewInitModel("myapp", "dev", "init", "", reader, WithInitClock(clock))
	m.currentOp = "Initializing the backend..."

	view := m.View()

	assert.Contains(t, view, "Initializing the backend...")
}

func TestInitModel_View_InProgress_WithLines(t *testing.T) {
	clock := newTestClock()
	reader := strings.NewReader("")
	m := NewInitModel("myapp", "dev", "init", "", reader, WithInitClock(clock))
	m.lines = []string{"- Installing hashicorp/aws v5.0.0..."}

	view := m.View()

	assert.Contains(t, view, "Installing hashicorp/aws")
}

func TestInitModel_View_Done_Success(t *testing.T) {
	clock := newTestClock()
	reader := strings.NewReader("")
	m := NewInitModel("myapp", "dev", "init", "", reader, WithInitClock(clock))
	m.done = true

	clock.advance(5 * time.Second)

	view := m.View()

	assert.Contains(t, view, "Init")
	assert.Contains(t, view, "dev/myapp")
	assert.Contains(t, view, "completed")
	assert.Contains(t, view, "5.0s")
}

func TestInitModel_View_Done_Error(t *testing.T) {
	clock := newTestClock()
	reader := strings.NewReader("")
	m := NewInitModel("myapp", "dev", "init", "", reader, WithInitClock(clock))
	m.done = true
	m.exitCode = 1
	m.err = assert.AnError

	clock.advance(2 * time.Second)

	view := m.View()

	assert.Contains(t, view, "Init")
	assert.Contains(t, view, "failed")
	assert.Contains(t, view, "2.0s")
}

func TestInitModel_View_Done_Workspace(t *testing.T) {
	clock := newTestClock()
	reader := strings.NewReader("")
	m := NewInitModel("myapp", "dev", "workspace", "staging", reader, WithInitClock(clock))
	m.done = true

	clock.advance(1 * time.Second)

	view := m.View()

	assert.Contains(t, view, "Selected")
	assert.Contains(t, view, "staging")
	assert.Contains(t, view, "workspace")
	assert.Contains(t, view, "dev/myapp")
	// Workspace doesn't say "completed".
	assert.NotContains(t, view, "completed")
}

func TestInitModel_FormatAction(t *testing.T) {
	tests := []struct {
		name       string
		subCommand string
		workspace  string
		expected   string
	}{
		{"init command", "init", "", "Init"},
		{"workspace with name", "workspace", "dev", "Selected `dev` workspace for"},
		{"workspace without name", "workspace", "", "Selected workspace for"},
		{"custom command", "custom", "", "Custom"},
		{"empty command", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader("")
			m := NewInitModel("comp", "stack", tt.subCommand, tt.workspace, reader)
			result := m.formatAction()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInitModel_GetError(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("comp", "stack", "init", "", reader)
	m.err = assert.AnError

	assert.Equal(t, assert.AnError, m.GetError())
}

func TestInitModel_GetExitCode(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("comp", "stack", "init", "", reader)
	m.exitCode = 42

	assert.Equal(t, 42, m.GetExitCode())
}

func TestInitModel_Init(t *testing.T) {
	reader := strings.NewReader("Initializing...\n")
	m := NewInitModel("comp", "stack", "init", "", reader)

	cmd := m.Init()

	// Init should return a batch command.
	assert.NotNil(t, cmd)
}

func TestInitModel_Update_LineWithANSI(t *testing.T) {
	reader := strings.NewReader("")
	m := NewInitModel("comp", "stack", "init", "", reader)

	// Line with ANSI codes should be stripped.
	updated, _ := m.Update(initLineMsg{line: "\x1b[32m- Installing hashicorp/aws\x1b[0m"})
	model := updated.(InitModel)

	require.Len(t, model.lines, 1)
	// Should be stripped of ANSI codes.
	assert.Equal(t, "- Installing hashicorp/aws", model.lines[0])
}
