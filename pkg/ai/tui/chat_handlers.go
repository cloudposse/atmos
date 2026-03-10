package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// Init initializes the chat model.
func (m *ChatModel) Init() tea.Cmd {
	// Clean any ANSI escape sequences that may have leaked into textarea during initialization.
	// This handles OSC sequences like ]11;rgb:0000/0000/0000 that terminals send on startup.
	if m.textarea.Value() != "" {
		m.textarea.Reset()
	}

	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
	)
}

// Update handles messages and updates the model state.
func (m *ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	// Handle different message types.
	if handled, returnCmd := m.handleMessage(msg, &cmds); handled {
		if returnCmd != nil {
			return m, returnCmd
		}
	}

	// Update textarea only if not loading and in chat mode.
	if !m.isLoading && m.currentView == viewModeChat {
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)

		// Strip any ANSI escape sequences that might be pasted.
		// Terminal emulators can send OSC sequences as input.
		if currentValue := m.textarea.Value(); currentValue != "" {
			cleanedValue := stripANSI(currentValue)
			if cleanedValue != currentValue {
				m.textarea.SetValue(cleanedValue)
			}
		}
	}

	// Update viewport.
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// handleMessage processes different message types and returns whether it was handled.
//
//revive:disable:cyclomatic // Message handling naturally requires branching for different message types.
func (m *ChatModel) handleMessage(msg tea.Msg, cmds *[]tea.Cmd) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.handleWindowResize(msg)
		return true, nil

	case tea.KeyMsg:
		return m.handleKeyMessage(msg)

	case tea.MouseMsg:
		return m.handleMouseMessage(msg)

	case sendMessageMsg:
		return m.handleSendMessage(msg)

	case aiResponseMsg, aiErrorMsg:
		return m.handleAIMessage(msg), nil

	case compactStatusMsg:
		return m.handleCompactStatus(msg), nil

	case spinner.TickMsg:
		return m.handleSpinnerTick(msg, cmds), nil

	case sessionListLoadedMsg:
		m.handleSessionListLoaded(msg)
		return true, nil

	case sessionSwitchedMsg:
		m.handleSessionSwitched(msg)
		return true, nil

	case sessionCreatedMsg:
		m.handleSessionCreated(msg)
		return true, nil

	case sessionDeletedMsg:
		return true, m.handleSessionDeleted(msg)

	case sessionRenamedMsg:
		return true, m.handleSessionRenamed(msg)

	case providerSwitchedMsg:
		m.handleProviderSwitched(msg)
		return true, nil

	case statusMsg:
		m.loadingText = string(msg)
		return true, nil
	}

	return false, nil
}

// handleKeyMessage handles keyboard input.
func (m *ChatModel) handleKeyMessage(msg tea.KeyMsg) (bool, tea.Cmd) {
	if keyCmd := m.handleKeyMsg(msg); keyCmd != nil {
		return true, keyCmd
	}
	// Fall through to update textarea with the key.
	return false, nil
}

// handleMouseMessage handles mouse input.
func (m *ChatModel) handleMouseMessage(msg tea.MouseMsg) (bool, tea.Cmd) {
	// Only handle left button presses.
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return false, nil
	}

	// Only handle mouse clicks in create session view.
	if m.currentView != viewModeCreateSession {
		return false, nil
	}

	// Simple heuristic: clicks in upper area (rows 0-6) focus name field,
	// clicks in lower area (rows 7+) focus provider field.
	m.focusCreateFormField(msg.Y <= createFormNameFieldMaxRow)
	return true, nil
}

// focusCreateFormField focuses the name or provider field in the create session form.
func (m *ChatModel) focusCreateFormField(focusName bool) {
	if focusName {
		if m.createForm.focusedField != 0 {
			m.createForm.focusedField = 0
			m.createForm.nameInput.Focus()
		}
		return
	}

	if m.createForm.focusedField != 1 {
		m.createForm.focusedField = 1
		m.createForm.nameInput.Blur()
	}
}

// handleSendMessage handles user message sending.
func (m *ChatModel) handleSendMessage(msg sendMessageMsg) (bool, tea.Cmd) {
	m.addMessage(roleUser, string(msg))
	// Add to message history for navigation.
	m.messageHistory = append(m.messageHistory, string(msg))
	m.historyIndex = -1  // Reset history navigation.
	m.historyBuffer = "" // Clear history buffer.
	m.textarea.Reset()
	m.isLoading = true
	m.loadingText = "AI is thinking..."
	m.isCancelling = false

	// Create cancellable context for this AI request.
	// Use 5-minute timeout to allow for complex operations with multiple tool executions.
	// User can still cancel manually with Esc key if needed.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	m.cancelFunc = cancel

	m.updateViewportContent()
	return true, tea.Batch(
		m.spinner.Tick,
		m.getAIResponseWithContext(string(msg), ctx),
	)
}

// handleAIMessage handles AI response and error messages.
func (m *ChatModel) handleAIMessage(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case aiResponseMsg:
		m.handleAIResponseMsg(msg)
	case aiErrorMsg:
		m.handleAIErrorMsg(msg)
	}
	m.isLoading = false
	m.isCancelling = false

	// Clean up cancel function.
	if m.cancelFunc != nil {
		m.cancelFunc()
		m.cancelFunc = nil
	}

	m.updateViewportContent()

	// Focus the textarea so the user can immediately type their next message.
	m.textarea.Focus()

	return true
}

// handleAIResponseMsg processes a successful AI response.
func (m *ChatModel) handleAIResponseMsg(msg aiResponseMsg) {
	// Track usage if available.
	if msg.usage != nil {
		m.lastUsage = msg.usage
		// Accumulate into cumulative usage.
		m.cumulativeUsage.InputTokens += msg.usage.InputTokens
		m.cumulativeUsage.OutputTokens += msg.usage.OutputTokens
		m.cumulativeUsage.TotalTokens += msg.usage.TotalTokens
		m.cumulativeUsage.CacheReadTokens += msg.usage.CacheReadTokens
		m.cumulativeUsage.CacheCreationTokens += msg.usage.CacheCreationTokens
	}

	// Add message with usage information if available.
	content := msg.content
	if msg.usage != nil && msg.usage.TotalTokens > 0 {
		usageInfo := formatUsage(msg.usage)
		if usageInfo != "" {
			content = fmt.Sprintf("%s\n\n---\n*Token usage: %s*", content, usageInfo)
		}
	}
	m.addMessage(roleAssistant, content)
}

// handleAIErrorMsg processes an AI error message.
func (m *ChatModel) handleAIErrorMsg(msg aiErrorMsg) {
	errorMsg := string(msg)
	// Don't show "Request cancelled" if user cancelled.
	if !m.isCancelling || errorMsg != "Request cancelled" {
		m.addMessage(roleSystem, fmt.Sprintf("Error: %s", errorMsg))
	} else if m.isCancelling {
		m.addMessage(roleSystem, "Request cancelled by user")
	}
}

// handleCompactStatus handles conversation compaction status updates.
func (m *ChatModel) handleCompactStatus(msg compactStatusMsg) bool {
	switch msg.stage {
	case "starting":
		// Show compacting message with spinner.
		m.isLoading = true
		compactMsg := fmt.Sprintf("Compacting conversation (%d messages)...", msg.messageCount)
		m.addMessage(roleSystem, compactMsg)

	case "completed":
		// Show completion message.
		m.isLoading = false
		successMsg := fmt.Sprintf("Conversation compacted successfully (%d messages summarized, ~%d tokens saved)",
			msg.messageCount, msg.savings)
		m.addMessage(roleSystem, successMsg)

	case "failed":
		// Show error message.
		m.isLoading = false
		if msg.err != nil {
			m.addMessage(roleSystem, fmt.Sprintf("Compaction failed: %v", msg.err))
		} else {
			m.addMessage(roleSystem, "Compaction failed")
		}
	}

	// Update viewport content after adding message.
	m.updateViewportContent()

	return true
}

// handleSpinnerTick handles spinner animation updates.
func (m *ChatModel) handleSpinnerTick(msg spinner.TickMsg, cmds *[]tea.Cmd) bool {
	if m.isLoading {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		*cmds = append(*cmds, cmd)
	}
	return true
}

// handleProviderSwitched handles the result of an async provider switch.
func (m *ChatModel) handleProviderSwitched(msg providerSwitchedMsg) {
	if msg.err != nil {
		m.addMessage(roleSystem, fmt.Sprintf("Error switching provider: %v", msg.err))
		m.updateViewportContent()
		return
	}

	// Replace the client.
	m.client = msg.newClient

	// Update session if available.
	if m.sess != nil {
		m.sess.Provider = msg.provider
		m.sess.Model = msg.providerConfig.Model

		// Persist session update asynchronously.
		if m.manager != nil {
			manager := m.manager
			sess := m.sess
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := manager.UpdateSession(ctx, sess); err != nil {
					log.Debugf("Failed to persist provider switch in session: %v", err)
				}
			}()
		}
	}

	// Add system message indicating the switch.
	providerName := msg.provider
	for _, p := range availableProviders {
		if p.Name == msg.provider {
			providerName = p.Description
			break
		}
	}
	m.addMessage(roleSystem, fmt.Sprintf("\U0001f504 Switched to %s (model: %s)\n\nStarting fresh conversation with this provider.", providerName, msg.providerConfig.Model))

	// Force viewport update to show the message immediately.
	m.updateViewportContent()
}

// handleWindowResize processes window size changes and adjusts UI components.
func (m *ChatModel) handleWindowResize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height

	// Fixed textarea height as requested (7 lines).
	const textareaHeight = 7

	// Set textarea size.
	m.textarea.SetWidth(msg.Width - 4)
	m.textarea.SetHeight(textareaHeight)

	viewportHeight := m.calculateViewportHeight(msg.Height)
	m.updateViewportSize(msg.Width, viewportHeight)
	m.cleanTextareaANSI()
	m.recreateMarkdownRenderer(msg.Width)
	m.updateViewportContent()
}

// calculateViewportHeight calculates the viewport height from the total window height.
func (m *ChatModel) calculateViewportHeight(totalHeight int) int {
	// Use fixed heights for header and footer to avoid measurement issues:
	// Header: title (1) + subtitle (1) + session info (1) + padding = 4 lines
	// Footer: border (1) + top padding (1) + textarea (7) + newline (1) + help (1) + bottom padding (1) = 12 lines
	// Separators: 2 newlines between header/viewport/footer = 2 lines
	// Total non-viewport space: 4 + 12 + 2 = 18 lines.
	const headerAndFooterHeight = 18

	viewportHeight := totalHeight - headerAndFooterHeight
	if viewportHeight < minViewportHeight {
		viewportHeight = minViewportHeight
	}
	return viewportHeight
}

// updateViewportSize initializes or updates the viewport dimensions.
func (m *ChatModel) updateViewportSize(width, height int) {
	const headerHeight = 4

	if !m.ready {
		m.viewport = viewport.New(width, height)
		m.viewport.YPosition = headerHeight + 1
		m.ready = true
	} else {
		m.viewport.Width = width
		m.viewport.Height = height
	}
}

// cleanTextareaANSI strips ANSI escape sequences from the textarea.
func (m *ChatModel) cleanTextareaANSI() {
	currentValue := m.textarea.Value()
	if currentValue == "" {
		return
	}

	cleanedValue := stripANSI(currentValue)
	if cleanedValue != currentValue {
		m.textarea.SetValue(cleanedValue)
	}
}

// recreateMarkdownRenderer creates a new markdown renderer with the given width.
func (m *ChatModel) recreateMarkdownRenderer(windowWidth int) {
	width := windowWidth - 4
	if width < minMarkdownWidth {
		width = minMarkdownWidth
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
		glamour.WithColorProfile(lipgloss.ColorProfile()),
		glamour.WithEmoji(),
	)
	if err != nil {
		log.Debugf("Failed to recreate markdown renderer on resize: %v", err)
		return
	}

	m.markdownRenderer = renderer
	m.renderedMessages = make([]string, 0) // Clear cache.
}

// View renders the chat interface.
func (m *ChatModel) View() string {
	if !m.ready {
		return "\n  Initializing Atmos AI Chat..."
	}

	// Render appropriate view based on current mode.
	switch m.currentView {
	case viewModeSessionList:
		return m.sessionListView()
	case viewModeCreateSession:
		return m.createSessionView()
	case viewModeProviderSelect:
		return m.providerSelectView()
	case viewModeSkillSelect:
		return m.skillSelectView()
	default:
		return fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.viewport.View(), m.footerView())
	}
}

func (m *ChatModel) headerView() string {
	title := "Atmos AI Assistant"
	subtitle := "Ask questions about your infrastructure, components, and stacks"

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorCyan)).
		Bold(true).
		Padding(0, 1)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Padding(0, 1)

	sessionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Italic(true).
		Padding(0, 1)

	lines := []string{
		titleStyle.Render(title),
		subtitleStyle.Render(subtitle),
	}

	// Add session info if available.
	if m.sess != nil {
		sessionInfo := fmt.Sprintf("Session: %s | Created: %s | Messages: %d",
			m.sess.Name,
			m.sess.CreatedAt.Format("Jan 02, 15:04"),
			len(m.messages))
		lines = append(lines, sessionStyle.Render(sessionInfo))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m *ChatModel) footerView() string {
	var content string

	if m.isLoading {
		content = m.loadingFooterContent()
	} else {
		content = m.inputFooterContent()
	}

	footerStyle := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		Padding(1, 0)

	return footerStyle.Render(content)
}

// loadingFooterContent renders the footer when AI is processing.
func (m *ChatModel) loadingFooterContent() string {
	cancelHint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		Render("(Press Esc to cancel)")

	usageStr := ""
	if m.cumulativeUsage.TotalTokens > 0 {
		usageStr = fmt.Sprintf(" \u00b7 %s tokens", formatTokenCount(m.cumulativeUsage.TotalTokens))
	}

	loadingText := m.loadingText
	if loadingText == "" {
		loadingText = "AI is thinking..."
	}
	return fmt.Sprintf("%s %s%s %s", m.spinner.View(), loadingText, usageStr, cancelHint)
}

// inputFooterContent renders the footer with the input area.
func (m *ChatModel) inputFooterContent() string {
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)

	help := helpStyle.Render("Enter: Send | Ctrl+J: Newline | Ctrl+L: Sessions | Ctrl+N: New | Ctrl+P: Provider | Ctrl+A: Skill | Alt+Drag: Select Text | Ctrl+C: Quit")
	return fmt.Sprintf("%s\n%s", m.textarea.View(), help)
}
