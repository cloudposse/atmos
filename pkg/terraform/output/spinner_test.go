package output

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSpinner(t *testing.T) {
	// Test that NewSpinner returns a valid tea.Program.
	message := "Loading..."
	p := NewSpinner(message)
	require.NotNil(t, p)
}

func TestModelSpinner_Init(t *testing.T) {
	model := modelSpinner{
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
			model := modelSpinner{
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
				// cmd should be tea.Quit.
				assert.NotNil(t, cmd)
			}
		})
	}
}

func TestModelSpinner_Update_UnknownMsg(t *testing.T) {
	model := modelSpinner{
		message: "Test",
	}

	// Send an unknown message type.
	type unknownMsg struct{}
	newModel, cmd := model.Update(unknownMsg{})

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
}

func TestModelSpinner_View(t *testing.T) {
	model := modelSpinner{
		message: "Loading data...",
	}

	view := model.View()
	// View should contain the message.
	assert.Contains(t, view, "Loading data...")
}
