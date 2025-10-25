package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/memory"
	"github.com/cloudposse/atmos/pkg/ai/session"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	aiTypes "github.com/cloudposse/atmos/pkg/ai/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// DefaultViewportWidth is the default width for the chat viewport before window sizing.
	DefaultViewportWidth = 80
	// DefaultViewportHeight is the default height for the chat viewport before window sizing.
	DefaultViewportHeight = 20

	// Markdown rendering constants.
	minMarkdownWidth = 20
	newlineChar      = "\n"

	// Message roles.
	roleUser      = "user"
	roleAssistant = "assistant"
	roleSystem    = "system"
)

// availableProviders lists all AI providers that can be switched between during a session.
var availableProviders = []struct {
	Name        string
	Description string
}{
	{"anthropic", "Anthropic Claude - Industry-leading reasoning and coding"},
	{"openai", "OpenAI GPT - Most popular, widely adopted models"},
	{"gemini", "Google Gemini - Strong multimodal capabilities"},
	{"grok", "xAI Grok - Real-time data access"},
	{"ollama", "Ollama - Local models for privacy and offline use"},
	{"bedrock", "AWS Bedrock - Enterprise-grade AI with AWS security and compliance"},
	{"azureopenai", "Azure OpenAI - Enterprise OpenAI with Microsoft Azure integration"},
}

// viewMode represents the current view mode of the TUI.
type viewMode int

const (
	viewModeChat viewMode = iota
	viewModeSessionList
	viewModeCreateSession
	viewModeProviderSelect
)

// ChatModel represents the state of the chat TUI.
type ChatModel struct {
	client               ai.Client
	atmosConfig          *schema.AtmosConfiguration // Configuration for recreating clients when switching providers
	manager              *session.Manager
	sess                 *session.Session
	executor             *tools.Executor
	memoryMgr            *memory.Manager
	messages             []ChatMessage
	viewport             viewport.Model
	textarea             textarea.Model
	spinner              spinner.Model
	isLoading            bool
	width                int
	height               int
	ready                bool
	currentView          viewMode
	availableSessions    []*session.Session
	selectedSessionIndex int
	sessionListError     string
	createForm           createSessionForm
	deleteConfirm        bool            // Whether we're in delete confirmation state
	deleteSessionID      string          // ID of session to delete
	renameMode           bool            // Whether we're in rename mode
	renameSessionID      string          // ID of session to rename
	renameInput          textinput.Model // Text input for new session name
	sessionFilter        string          // Current provider filter ("all", "anthropic", "openai", "gemini", "grok")
	messageHistory       []string        // History of user messages for navigation
	historyIndex         int             // Current position in history (-1 = not navigating)
	historyBuffer        string          // Temporary buffer for current input when navigating
	providerSelectMode   bool            // Whether we're in provider selection mode
	selectedProviderIdx  int             // Selected provider index in provider selection
}

// ChatMessage represents a single message in the chat.
type ChatMessage struct {
	Role    string // "user" or "assistant"
	Content string
	Time    time.Time
}

// NewChatModel creates a new chat model with the provided AI client.
func NewChatModel(client ai.Client, atmosConfig *schema.AtmosConfiguration, manager *session.Manager, sess *session.Session, executor *tools.Executor, memoryMgr *memory.Manager) (*ChatModel, error) {
	if client == nil {
		return nil, errUtils.ErrAIClientNil
	}

	// Initialize viewport.
	vp := viewport.New(DefaultViewportWidth, DefaultViewportHeight)
	vp.SetContent("")

	// Initialize textarea.
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Enter to send, Ctrl+J for new line, Ctrl+C to quit)"
	ta.Focus()
	ta.ShowLineNumbers = false
	ta.CharLimit = 0 // No character limit

	// Initialize spinner.
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))

	model := &ChatModel{
		client:               client,
		atmosConfig:          atmosConfig,
		manager:              manager,
		sess:                 sess,
		executor:             executor,
		memoryMgr:            memoryMgr,
		messages:             make([]ChatMessage, 0),
		viewport:             vp,
		textarea:             ta,
		spinner:              s,
		isLoading:            false,
		currentView:          viewModeChat,
		availableSessions:    make([]*session.Session, 0),
		selectedSessionIndex: 0,
		createForm:           newCreateSessionForm(),
		messageHistory:       make([]string, 0),
		historyIndex:         -1,
	}

	// Load existing messages from session if available.
	if manager != nil && sess != nil {
		if err := model.loadSessionMessages(); err != nil {
			log.Warn(fmt.Sprintf("Failed to load session messages: %v", err))
		}
	}

	return model, nil
}

// loadSessionMessages loads existing messages from the session.
func (m *ChatModel) loadSessionMessages() error {
	if m.manager == nil || m.sess == nil {
		return nil
	}

	ctx := context.Background()
	sessionMessages, err := m.manager.GetMessages(ctx, m.sess.ID, 0)
	if err != nil {
		return fmt.Errorf("failed to get session messages: %w", err)
	}

	// Convert session messages to chat messages and populate history.
	for _, msg := range sessionMessages {
		m.messages = append(m.messages, ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
			Time:    msg.CreatedAt,
		})
		// Add user messages to history for navigation
		if msg.Role == roleUser {
			m.messageHistory = append(m.messageHistory, msg.Content)
		}
	}

	return nil
}

// switchProvider switches the AI provider mid-session while preserving message history.
func (m *ChatModel) switchProvider(provider string) error {
	if m.atmosConfig == nil {
		return fmt.Errorf("cannot switch provider: atmosConfig is nil")
	}

	// Get provider-specific configuration to validate it exists.
	providerConfig, err := ai.GetProviderConfig(m.atmosConfig, provider)
	if err != nil {
		return fmt.Errorf("cannot switch to provider %s: %w", provider, err)
	}

	// Store old provider for rollback on failure.
	oldDefaultProvider := m.atmosConfig.Settings.AI.DefaultProvider

	// Update atmosConfig to use the new provider.
	m.atmosConfig.Settings.AI.DefaultProvider = provider

	// Create new client with the updated provider.
	newClient, err := ai.NewClient(m.atmosConfig)
	if err != nil {
		// Restore old settings on failure.
		m.atmosConfig.Settings.AI.DefaultProvider = oldDefaultProvider
		return fmt.Errorf("failed to create new client for provider %s: %w", provider, err)
	}

	// Replace the client.
	m.client = newClient

	// Update session if available.
	if m.sess != nil {
		m.sess.Provider = provider
		m.sess.Model = providerConfig.Model

		// Persist session update if manager is available.
		if m.manager != nil {
			ctx := context.Background()
			if err := m.manager.UpdateSession(ctx, m.sess); err != nil {
				log.Warn(fmt.Sprintf("Failed to persist provider switch in session: %v", err))
			}
		}
	}

	// Add system message indicating the switch.
	providerName := provider
	for _, p := range availableProviders {
		if p.Name == provider {
			providerName = p.Description
			break
		}
	}
	m.addMessage(roleAssistant, fmt.Sprintf("🔄 Switched to %s (model: %s)\n\nYour message history has been preserved.", providerName, providerConfig.Model))

	return nil
}

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

// handleWindowResize processes window size changes and adjusts UI components.
func (m *ChatModel) handleWindowResize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height

	// Fixed textarea height as requested (7 lines).
	const textareaHeight = 7

	// Set textarea size.
	m.textarea.SetWidth(msg.Width - 4)
	m.textarea.SetHeight(textareaHeight)

	// Use fixed heights for header and footer to avoid measurement issues:
	// Header: title (1) + subtitle (1) + session info (1) + padding = 4 lines
	// Footer: border (1) + top padding (1) + textarea (7) + newline (1) + help (1) + bottom padding (1) = 12 lines
	// Separators: 2 newlines between header/viewport/footer = 2 lines
	// Total non-viewport space: 4 + 12 + 2 = 18 lines
	const headerHeight = 4
	const headerAndFooterHeight = 18

	// Viewport gets all remaining space.
	viewportHeight := msg.Height - headerAndFooterHeight
	if viewportHeight < 10 {
		viewportHeight = 10 // Minimum viewport height
	}

	if !m.ready {
		// Initialize viewport size.
		m.viewport = viewport.New(msg.Width, viewportHeight)
		m.viewport.YPosition = headerHeight + 1
		m.ready = true
	} else {
		// Adjust existing sizes.
		m.viewport.Width = msg.Width
		m.viewport.Height = viewportHeight
	}

	// Clean any ANSI escape sequences that may have leaked into textarea during resize.
	// Terminal emulators often send OSC queries during resize events.
	if currentValue := m.textarea.Value(); currentValue != "" {
		cleanedValue := stripANSI(currentValue)
		if cleanedValue != currentValue {
			m.textarea.SetValue(cleanedValue)
		}
	}

	m.updateViewportContent()
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

		// Strip any ANSI escape sequences that leaked into textarea during the update.
		// Terminal emulators can send OSC sequences as input at any time.
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

	case sendMessageMsg:
		return m.handleSendMessage(msg)

	case aiResponseMsg, aiErrorMsg:
		return m.handleAIMessage(msg), nil

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

// handleSendMessage handles user message sending.
func (m *ChatModel) handleSendMessage(msg sendMessageMsg) (bool, tea.Cmd) {
	m.addMessage(roleUser, string(msg))
	// Add to message history for navigation
	m.messageHistory = append(m.messageHistory, string(msg))
	m.historyIndex = -1  // Reset history navigation
	m.historyBuffer = "" // Clear history buffer
	m.textarea.Reset()
	m.isLoading = true
	m.updateViewportContent()
	return true, tea.Batch(
		m.spinner.Tick,
		m.getAIResponse(string(msg)),
	)
}

// handleAIMessage handles AI response and error messages.
func (m *ChatModel) handleAIMessage(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case aiResponseMsg:
		m.addMessage(roleAssistant, string(msg))
	case aiErrorMsg:
		m.addMessage(roleSystem, fmt.Sprintf("Error: %s", string(msg)))
	}
	m.isLoading = false
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
		content = fmt.Sprintf("%s AI is thinking...", m.spinner.View())
	} else {
		helpStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)

		help := helpStyle.Render("Enter: Send | Ctrl+J: Newline | Ctrl+L: Sessions | Ctrl+N: New | Ctrl+P: Provider | Alt+Drag: Select Text | Ctrl+C: Quit")
		content = fmt.Sprintf("%s\n%s", m.textarea.View(), help)
	}

	footerStyle := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		Padding(1, 0)

	return footerStyle.Render(content)
}

func (m *ChatModel) addMessage(role, content string) {
	message := ChatMessage{
		Role:    role,
		Content: content,
		Time:    time.Now(),
	}
	m.messages = append(m.messages, message)

	// Save message to session if available.
	if m.manager != nil && m.sess != nil && role != roleSystem {
		ctx := context.Background()
		if err := m.manager.AddMessage(ctx, m.sess.ID, role, content); err != nil {
			log.Warn(fmt.Sprintf("Failed to save message to session: %v", err))
		}
	}
}

func (m *ChatModel) updateViewportContent() {
	var contentParts []string

	// Add empty line at the top for spacing after header.
	contentParts = append(contentParts, "")

	for _, msg := range m.messages {
		var style lipgloss.Style
		var prefix string

		switch msg.Role {
		case roleUser:
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.ColorGreen)).
				Bold(true)
			prefix = "You:"
		case roleAssistant:
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.ColorCyan))
			prefix = "Atmos AI:"
		case roleSystem:
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.ColorRed)).
				Italic(true)
			prefix = "System:"
		}

		timestamp := msg.Time.Format("15:04")
		timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

		header := fmt.Sprintf("%s %s", style.Render(prefix), timeStyle.Render(timestamp))

		// Render content with markdown for assistant messages
		var renderedContent string
		if msg.Role == roleAssistant {
			// Render markdown with syntax highlighting
			renderedContent = m.renderMarkdown(msg.Content)
		} else {
			// Plain text for user and system messages
			contentStyle := lipgloss.NewStyle().
				PaddingLeft(2).
				Width(m.viewport.Width - 4)
			renderedContent = contentStyle.Render(msg.Content)
		}

		contentParts = append(contentParts, header)
		contentParts = append(contentParts, renderedContent)
		contentParts = append(contentParts, "") // Empty line between messages
	}

	m.viewport.SetContent(strings.Join(contentParts, newlineChar))
	m.viewport.GotoBottom()
}

// renderMarkdown renders markdown content with syntax highlighting using glamour.
func (m *ChatModel) renderMarkdown(content string) string {
	// Create glamour renderer with dark theme optimized for terminals
	width := m.viewport.Width - 4
	if width < minMarkdownWidth {
		width = minMarkdownWidth // Minimum width
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		// Fallback to plain text if markdown rendering fails
		return lipgloss.NewStyle().
			PaddingLeft(2).
			Width(m.viewport.Width - 4).
			Render(content)
	}

	rendered, err := renderer.Render(content)
	if err != nil {
		// Fallback to plain text if rendering fails
		return lipgloss.NewStyle().
			PaddingLeft(2).
			Width(m.viewport.Width - 4).
			Render(content)
	}

	// Add left padding to match other messages
	paddedLines := make([]string, 0)
	for _, line := range strings.Split(rendered, newlineChar) {
		paddedLines = append(paddedLines, "  "+line)
	}

	return strings.TrimRight(strings.Join(paddedLines, newlineChar), newlineChar)
}

// Custom message types.
type sendMessageMsg string

type aiResponseMsg string

type aiErrorMsg string

func (m *ChatModel) sendMessage(content string) tea.Cmd {
	return func() tea.Msg {
		return sendMessageMsg(content)
	}
}

func (m *ChatModel) getAIResponse(userMessage string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Build message with memory context if available.
		messageWithContext := userMessage
		if m.memoryMgr != nil {
			memoryContext := m.memoryMgr.GetContext()
			if memoryContext != "" {
				// Prepend memory context to the user message.
				messageWithContext = memoryContext + "\n---\n\n" + userMessage
			}
		}

		// Check if tools are available.
		var availableTools []tools.Tool
		if m.executor != nil {
			availableTools = m.executor.ListTools()
		}

		// Use tool calling if tools are available.
		if len(availableTools) > 0 {
			// Send message with tools.
			response, err := m.client.SendMessageWithTools(ctx, messageWithContext, availableTools)
			if err != nil {
				return aiErrorMsg(err.Error())
			}

			// Check if AI wants to use tools.
			if response.StopReason == aiTypes.StopReasonToolUse && len(response.ToolCalls) > 0 {
				// Execute tools and get results.
				toolResults := m.executeToolCalls(ctx, response.ToolCalls)

				// For now, just display tool results to the user.
				// TODO: Implement multi-turn conversation with tool results.
				var resultText string
				for i, result := range toolResults {
					if i > 0 {
						resultText += "\n\n"
					}
					resultText += fmt.Sprintf("**Tool: %s**\n%s", response.ToolCalls[i].Name, result.Output)
				}

				finalResponse := response.Content
				if resultText != "" {
					if finalResponse != "" {
						finalResponse += "\n\n--- Tool Execution ---\n\n" + resultText
					} else {
						finalResponse = "--- Tool Execution ---\n\n" + resultText
					}
				}
				return aiResponseMsg(finalResponse)
			}

			// No tool use, return the text response.
			return aiResponseMsg(response.Content)
		}

		// Fallback to simple message without tools.
		response, err := m.client.SendMessage(ctx, messageWithContext)
		if err != nil {
			return aiErrorMsg(err.Error())
		}

		return aiResponseMsg(response)
	}
}

// executeToolCalls executes tool calls and returns the results.
func (m *ChatModel) executeToolCalls(ctx context.Context, toolCalls []aiTypes.ToolCall) []*tools.Result {
	results := make([]*tools.Result, len(toolCalls))

	for i, toolCall := range toolCalls {
		log.Debug(fmt.Sprintf("Executing tool: %s with params: %v", toolCall.Name, toolCall.Input))

		// Execute the tool.
		result, err := m.executor.Execute(ctx, toolCall.Name, toolCall.Input)
		if err != nil {
			results[i] = &tools.Result{
				Success: false,
				Output:  fmt.Sprintf("Error: %v", err),
				Error:   err,
			}
		} else {
			results[i] = result
		}
	}

	return results
}

// RunChat starts the chat TUI with the provided AI client.
func RunChat(client ai.Client, atmosConfig *schema.AtmosConfiguration, manager *session.Manager, sess *session.Session, executor *tools.Executor, memoryMgr *memory.Manager) error {
	model, err := NewChatModel(client, atmosConfig, manager, sess, executor, memoryMgr)
	if err != nil {
		return fmt.Errorf("failed to create chat model: %w", err)
	}

	// Add welcome message only if this is a new session (no existing messages).
	if len(model.messages) == 0 {
		model.addMessage(roleAssistant, `Welcome to Atmos AI Assistant! 👽

I'm here to help you with your Atmos infrastructure management. I can:

• Describe components and their configurations
• List available components and stacks
• Validate stack configurations
• Generate Terraform plans (read-only)
• Answer questions about Atmos concepts and best practices
• Help debug configuration issues

Try asking me something like:
- "List all available components"
- "Describe the vpc component in the dev stack"
- "What are Atmos stacks?"
- "How do I validate my stack configuration?"

What would you like to know?`)
	} else {
		// Resuming existing session.
		sessionName := "session"
		if sess != nil {
			sessionName = sess.Name
		}
		model.addMessage(roleSystem, fmt.Sprintf("Resumed session: %s (%d messages)", sessionName, len(model.messages)))
	}

	model.updateViewportContent()

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // Enable mouse wheel scrolling
	)
	_, err = p.Run()
	return err
}

// getConfiguredProviders returns only the providers that are configured in atmos.yaml.
func (m *ChatModel) getConfiguredProviders() []struct {
	Name        string
	Description string
} {
	if m.atmosConfig == nil || m.atmosConfig.Settings.AI.Providers == nil {
		return availableProviders
	}

	configured := make([]struct {
		Name        string
		Description string
	}, 0)

	for _, provider := range availableProviders {
		// Check if this provider is configured
		if _, exists := m.atmosConfig.Settings.AI.Providers[provider.Name]; exists {
			configured = append(configured, provider)
		}
	}

	// If no providers are configured, show all (fallback for backward compatibility)
	if len(configured) == 0 {
		return availableProviders
	}

	return configured
}

// providerSelectView renders the provider selection interface.
func (m *ChatModel) providerSelectView() string {
	var content strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan)).
		MarginBottom(1)
	content.WriteString(titleStyle.Render("Switch AI Provider"))
	content.WriteString(newlineChar)

	// Help text
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGray)).
		Margin(0, 0, 1, 0)
	content.WriteString(helpStyle.Render("↑/↓: Navigate | Enter: Select | Esc/q: Cancel"))
	content.WriteString(doubleNewline)

	// Current provider indicator
	// Use session provider if available, otherwise use configured default.
	currentProvider := ""
	if m.sess != nil && m.sess.Provider != "" {
		currentProvider = m.sess.Provider
	} else if m.atmosConfig.Settings.AI.DefaultProvider != "" {
		currentProvider = m.atmosConfig.Settings.AI.DefaultProvider
	} else {
		currentProvider = "anthropic" // Default fallback
	}

	// Provider list
	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan)).
		Background(lipgloss.Color(theme.ColorGray))
	normalStyle := lipgloss.NewStyle()
	currentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGreen))

	// Get only configured providers
	configuredProviders := m.getConfiguredProviders()

	for i, provider := range configuredProviders {
		var line string
		prefix := "  "
		if i == m.selectedProviderIdx {
			prefix = "▶ "
		}

		providerInfo := fmt.Sprintf("%s%s", prefix, provider.Name)
		if provider.Name == currentProvider {
			providerInfo += " (current)"
		}
		providerInfo += fmt.Sprintf("\n    %s", provider.Description)

		if i == m.selectedProviderIdx {
			line = selectedStyle.Render(providerInfo)
		} else if provider.Name == currentProvider {
			line = currentStyle.Render(providerInfo)
		} else {
			line = normalStyle.Render(providerInfo)
		}

		content.WriteString(line)
		content.WriteString(doubleNewline)
	}

	return content.String()
}
