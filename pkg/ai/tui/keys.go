package tui

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/session"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// ansiEscapeRegex matches ANSI escape sequences and OSC (Operating System Command) sequences.
// This includes:
// - CSI sequences: ESC [ ... (e.g., colors, cursor movement, CPR).
// - Mouse tracking: [<digits>M (e.g., [<64;122;37M).
// - Bare CSI Cursor Position Report: [<row>;<col>R or partial fragments like <number>R or row;colR.
// - OSC sequences with BEL terminator: ESC ] ... BEL.
// - OSC sequences with ST terminator: ESC ] ... ESC \.
// - Bare OSC sequences: ] ... \ or rgb:... \ or <number>;rgb:... \ (fragments without ESC prefix).
// - Color query fragments: :0000/0000/0000\<letter> or bare [0-]0000/0000/0000[\\] (hex color responses, 1-4 digits per component).
// - Two-component color fragments: 000/0000\ (2 hex components with backslash terminator).
// - Standalone escape terminators: space-backslash " \" or bare "\" at start of line.
var ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\[<[0-9;]+M|\[[0-9;]+R|\d+;\d+R|^\d+R|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|][^\\]*\\|\d*;?rgb:[0-9a-fA-F/]*\\|:[0-9a-fA-F/]+\\[a-zA-Z]?|[0-9a-fA-F]{1,4}/[0-9a-fA-F]{1,4}/[0-9a-fA-F]{1,4}\\?|[0-9a-fA-F]{1,4}/[0-9a-fA-F]{1,4}\\|^\s*\\$`)

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	return ansiEscapeRegex.ReplaceAllString(s, "")
}

// handleKeyMsg processes keyboard input and returns a command if the key was handled.
// Returns nil if the key should be passed to the textarea.
//
//revive:disable:cyclomatic // TUI keyboard handlers naturally have high complexity.
func (m *ChatModel) handleKeyMsg(msg tea.KeyMsg) tea.Cmd {
	if m.isLoading {
		// Allow quitting or cancelling while loading.
		switch msg.String() {
		case "ctrl+c":
			return tea.Quit
		case "esc":
			// Cancel the ongoing AI request.
			if m.cancelFunc != nil && !m.isCancelling {
				m.isCancelling = true
				m.cancelFunc()
			}
			return func() tea.Msg { return nil }
		}
		// Don't pass keys to textarea while loading.
		return func() tea.Msg { return nil }
	}

	// Handle view-specific keys first (before chat keys).
	// This ensures Enter works correctly in session list, create session, and provider select views.

	if m.currentView == viewModeSessionList {
		return m.handleSessionListKeys(msg)
	}

	if m.currentView == viewModeCreateSession {
		return m.handleCreateSessionKeys(msg)
	}

	if m.currentView == viewModeProviderSelect {
		return m.handleProviderSelectKeys(msg)
	}

	if m.currentView == viewModeAgentSelect {
		return m.handleAgentSelectKeys(msg)
	}

	// Handle chat view Enter key variants.
	keyStr := msg.String()

	// Handle multiline input: Ctrl+J inserts newline (works in all terminals).
	// Note: Most terminals don't send distinct codes for Shift+Enter, so we use Ctrl+J.
	if keyStr == "ctrl+j" {
		// Ctrl+J: insert newline for multi-line messages.
		currentValue := m.textarea.Value()
		m.textarea.SetValue(currentValue + "\n")
		return func() tea.Msg { return nil }
	}

	// Plain Enter: send message (only in chat view).
	if msg.Type == tea.KeyEnter {
		value := stripANSI(m.textarea.Value()) // Strip any ANSI escape sequences
		if value != "" && len(value) > 0 {
			return m.sendMessage(value)
		}
		// Don't send empty messages, but don't pass Enter to textarea either.
		return func() tea.Msg { return nil }
	}

	// Handle chat view keys.
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "ctrl+l":
		// Open session list.
		if m.manager == nil {
			m.addMessage(roleSystem, "Sessions are not enabled. Enable them in your atmos.yaml config: settings.ai.sessions.enabled: true")
			return func() tea.Msg { return nil }
		}
		return m.loadSessionList()
	case "ctrl+n":
		// Open create session form.
		if m.manager == nil {
			m.addMessage(roleSystem, "Sessions are not enabled. Enable them in your atmos.yaml config: settings.ai.sessions.enabled: true")
			return func() tea.Msg { return nil }
		}
		m.currentView = viewModeCreateSession
		m.createForm = newCreateSessionForm() // Reset form
		return func() tea.Msg { return nil }
	case "ctrl+p":
		// Open provider selection.
		if m.atmosConfig != nil {
			m.currentView = viewModeProviderSelect
			m.selectedProviderIdx = 0
			// Find current provider index in configured providers.
			currentProvider := ""
			if m.sess != nil && m.sess.Provider != "" {
				currentProvider = m.sess.Provider
			} else if m.atmosConfig.Settings.AI.DefaultProvider != "" {
				currentProvider = m.atmosConfig.Settings.AI.DefaultProvider
			} else {
				currentProvider = "anthropic"
			}
			// Use configured providers only
			configuredProviders := m.getConfiguredProviders()
			for i, p := range configuredProviders {
				if p.Name == currentProvider {
					m.selectedProviderIdx = i
					break
				}
			}
		}
		return func() tea.Msg { return nil }
	case "ctrl+a":
		// Open agent selection.
		if m.agentRegistry != nil {
			m.currentView = viewModeAgentSelect
			m.selectedAgentIdx = 0
			// Find current agent index in available agents.
			availableAgents := m.agentRegistry.List()
			if m.currentAgent != nil {
				for i, agent := range availableAgents {
					if agent.Name == m.currentAgent.Name {
						m.selectedAgentIdx = i
						break
					}
				}
			}
		}
		return func() tea.Msg { return nil }
	case "up":
		// Only use up arrow for history if textarea is single-line.
		// For multiline text, let the textarea handle cursor movement.
		if !strings.Contains(m.textarea.Value(), "\n") {
			m.navigateHistoryUp()
			return func() tea.Msg { return nil }
		}
		// Let textarea handle up arrow for multiline navigation.
		return nil
	case "down":
		// Only use down arrow for history if textarea is single-line.
		// For multiline text, let the textarea handle cursor movement.
		if !strings.Contains(m.textarea.Value(), "\n") {
			m.navigateHistoryDown()
			return func() tea.Msg { return nil }
		}
		// Let textarea handle down arrow for multiline navigation.
		return nil
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

	// Check for Enter key using Type (more reliable than String).
	if msg.Type == tea.KeyEnter {
		// Select session from filtered list.
		if m.selectedSessionIndex < len(filteredSessions) {
			return m.switchSession(filteredSessions[m.selectedSessionIndex])
		}
		return nil
	}

	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc", "q":
		// Return to chat view.
		m.currentView = viewModeChat
		m.textarea.Focus()
		// Return empty command to consume the key event and prevent it from reaching the textarea
		return func() tea.Msg { return nil }
	case "up", "k":
		// Navigate up in filtered list with wraparound.
		if m.selectedSessionIndex > 0 {
			m.selectedSessionIndex--
		} else if len(filteredSessions) > 0 {
			m.selectedSessionIndex = len(filteredSessions) - 1
		}
		return func() tea.Msg { return nil }
	case "down", "j":
		// Navigate down in filtered list with wraparound.
		if m.selectedSessionIndex < len(filteredSessions)-1 {
			m.selectedSessionIndex++
		} else if len(filteredSessions) > 0 {
			m.selectedSessionIndex = 0
		}
		return func() tea.Msg { return nil }
	case "d", "D":
		// Delete session from filtered list.
		if m.selectedSessionIndex < len(filteredSessions) {
			m.deleteConfirm = true
			m.deleteSessionID = filteredSessions[m.selectedSessionIndex].ID
		}
		return func() tea.Msg { return nil }
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
		return func() tea.Msg { return nil }
	case "f", "F":
		// Cycle through provider filters.
		m.cycleFilter()
		return func() tea.Msg { return nil }
	case "ctrl+n", "n":
		// Open create session form.
		m.currentView = viewModeCreateSession
		m.createForm = newCreateSessionForm() // Reset form
		return func() tea.Msg { return nil }
	}

	return func() tea.Msg { return nil }
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

	// First time navigating: save current input (strip ANSI codes)
	if m.historyIndex == -1 {
		m.historyBuffer = stripANSI(m.textarea.Value())
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
	// Get configured providers
	configuredProviders := m.getConfiguredProviders()

	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc", "q":
		// Return to chat view.
		m.currentView = viewModeChat
		m.textarea.Focus()
		// Return empty command to consume the key event
		return func() tea.Msg { return nil }
	case "up", "k":
		// Move selection up with wraparound.
		if m.selectedProviderIdx > 0 {
			m.selectedProviderIdx--
		} else if len(configuredProviders) > 0 {
			m.selectedProviderIdx = len(configuredProviders) - 1
		}
		return func() tea.Msg { return nil }
	case "down", "j":
		// Move selection down with wraparound.
		if m.selectedProviderIdx < len(configuredProviders)-1 {
			m.selectedProviderIdx++
		} else if len(configuredProviders) > 0 {
			m.selectedProviderIdx = 0
		}
		return func() tea.Msg { return nil }
	case "enter":
		// Switch to selected provider asynchronously.
		if m.selectedProviderIdx < len(configuredProviders) {
			selectedProvider := configuredProviders[m.selectedProviderIdx].Name
			// Add immediate feedback that switch is starting.
			m.addMessage(roleSystem, fmt.Sprintf("Switching to %s...", selectedProvider))
			// Return to chat view immediately and start async switch.
			m.currentView = viewModeChat
			m.textarea.Focus()
			m.updateViewportContent()
			// Initiate async provider switch (will send providerSwitchedMsg when done).
			return m.switchProviderAsync(selectedProvider)
		}
		// Return to chat view if no provider selected.
		m.currentView = viewModeChat
		m.textarea.Focus()
		return func() tea.Msg { return nil }
	}

	return func() tea.Msg { return nil }
}

// handleAgentSelectKeys processes keyboard input for the agent selection view.
func (m *ChatModel) handleAgentSelectKeys(msg tea.KeyMsg) tea.Cmd {
	// Get available agents.
	availableAgents := m.agentRegistry.List()

	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc", "q":
		// Return to chat view.
		m.currentView = viewModeChat
		m.textarea.Focus()
		// Return empty command to consume the key event.
		return func() tea.Msg { return nil }
	case "up", "k":
		// Move selection up with wraparound.
		if m.selectedAgentIdx > 0 {
			m.selectedAgentIdx--
		} else if len(availableAgents) > 0 {
			m.selectedAgentIdx = len(availableAgents) - 1
		}
		return func() tea.Msg { return nil }
	case "down", "j":
		// Move selection down with wraparound.
		if m.selectedAgentIdx < len(availableAgents)-1 {
			m.selectedAgentIdx++
		} else if len(availableAgents) > 0 {
			m.selectedAgentIdx = 0
		}
		return func() tea.Msg { return nil }
	case "enter":
		// Switch to selected agent.
		if m.selectedAgentIdx < len(availableAgents) {
			selectedAgent := availableAgents[m.selectedAgentIdx]

			// Load agent's system prompt from file (if configured).
			systemPrompt, err := selectedAgent.LoadSystemPrompt()
			if err != nil {
				log.Warn(fmt.Sprintf("Failed to load system prompt for agent %q: %v, using default", selectedAgent.Name, err))
				// Keep the existing SystemPrompt as fallback.
			} else {
				// Update agent with loaded prompt.
				selectedAgent.SystemPrompt = systemPrompt
			}

			m.currentAgent = selectedAgent

			// Persist agent to session if session management is enabled.
			if m.manager != nil && m.sess != nil {
				m.sess.Agent = selectedAgent.Name
				m.sess.UpdatedAt = time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := m.manager.UpdateSession(ctx, m.sess); err != nil {
					log.Debug(fmt.Sprintf("Failed to persist agent to session: %v", err))
				}
			}

			// Add feedback message.
			m.addMessage(roleSystem, fmt.Sprintf("Switched to agent: %s (%s)", selectedAgent.DisplayName, selectedAgent.Description))
			m.updateViewportContent()
		}
		// Return to chat view.
		m.currentView = viewModeChat
		m.textarea.Focus()
		return func() tea.Msg { return nil }
	}

	return func() tea.Msg { return nil }
}
