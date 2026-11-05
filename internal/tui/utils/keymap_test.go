package utils

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestNewAtmosKeyMap_QuitKeys(t *testing.T) {
	keyMap := NewAtmosKeyMap()

	tests := []struct {
		name string
		msg  tea.KeyMsg
		want bool
	}{
		{name: "ctrl+c", msg: tea.KeyMsg{Type: tea.KeyCtrlC}, want: true},
		{name: "esc", msg: tea.KeyMsg{Type: tea.KeyEsc}, want: true},
		{name: "ctrl+d", msg: tea.KeyMsg{Type: tea.KeyCtrlD}, want: true},
		{name: "enter submits, it does not quit", msg: tea.KeyMsg{Type: tea.KeyEnter}, want: false},
		{name: "a letter does not quit", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, key.Matches(tt.msg, keyMap.Quit))
		})
	}
}

func TestNewAtmosKeyMap_KeepsDefaultNavigation(t *testing.T) {
	keyMap := NewAtmosKeyMap()

	assert.True(t, key.Matches(tea.KeyMsg{Type: tea.KeyEnter}, keyMap.Input.Next), "enter still moves an input forward")
	assert.True(t, key.Matches(tea.KeyMsg{Type: tea.KeyUp}, keyMap.Select.Up), "arrow keys still move a selection")
}
