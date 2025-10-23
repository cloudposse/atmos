package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/session"
)

// handleKeyMsg processes keyboard input and returns a command if the key was handled.
// Returns nil if the key should be passed to the textarea.
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

	// Handle chat view keys.
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "ctrl+l":
		// Open session list.
		return m.loadSessionList()
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
	case "ctrl+n":
		// Create new session (placeholder for now).
		m.sessionListError = "Create new session: not yet implemented"
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
