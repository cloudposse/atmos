package output

import (
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

type blockingWriter struct {
	started chan struct{}
	release <-chan struct{}
}

func (w blockingWriter) Write(p []byte) (int, error) {
	close(w.started)
	<-w.release
	return len(p), nil
}

func TestNewSpinner(t *testing.T) {
	// Test that NewSpinner returns a valid tea.Program.
	message := "Loading..."
	p := NewSpinner(message)
	require.NotNil(t, p)
}

func TestSuppressSpinnersRestoresNestedScopes(t *testing.T) {
	restoreOuter := SuppressSpinners()
	t.Cleanup(restoreOuter)
	require.True(t, spinnersSuppressed())

	restoreInner := SuppressSpinners()
	t.Cleanup(restoreInner)
	require.True(t, spinnersSuppressed())

	restoreOuter()
	restoreOuter()
	require.True(t, spinnersSuppressed())

	restoreInner()
	require.False(t, spinnersSuppressed())
}

func TestClearSpinnerLineBlocksConcurrentSuppression(t *testing.T) {
	clearStarted := make(chan struct{})
	releaseClear := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseClear) }) }
	t.Cleanup(release)

	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.Reset)
	restoreUI := iolib.PushUIWriter(blockingWriter{started: clearStarted, release: releaseClear})
	t.Cleanup(restoreUI)

	clearDone := make(chan struct{})
	go func() {
		clearSpinnerLine()
		close(clearDone)
	}()
	select {
	case <-clearStarted:
	case <-time.After(time.Second):
		t.Fatal("ClearLine did not start")
	}

	suppressionDone := make(chan struct{})
	go func() {
		restore := SuppressSpinners()
		restore()
		close(suppressionDone)
	}()
	select {
	case <-suppressionDone:
		t.Fatal("Suppression interleaved with ClearLine")
	case <-time.After(100 * time.Millisecond):
	}

	release()
	select {
	case <-clearDone:
	case <-time.After(time.Second):
		t.Fatal("ClearLine did not finish")
	}
	select {
	case <-suppressionDone:
	case <-time.After(time.Second):
		t.Fatal("Suppression did not finish")
	}
}

func TestModelSpinner_Init(t *testing.T) {
	s := spinner.New()
	model := modelSpinner{
		spinner: s,
		message: "Test message",
	}

	cmd := model.Init()
	// Init should return a tick command for the spinner.
	assert.NotNil(t, cmd)
}

func TestModelSpinner_Update_KeyMsg(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		wantQuit bool
	}{
		{
			name:     "ctrl+c quits",
			key:      "ctrl+c",
			wantQuit: true,
		},
		{
			name:     "q quits",
			key:      "q",
			wantQuit: true,
		},
		{
			name:     "other key does not quit",
			key:      "a",
			wantQuit: false,
		},
		{
			name:     "enter does not quit",
			key:      "enter",
			wantQuit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := spinner.New()
			model := modelSpinner{
				spinner: s,
				message: "Test",
			}

			var keyMsg tea.KeyMsg
			switch tt.key {
			case "ctrl+c":
				keyMsg = tea.KeyMsg{Type: tea.KeyCtrlC}
			case "q":
				keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
			case "enter":
				keyMsg = tea.KeyMsg{Type: tea.KeyEnter}
			default:
				keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)}
			}

			newModel, cmd := model.Update(keyMsg)
			assert.NotNil(t, newModel)

			if tt.wantQuit {
				// Verify it's the quit command by executing and comparing result.
				require.NotNil(t, cmd)
				assert.Equal(t, tea.Quit(), cmd())
			} else {
				// Non-quit keys shouldn't return a command.
				assert.Nil(t, cmd)
			}
		})
	}
}

func TestModelSpinner_Update_UnknownMsg(t *testing.T) {
	s := spinner.New()
	model := modelSpinner{
		spinner: s,
		message: "Test",
	}

	// Send an unknown message type.
	type unknownMsg struct{}
	newModel, cmd := model.Update(unknownMsg{})

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
}

func TestModelSpinner_Update_TickMsg(t *testing.T) {
	s := spinner.New()
	model := modelSpinner{
		spinner: s,
		message: "Test",
	}

	// Get the tick command from the spinner and execute it to get the TickMsg.
	tickCmd := s.Tick
	msg := tickCmd()
	newModel, cmd := model.Update(msg)

	assert.NotNil(t, newModel)
	// Tick should return the next tick command for continuous animation.
	assert.NotNil(t, cmd)
}

func TestModelSpinner_View(t *testing.T) {
	s := spinner.New()
	model := modelSpinner{
		spinner: s,
		message: "Loading data...",
	}

	view := model.View()
	// View should contain the message.
	assert.Contains(t, view, "Loading data...")
}
