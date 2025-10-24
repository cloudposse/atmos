package tui

import (
	"context"
	"fmt"
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

	// Handle provider select view keys.
	if m.currentView == viewModeProviderSelect {
		return m.handleProviderSelectKeys(msg)
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
	case "ctrl+p":
		// Open provider selection.
		if m.atmosConfig != nil {
			m.currentView = viewModeProviderSelect
			m.selectedProviderIdx = 0
			// Find current provider index.
			currentProvider := ""
			if m.sess != nil && m.sess.Provider != "" {
				currentProvider = m.sess.Provider
			} else if m.atmosConfig.Settings.AI.DefaultProvider != "" {
				currentProvider = m.atmosConfig.Settings.AI.DefaultProvider
			} else {
				currentProvider = "anthropic"
			}
			for i, p := range availableProviders {
				if p.Name == currentProvider {
					m.selectedProviderIdx = i
					break
				}
			}
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
	case "up":
		// Navigate to previous message in history.
		m.navigateHistoryUp()
		return func() tea.Msg { return nil }
	case "down":
		// Navigate to next message in history.
		m.navigateHistoryDown()
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
//
//nolint:funlen // TUI keyboard handlers require comprehensive switch cases.
func (m *ChatModel) handleNormalSessionListKeys(msg tea.KeyMsg) tea.Cmd {
	// Get filtered sessions for navigation
	filteredSessions := m.getFilteredSessions()

	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc", "q":
		// Return to chat view.
		m.currentView = viewModeChat
		return nil
	case "up", "k":
		// Navigate up in filtered list.
		if m.selectedSessionIndex > 0 {
			m.selectedSessionIndex--
		}
		return nil
	case "down", "j":
		// Navigate down in filtered list.
		if m.selectedSessionIndex < len(filteredSessions)-1 {
			m.selectedSessionIndex++
		}
		return nil
	case "enter":
		// Select session from filtered list.
		if m.selectedSessionIndex < len(filteredSessions) {
			return m.switchSession(filteredSessions[m.selectedSessionIndex])
		}
		return nil
	case "d", "D":
		// Delete session from filtered list.
		if m.selectedSessionIndex < len(filteredSessions) {
			m.deleteConfirm = true
			m.deleteSessionID = filteredSessions[m.selectedSessionIndex].ID
		}
		return nil
	case "r", "R":
		// Rename session from filtered list.
		if m.selectedSessionIndex < len(filteredSessions) {
			sess := filteredSessions[m.selectedSessionIndex]
			m.renameMode = true
			m.renameSessionID = sess.ID
			// Initialize rename input with current name
			m.renameInput = textinput.New()
			m.renameInput.Placeholder = sessionNamePlaceholder
			m.renameInput.SetValue(sess.Name)
			m.renameInput.Focus()
		}
		return nil
	case "f", "F":
		// Cycle through provider filters.
		m.cycleFilter()
		return nil
	case "ctrl+n", "n":
		// Open create session form.
		m.currentView = viewModeCreateSession
		m.createForm = newCreateSessionForm() // Reset form
		return nil
	}

	return nil
}

// getFilteredSessions returns sessions filtered by the current provider filter.
func (m *ChatModel) getFilteredSessions() []*session.Session {
	if m.sessionFilter == filterAll {
		return m.availableSessions
	}

	filtered := make([]*session.Session, 0)
	for _, sess := range m.availableSessions {
		if sess.Provider == m.sessionFilter {
			filtered = append(filtered, sess)
		}
	}
	return filtered
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

// navigateHistoryUp navigates to the previous message in history.
func (m *ChatModel) navigateHistoryUp() {
	if len(m.messageHistory) == 0 {
		return
	}

	// First time navigating: save current input
	if m.historyIndex == -1 {
		m.historyBuffer = m.textarea.Value()
		m.historyIndex = len(m.messageHistory)
	}

	// Navigate backwards in history
	if m.historyIndex > 0 {
		m.historyIndex--
		m.textarea.SetValue(m.messageHistory[m.historyIndex])
	}
}

// navigateHistoryDown navigates to the next message in history.
func (m *ChatModel) navigateHistoryDown() {
	if len(m.messageHistory) == 0 || m.historyIndex == -1 {
		return
	}

	// Navigate forwards in history
	m.historyIndex++

	if m.historyIndex >= len(m.messageHistory) {
		// Reached the end: restore original input
		m.textarea.SetValue(m.historyBuffer)
		m.historyIndex = -1
		m.historyBuffer = ""
	} else {
		m.textarea.SetValue(m.messageHistory[m.historyIndex])
	}
}

// handleProviderSelectKeys processes keyboard input for the provider selection view.
func (m *ChatModel) handleProviderSelectKeys(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc", "q":
		// Return to chat view.
		m.currentView = viewModeChat
		return nil
	case "up", "k":
		// Move selection up.
		if m.selectedProviderIdx > 0 {
			m.selectedProviderIdx--
		}
		return nil
	case "down", "j":
		// Move selection down.
		if m.selectedProviderIdx < len(availableProviders)-1 {
			m.selectedProviderIdx++
		}
		return nil
	case "enter":
		// Switch to selected provider.
		selectedProvider := availableProviders[m.selectedProviderIdx].Name
		if err := m.switchProvider(selectedProvider); err != nil {
			m.addMessage(roleSystem, fmt.Sprintf("Error switching provider: %v", err))
		}
		// Return to chat view.
		m.currentView = viewModeChat
		m.updateViewportContent()
		return nil
	}

	return nil
}
