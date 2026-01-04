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

// mockClock implements Clock for testing with controllable time.
type mockClock struct {
	now time.Time
}

func (c *mockClock) Now() time.Time {
	return c.now
}

func (c *mockClock) Since(t time.Time) time.Duration {
	return c.now.Sub(t)
}

// newTestClock creates a mock clock at a fixed time.
func newTestClock() *mockClock {
	return &mockClock{
		now: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}
}

// advance moves the mock clock forward by the given duration.
func (c *mockClock) advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func TestNewModel(t *testing.T) {
	reader := strings.NewReader("")
	m := NewModel("mycomponent", "mystack", "plan", reader)

	assert.NotNil(t, m)
	assert.Equal(t, "mycomponent", m.component)
	assert.Equal(t, "mystack", m.stack)
	assert.Equal(t, "plan", m.command)
	assert.NotNil(t, m.tracker)
	assert.NotNil(t, m.parser)
	assert.NotNil(t, m.clock)
	assert.False(t, m.done)
	assert.Equal(t, 0, m.exitCode)
}

func TestNewModel_WithClock(t *testing.T) {
	reader := strings.NewReader("")
	clock := newTestClock()
	m := NewModel("comp", "stack", "apply", reader, WithClock(clock))

	assert.Equal(t, clock, m.clock)
	assert.Equal(t, clock.Now(), m.startTime)
}

func TestModel_Update_WindowSizeMsg(t *testing.T) {
	tests := []struct {
		name           string
		width          int
		height         int
		expectedWidth  int
		expectedHeight int
	}{
		{"small terminal", 80, 24, 80, 24},
		{"large terminal", 200, 50, 200, 50},
		{"minimum width", 30, 10, 30, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader("")
			m := NewModel("comp", "stack", "plan", reader)

			updated, cmd := m.Update(tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			model := updated.(Model)

			assert.Equal(t, tt.expectedWidth, model.width)
			assert.Equal(t, tt.expectedHeight, model.height)
			assert.Nil(t, cmd)
		})
	}
}

func TestModel_Update_KeyMsg_Quit(t *testing.T) {
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
			m := NewModel("comp", "stack", "plan", reader)

			updated, cmd := m.Update(tt.key)
			model := updated.(Model)

			assert.True(t, model.done)
			assert.NotNil(t, cmd) // Should return tea.Quit.
		})
	}
}

func TestModel_Update_MessageMsg(t *testing.T) {
	reader := strings.NewReader("")
	m := NewModel("comp", "stack", "plan", reader)

	// Send a planned change message.
	msg := messageMsg{
		result: &ParseResult{
			Message: &PlannedChangeMessage{
				Change: PlannedChange{
					Resource: ResourceAddr{
						Addr:         "aws_vpc.main",
						ResourceType: "aws_vpc",
						ResourceName: "main",
					},
					Action: "create",
				},
			},
		},
	}

	updated, cmd := m.Update(msg)
	model := updated.(Model)

	assert.Equal(t, 1, model.tracker.GetTotalCount())
	assert.NotNil(t, cmd) // Should return listenForMessages cmd.
}

func TestModel_Update_MessageMsg_WithError(t *testing.T) {
	reader := strings.NewReader("")
	m := NewModel("comp", "stack", "plan", reader)

	// Send a message with a parse error.
	msg := messageMsg{
		result: &ParseResult{
			Err: assert.AnError,
		},
	}

	updated, cmd := m.Update(msg)
	model := updated.(Model)

	// Tracker should not be updated on error.
	assert.Equal(t, 0, model.tracker.GetTotalCount())
	assert.NotNil(t, cmd) // Should still return listenForMessages cmd.
}

func TestModel_Update_DoneMsg(t *testing.T) {
	tests := []struct {
		name       string
		exitCode   int
		err        error
		expectDone bool
		expectCode int
	}{
		{"success", 0, nil, true, 0},
		{"failure with exit code", 1, nil, true, 1},
		{"failure with error", 1, assert.AnError, true, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader("")
			m := NewModel("comp", "stack", "plan", reader)

			updated, cmd := m.Update(doneMsg{exitCode: tt.exitCode, err: tt.err})
			model := updated.(Model)

			assert.Equal(t, tt.expectDone, model.done)
			assert.Equal(t, tt.expectCode, model.exitCode)
			assert.NotNil(t, cmd) // Should return tea.Quit.
		})
	}
}

func TestModel_Update_TickMsg(t *testing.T) {
	reader := strings.NewReader("")
	m := NewModel("comp", "stack", "plan", reader)

	updated, cmd := m.Update(tickMsg(time.Now()))
	_ = updated.(Model)

	assert.NotNil(t, cmd) // Should return another tick cmd.
}

func TestModel_Update_SpinnerTickMsg(t *testing.T) {
	reader := strings.NewReader("")
	m := NewModel("comp", "stack", "plan", reader)

	// Trigger spinner tick.
	updated, cmd := m.Update(spinner.TickMsg{})
	_ = updated.(Model)

	assert.NotNil(t, cmd) // Spinner returns its own tick cmd.
}

func TestModel_View_Progress(t *testing.T) {
	clock := newTestClock()
	reader := strings.NewReader("")
	m := NewModel("myapp", "dev", "plan", reader, WithClock(clock))
	m.width = 120

	// Advance time a bit.
	clock.advance(2 * time.Second)

	view := m.View()

	// Should contain the command, stack, component.
	assert.Contains(t, view, "plan")
	assert.Contains(t, view, "dev")
	assert.Contains(t, view, "myapp")
	// Should show elapsed time.
	assert.Contains(t, view, "2.0s")
}

func TestModel_View_Progress_WithResources(t *testing.T) {
	clock := newTestClock()
	reader := strings.NewReader("")
	m := NewModel("myapp", "dev", "apply", reader, WithClock(clock))
	m.width = 120

	// Add resources via tracker.
	m.tracker.HandleMessage(&PlannedChangeMessage{
		Change: PlannedChange{
			Resource: ResourceAddr{Addr: "aws_vpc.main"},
			Action:   "create",
		},
	})
	m.tracker.HandleMessage(&ApplyStartMessage{
		Hook: ApplyHook{
			Resource: ResourceAddr{Addr: "aws_vpc.main"},
			Action:   "create",
		},
	})
	m.tracker.HandleMessage(&ApplyCompleteMessage{
		Hook: ApplyHook{
			Resource:    ResourceAddr{Addr: "aws_vpc.main"},
			Action:      "create",
			ElapsedSecs: 5,
		},
	})

	view := m.View()

	// Should show completed resource.
	assert.Contains(t, view, "aws_vpc.main")
	assert.Contains(t, view, "Created")
	assert.Contains(t, view, "5.0s")
}

func TestModel_View_Done_Success(t *testing.T) {
	clock := newTestClock()
	reader := strings.NewReader("")
	m := NewModel("myapp", "dev", "apply", reader, WithClock(clock))
	m.done = true

	clock.advance(10 * time.Second)

	view := m.View()

	// Should show success and elapsed time.
	assert.Contains(t, view, "Apply")
	assert.Contains(t, view, "dev/myapp")
	assert.Contains(t, view, "completed")
	assert.Contains(t, view, "10.0s")
}

func TestModel_View_Done_WithErrors(t *testing.T) {
	clock := newTestClock()
	reader := strings.NewReader("")
	m := NewModel("myapp", "dev", "apply", reader, WithClock(clock))
	m.done = true

	// Add error diagnostic.
	m.tracker.HandleMessage(&DiagnosticMessage{
		Diagnostic: Diagnostic{
			Severity: "error",
			Summary:  "Resource creation failed",
			Detail:   "Could not create VPC",
		},
	})

	clock.advance(5 * time.Second)

	view := m.View()

	// Should show error information.
	assert.Contains(t, view, "failed")
	assert.Contains(t, view, "error")
}

func TestModel_View_Done_NoChanges(t *testing.T) {
	clock := newTestClock()
	reader := strings.NewReader("")
	m := NewModel("myapp", "dev", "apply", reader, WithClock(clock))
	m.done = true

	// Add change summary with no changes.
	m.tracker.HandleMessage(&ChangeSummaryMessage{
		Changes: Changes{
			Add:       0,
			Change:    0,
			Remove:    0,
			Operation: "apply",
		},
	})

	view := m.View()

	assert.Contains(t, view, "no changes")
}

func TestModel_View_Done_Destroy(t *testing.T) {
	clock := newTestClock()
	reader := strings.NewReader("")
	m := NewModel("myapp", "dev", "apply", reader, WithClock(clock))
	m.done = true

	// Add change summary with only deletions - should show "Destroy" instead of "Apply".
	m.tracker.HandleMessage(&ChangeSummaryMessage{
		Changes: Changes{
			Add:       0,
			Change:    0,
			Remove:    5,
			Operation: "apply",
		},
	})

	view := m.View()

	assert.Contains(t, view, "Destroy")
}

func TestFormatActionVerb(t *testing.T) {
	reader := strings.NewReader("")
	m := NewModel("comp", "stack", "plan", reader)

	tests := []struct {
		state    ResourceState
		action   string
		expected string
	}{
		{ResourceStateRefreshing, "", "Reading"},
		{ResourceStateInProgress, "create", "Creating"},
		{ResourceStateInProgress, "update", "Updating"},
		{ResourceStateInProgress, "delete", "Destroying"},
		{ResourceStateInProgress, "read", "Reading"},
		{ResourceStateInProgress, "unknown", "Processing"},
		{ResourceStatePending, "", "Processing"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			op := &ResourceOperation{State: tt.state, Action: tt.action}
			result := m.formatActivityVerb(op)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatActionPending(t *testing.T) {
	reader := strings.NewReader("")
	m := NewModel("comp", "stack", "plan", reader)

	tests := []struct {
		action   string
		expected string
	}{
		{"create", "Create"},
		{"read", "Read"},
		{"update", "Update"},
		{"delete", "Destroy"},
		{"no-op", "No change"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			result := m.formatActionPending(tt.action)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatActionInProgress(t *testing.T) {
	reader := strings.NewReader("")
	m := NewModel("comp", "stack", "plan", reader)

	tests := []struct {
		action   string
		expected string
	}{
		{"create", "Creating"},
		{"read", "Reading"},
		{"update", "Updating"},
		{"delete", "Destroying"},
		{"no-op", "No change"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			result := m.formatActionInProgress(tt.action)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatActionComplete(t *testing.T) {
	reader := strings.NewReader("")
	m := NewModel("comp", "stack", "plan", reader)

	tests := []struct {
		action   string
		expected string
	}{
		{"create", "Created"},
		{"read", "Read"},
		{"update", "Updated"},
		{"delete", "Destroyed"},
		{"no-op", "No change"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			result := m.formatActionComplete(tt.action)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCapitalizeCommand(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"plan", "Plan"},
		{"apply", "Apply"},
		{"destroy", "Destroy"},
		{"init", "Init"},
		{"", ""},
		{"ALREADY", "ALREADY"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := capitalizeCommand(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestModel_GetExitCode(t *testing.T) {
	reader := strings.NewReader("")
	m := NewModel("comp", "stack", "plan", reader)
	m.exitCode = 42

	assert.Equal(t, 42, m.GetExitCode())
}

func TestModel_GetError(t *testing.T) {
	reader := strings.NewReader("")
	m := NewModel("comp", "stack", "plan", reader)
	m.err = assert.AnError

	assert.Equal(t, assert.AnError, m.GetError())
}

func TestModel_GetTracker(t *testing.T) {
	reader := strings.NewReader("")
	m := NewModel("comp", "stack", "plan", reader)

	tracker := m.GetTracker()
	require.NotNil(t, tracker)
	assert.Equal(t, m.tracker, tracker)
}

func TestModel_Init(t *testing.T) {
	reader := strings.NewReader(`{"type":"version","terraform":"1.9.0","ui":"1.2"}` + "\n")
	m := NewModel("comp", "stack", "plan", reader)

	cmd := m.Init()

	// Init should return a batch command.
	assert.NotNil(t, cmd)
}
