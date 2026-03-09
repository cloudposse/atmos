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
	"github.com/cloudposse/atmos/pkg/ai/skills"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// Key binding string constants.
const (
	keyEsc   = "esc"
	keyCtrlC = "ctrl+c"
	keyUp    = "up"
	keyDown  = "down"
	keyK     = "k"
	keyJ     = "j"
	keyQ     = "q"
	keyEnter = "enter"
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

// noopCmd returns a command that produces a nil message (consumes the key event).
func noopCmd() tea.Cmd {
	return func() tea.Msg { return nil }
}

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
		return m.handleLoadingKeys(msg)
	}

	// Handle view-specific keys first (before chat keys).
	switch m.currentView {
	case viewModeSessionList:
		return m.handleSessionListKeys(msg)
	case viewModeCreateSession:
		return m.handleCreateSessionKeys(msg)
	case viewModeProviderSelect:
		return m.handleProviderSelectKeys(msg)
	case viewModeSkillSelect:
		return m.handleSkillSelectKeys(msg)
	}

	return m.handleChatKeys(msg)
}

// handleLoadingKeys handles keys while the AI is processing.
func (m *ChatModel) handleLoadingKeys(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case keyCtrlC:
		return tea.Quit
	case keyEsc:
		// Cancel the ongoing AI request.
		if m.cancelFunc != nil && !m.isCancelling {
			m.isCancelling = true
			m.cancelFunc()
		}
		return noopCmd()
	}
	// Don't pass keys to textarea while loading.
	return noopCmd()
}

// handleChatKeys handles keyboard input in the chat view.
func (m *ChatModel) handleChatKeys(msg tea.KeyMsg) tea.Cmd {
	keyStr := msg.String()

	// Handle multiline input: Ctrl+J inserts newline (works in all terminals).
	if keyStr == "ctrl+j" {
		currentValue := m.textarea.Value()
		m.textarea.SetValue(currentValue + "\n")
		return noopCmd()
	}

	// Plain Enter: send message (only in chat view).
	if msg.Type == tea.KeyEnter {
		return m.handleChatEnter()
	}

	return m.handleChatControlKeys(keyStr)
}

// handleChatEnter handles the Enter key in chat view - sends message if non-empty.
func (m *ChatModel) handleChatEnter() tea.Cmd {
	value := stripANSI(m.textarea.Value())
	if value != "" && len(value) > 0 {
		return m.sendMessage(value)
	}
	// Don't send empty messages, but don't pass Enter to textarea either.
	return noopCmd()
}

// handleChatControlKeys handles control key combinations in chat view.
func (m *ChatModel) handleChatControlKeys(keyStr string) tea.Cmd {
	switch keyStr {
	case keyCtrlC:
		return tea.Quit
	case "ctrl+l":
		return m.handleCtrlL()
	case "ctrl+n":
		return m.handleCtrlN()
	case "ctrl+p":
		return m.handleCtrlP()
	case "ctrl+a":
		return m.handleCtrlA()
	case keyUp:
		return m.handleHistoryUp()
	case keyDown:
		return m.handleHistoryDown()
	}

	// Return nil to allow textarea to handle the key.
	return nil
}

// handleCtrlL opens the session list.
func (m *ChatModel) handleCtrlL() tea.Cmd {
	if m.manager == nil {
		m.addMessage(roleSystem, "Sessions are not enabled. Enable them in your atmos.yaml config: ai.sessions.enabled: true")
		return noopCmd()
	}
	return m.loadSessionList()
}

// handleCtrlN opens the create session form.
func (m *ChatModel) handleCtrlN() tea.Cmd {
	if m.manager == nil {
		m.addMessage(roleSystem, "Sessions are not enabled. Enable them in your atmos.yaml config: ai.sessions.enabled: true")
		return noopCmd()
	}
	m.currentView = viewModeCreateSession
	m.createForm = newCreateSessionForm()
	return noopCmd()
}

// handleCtrlP opens the provider selection view.
func (m *ChatModel) handleCtrlP() tea.Cmd {
	if m.atmosConfig == nil {
		return noopCmd()
	}

	m.currentView = viewModeProviderSelect
	m.selectedProviderIdx = 0

	// Find current provider index in configured providers.
	currentProvider := "anthropic"
	switch {
	case m.sess != nil && m.sess.Provider != "":
		currentProvider = m.sess.Provider
	case m.atmosConfig.AI.DefaultProvider != "":
		currentProvider = m.atmosConfig.AI.DefaultProvider
	}

	configuredProviders := m.getConfiguredProviders()
	for i, p := range configuredProviders {
		if p.Name == currentProvider {
			m.selectedProviderIdx = i
			break
		}
	}

	return noopCmd()
}

// handleCtrlA opens the skill selection view.
func (m *ChatModel) handleCtrlA() tea.Cmd {
	if m.skillRegistry == nil {
		return noopCmd()
	}

	m.currentView = viewModeSkillSelect
	m.selectedSkillIdx = 0

	// Find current skill index in available skills.
	availableSkills := m.skillRegistry.List()
	if m.currentSkill != nil {
		for i, skill := range availableSkills {
			if skill.Name == m.currentSkill.Name {
				m.selectedSkillIdx = i
				break
			}
		}
	}

	return noopCmd()
}

// handleHistoryUp navigates up in message history (single-line textarea only).
func (m *ChatModel) handleHistoryUp() tea.Cmd {
	if !strings.Contains(m.textarea.Value(), "\n") {
		m.navigateHistoryUp()
		return noopCmd()
	}
	// Let textarea handle up arrow for multiline navigation.
	return nil
}

// handleHistoryDown navigates down in message history (single-line textarea only).
func (m *ChatModel) handleHistoryDown() tea.Cmd {
	if !strings.Contains(m.textarea.Value(), "\n") {
		m.navigateHistoryDown()
		return noopCmd()
	}
	// Let textarea handle down arrow for multiline navigation.
	return nil
}

// handleSessionListKeys processes keyboard input for the session list view.
func (m *ChatModel) handleSessionListKeys(msg tea.KeyMsg) tea.Cmd {
	// Handle different modes.
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
		// Confirm deletion.
		return m.deleteSession(m.deleteSessionID)
	case "n", "N", keyEsc:
		// Cancel deletion.
		m.deleteConfirm = false
		m.deleteSessionID = ""
		return nil
	}
	return nil
}

// handleRenameKeys handles keyboard input during rename mode.
func (m *ChatModel) handleRenameKeys(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case keyEnter:
		// Submit rename.
		newName := strings.TrimSpace(m.renameInput.Value())
		if newName != "" {
			return m.renameSession(m.renameSessionID, newName)
		}
		// Empty name, cancel rename.
		m.renameMode = false
		m.renameSessionID = ""
		return nil
	case keyEsc:
		// Cancel rename.
		m.renameMode = false
		m.renameSessionID = ""
		return nil
	default:
		// Update text input.
		var cmd tea.Cmd
		m.renameInput, cmd = m.renameInput.Update(msg)
		return cmd
	}
}

// handleNormalSessionListKeys handles keyboard input during normal session list navigation.
func (m *ChatModel) handleNormalSessionListKeys(msg tea.KeyMsg) tea.Cmd {
	filteredSessions := m.getFilteredSessions()

	// Check for Enter key using Type (more reliable than String).
	if msg.Type == tea.KeyEnter {
		return m.handleSessionSelect(filteredSessions)
	}

	switch msg.String() {
	case keyCtrlC:
		return tea.Quit
	case keyEsc, keyQ:
		return m.returnToChat()
	case keyUp, keyK:
		m.navigateSessionList(filteredSessions, -1)
		return noopCmd()
	case keyDown, keyJ:
		m.navigateSessionList(filteredSessions, 1)
		return noopCmd()
	case "d", "D":
		m.initiateSessionDelete(filteredSessions)
		return noopCmd()
	case "r", "R":
		m.initiateSessionRename(filteredSessions)
		return noopCmd()
	case "f", "F":
		m.cycleFilter()
		return noopCmd()
	case "ctrl+n", "n":
		m.currentView = viewModeCreateSession
		m.createForm = newCreateSessionForm()
		return noopCmd()
	}

	return noopCmd()
}

// handleSessionSelect handles selecting a session from the filtered list.
func (m *ChatModel) handleSessionSelect(filteredSessions []*session.Session) tea.Cmd {
	if m.selectedSessionIndex < len(filteredSessions) {
		return m.switchSession(filteredSessions[m.selectedSessionIndex])
	}
	return nil
}

// returnToChat switches back to chat view.
func (m *ChatModel) returnToChat() tea.Cmd {
	m.currentView = viewModeChat
	m.textarea.Focus()
	return noopCmd()
}

// navigateSessionList moves the session selection up or down with wraparound.
func (m *ChatModel) navigateSessionList(sessions []*session.Session, direction int) {
	if len(sessions) == 0 {
		return
	}

	m.selectedSessionIndex += direction
	if m.selectedSessionIndex < 0 {
		m.selectedSessionIndex = len(sessions) - 1
	} else if m.selectedSessionIndex >= len(sessions) {
		m.selectedSessionIndex = 0
	}
}

// initiateSessionDelete starts the delete confirmation for the selected session.
func (m *ChatModel) initiateSessionDelete(filteredSessions []*session.Session) {
	if m.selectedSessionIndex < len(filteredSessions) {
		m.deleteConfirm = true
		m.deleteSessionID = filteredSessions[m.selectedSessionIndex].ID
	}
}

// initiateSessionRename starts the rename mode for the selected session.
func (m *ChatModel) initiateSessionRename(filteredSessions []*session.Session) {
	if m.selectedSessionIndex >= len(filteredSessions) {
		return
	}

	sess := filteredSessions[m.selectedSessionIndex]
	m.renameMode = true
	m.renameSessionID = sess.ID
	m.renameInput = textinput.New()
	m.renameInput.Placeholder = sessionNamePlaceholder
	m.renameInput.SetValue(sess.Name)
	m.renameInput.Focus()
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

	// First time navigating: save current input (strip ANSI codes).
	if m.historyIndex == -1 {
		m.historyBuffer = stripANSI(m.textarea.Value())
		m.historyIndex = len(m.messageHistory)
	}

	// Navigate backwards in history.
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

	// Navigate forwards in history.
	m.historyIndex++

	if m.historyIndex >= len(m.messageHistory) {
		// Reached the end: restore original input.
		m.textarea.SetValue(m.historyBuffer)
		m.historyIndex = -1
		m.historyBuffer = ""
	} else {
		m.textarea.SetValue(m.messageHistory[m.historyIndex])
	}
}

// handleProviderSelectKeys processes keyboard input for the provider selection view.
func (m *ChatModel) handleProviderSelectKeys(msg tea.KeyMsg) tea.Cmd {
	configuredProviders := m.getConfiguredProviders()

	switch msg.String() {
	case keyCtrlC:
		return tea.Quit
	case keyEsc, keyQ:
		return m.returnToChat()
	case keyUp, keyK:
		m.selectedProviderIdx = navigateListUp(m.selectedProviderIdx, len(configuredProviders))
		return noopCmd()
	case keyDown, keyJ:
		m.selectedProviderIdx = navigateListDown(m.selectedProviderIdx, len(configuredProviders))
		return noopCmd()
	case keyEnter:
		return m.selectProvider(configuredProviders)
	}

	return noopCmd()
}

// navigateListUp moves selection up with wraparound.
func navigateListUp(current, total int) int {
	if current > 0 {
		return current - 1
	}
	if total > 0 {
		return total - 1
	}
	return 0
}

// navigateListDown moves selection down with wraparound.
func navigateListDown(current, total int) int {
	if current < total-1 {
		return current + 1
	}
	if total > 0 {
		return 0
	}
	return 0
}

// selectProvider handles the Enter key in provider selection view.
func (m *ChatModel) selectProvider(configuredProviders []struct{ Name, Description string }) tea.Cmd {
	if m.selectedProviderIdx >= len(configuredProviders) {
		return m.returnToChat()
	}

	selectedProvider := configuredProviders[m.selectedProviderIdx].Name
	m.addMessage(roleSystem, fmt.Sprintf("Switching to %s...", selectedProvider))
	m.currentView = viewModeChat
	m.textarea.Focus()
	m.updateViewportContent()
	return m.switchProviderAsync(selectedProvider)
}

// handleSkillSelectKeys processes keyboard input for the skill selection view.
func (m *ChatModel) handleSkillSelectKeys(msg tea.KeyMsg) tea.Cmd {
	availableSkills := m.skillRegistry.List()

	switch msg.String() {
	case keyCtrlC:
		return tea.Quit
	case keyEsc, keyQ:
		return m.returnToChat()
	case keyUp, keyK:
		m.selectedSkillIdx = navigateListUp(m.selectedSkillIdx, len(availableSkills))
		return noopCmd()
	case keyDown, keyJ:
		m.selectedSkillIdx = navigateListDown(m.selectedSkillIdx, len(availableSkills))
		return noopCmd()
	case keyEnter:
		return m.selectSkill(availableSkills)
	}

	return noopCmd()
}

// selectSkill handles the Enter key in skill selection view.
func (m *ChatModel) selectSkill(availableSkills []*skills.Skill) tea.Cmd {
	if m.selectedSkillIdx >= len(availableSkills) {
		return m.returnToChat()
	}

	selectedSkill := availableSkills[m.selectedSkillIdx]
	m.activateSkill(selectedSkill)

	m.currentView = viewModeChat
	m.textarea.Focus()
	return noopCmd()
}

// activateSkill loads and activates the given skill.
func (m *ChatModel) activateSkill(skill *skills.Skill) {
	// Load skill's system prompt from file (if configured).
	systemPrompt, err := skill.LoadSystemPrompt()
	if err != nil {
		log.Warnf("Failed to load system prompt for skill %q: %v, using default", skill.Name, err)
	} else {
		skill.SystemPrompt = systemPrompt
	}

	m.currentSkill = skill

	// Persist skill to session if session management is enabled.
	if m.manager != nil && m.sess != nil {
		m.sess.Skill = skill.Name
		m.sess.UpdatedAt = time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if persistErr := m.manager.UpdateSession(ctx, m.sess); persistErr != nil {
			log.Debugf("Failed to persist skill to session: %v", persistErr)
		}
	}

	// Add feedback message.
	m.addMessage(roleSystem, fmt.Sprintf("Switched to skill: %s (%s)", skill.DisplayName, skill.Description))
	m.updateViewportContent()
}
