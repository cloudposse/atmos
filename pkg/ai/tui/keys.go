package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/session"
)

// handleKeyMsg processes keyboard input and returns a command if the key was handled.
// Returns nil if the key should be passed to the textarea.
//
//revive:disable:cyclomatic // TUI keyboard handlers naturally have high complexity.
func (m *ChatModel) handleKeyMsg(msg tea.KeyMsg) tea.Cmd {
	if m.isLoading {
		// Only allow quitting while loading.
		if msg.String() == "ctrl+c" {
			return tea.Quit
		}
		// Don't pass keys to textarea while loading.
		return func() tea.Msg { return nil }
	}

	// Handle session list view keys.
	if m.currentView == viewModeSessionList {
		return m.handleSessionListKeys(msg)
	}

	// Handle create session view keys.
	if m.currentView == viewModeCreateSession {
		return m.handleCreateSessionKeys(msg)
	}

	// Handle chat view keys.
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "ctrl+l":
		// Open session list.
		return m.loadSessionList()
	case "ctrl+n":
		// Open create session form.
		if m.manager != nil {
			m.currentView = viewModeCreateSession
			m.createForm = newCreateSessionForm() // Reset form
		}
		return nil
	case "shift+enter", "alt+enter":
		// Shift+Enter or Alt+Enter: let textarea handle it (adds newline).
		return nil
	case "enter":
		// Plain Enter: send message.
		if m.textarea.Value() != "" && len(m.textarea.Value()) > 0 {
			return m.sendMessage(m.textarea.Value())
		}
		// Don't send empty messages, but don't pass Enter to textarea either.
		return func() tea.Msg { return nil }
	}

	// Return nil to allow textarea to handle the key.
	return nil
}

// handleSessionListKeys processes keyboard input for the session list view.
func (m *ChatModel) handleSessionListKeys(msg tea.KeyMsg) tea.Cmd {
	// Handle different modes
	if m.deleteConfirm {
		return m.handleDeleteConfirmationKeys(msg)
	}

	if m.renameMode {
		return m.handleRenameKeys(msg)
	}

	return m.handleNormalSessionListKeys(msg)
}

// handleDeleteConfirmationKeys handles keyboard input during delete confirmation.
func (m *ChatModel) handleDeleteConfirmationKeys(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "y", "Y":
		// Confirm deletion
		return m.deleteSession(m.deleteSessionID)
	case "n", "N", "esc":
		// Cancel deletion
		m.deleteConfirm = false
		m.deleteSessionID = ""
		return nil
	}
	return nil
}

// handleRenameKeys handles keyboard input during rename mode.
func (m *ChatModel) handleRenameKeys(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		// Submit rename
		newName := strings.TrimSpace(m.renameInput.Value())
		if newName != "" {
			return m.renameSession(m.renameSessionID, newName)
		}
		// Empty name, cancel rename
		m.renameMode = false
		m.renameSessionID = ""
		return nil
	case "esc":
		// Cancel rename
		m.renameMode = false
		m.renameSessionID = ""
		return nil
	default:
		// Update text input
		var cmd tea.Cmd
		m.renameInput, cmd = m.renameInput.Update(msg)
		return cmd
	}
}

// handleNormalSessionListKeys handles keyboard input during normal session list navigation.
func (m *ChatModel) handleNormalSessionListKeys(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc", "q":
		// Return to chat view.
		m.currentView = viewModeChat
		return nil
	case "up", "k":
		// Navigate up.
		if m.selectedSessionIndex > 0 {
			m.selectedSessionIndex--
		}
		return nil
	case "down", "j":
		// Navigate down.
		if m.selectedSessionIndex < len(m.availableSessions)-1 {
			m.selectedSessionIndex++
		}
		return nil
	case "enter":
		// Select session.
		if m.selectedSessionIndex < len(m.availableSessions) {
			return m.switchSession(m.availableSessions[m.selectedSessionIndex])
		}
		return nil
	case "d", "D":
		// Delete session - enter confirmation mode.
		if m.selectedSessionIndex < len(m.availableSessions) {
			m.deleteConfirm = true
			m.deleteSessionID = m.availableSessions[m.selectedSessionIndex].ID
		}
		return nil
	case "r", "R":
		// Rename session - enter rename mode.
		if m.selectedSessionIndex < len(m.availableSessions) {
			sess := m.availableSessions[m.selectedSessionIndex]
			m.renameMode = true
			m.renameSessionID = sess.ID
			// Initialize rename input with current name
			m.renameInput = textinput.New()
			m.renameInput.Placeholder = "Enter new session name"
			m.renameInput.SetValue(sess.Name)
			m.renameInput.Focus()
		}
		return nil
	case "ctrl+n", "n":
		// Open create session form.
		m.currentView = viewModeCreateSession
		m.createForm = newCreateSessionForm() // Reset form
		return nil
	}

	return nil
}

// loadSessionList loads the list of available sessions.
func (m *ChatModel) loadSessionList() tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return sessionListLoadedMsg{err: errUtils.ErrAISessionManagerNotAvailable}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		sessions, err := m.manager.ListSessions(ctx)
		if err != nil {
			return sessionListLoadedMsg{err: err}
		}

		return sessionListLoadedMsg{sessions: sessions}
	}
}

// switchSession switches to a different session.
func (m *ChatModel) switchSession(sess *session.Session) tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return sessionSwitchedMsg{err: errUtils.ErrAISessionManagerNotAvailable}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Load messages for the new session.
		messages, err := m.manager.GetMessages(ctx, sess.ID, 0)
		if err != nil {
			return sessionSwitchedMsg{err: err}
		}

		return sessionSwitchedMsg{
			session:  sess,
			messages: messages,
		}
	}
}
