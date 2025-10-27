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
	deleteConfirm        bool                  // Whether we're in delete confirmation state
	deleteSessionID      string                // ID of session to delete
	renameMode           bool                  // Whether we're in rename mode
	renameSessionID      string                // ID of session to rename
	renameInput          textinput.Model       // Text input for new session name
	sessionFilter        string                // Current provider filter ("all", "anthropic", "openai", "gemini", "grok")
	messageHistory       []string              // History of user messages for navigation
	historyIndex         int                   // Current position in history (-1 = not navigating)
	historyBuffer        string                // Temporary buffer for current input when navigating
	providerSelectMode   bool                  // Whether we're in provider selection mode
	selectedProviderIdx  int                   // Selected provider index in provider selection
	markdownRenderer     *glamour.TermRenderer // Cached markdown renderer for performance
	renderedMessages     []string              // Cache of rendered messages to avoid re-rendering
}

// ChatMessage represents a single message in the chat.
type ChatMessage struct {
	Role     string // "user" or "assistant"
	Content  string
	Time     time.Time
	Provider string // The AI provider that generated this message (for assistant messages)
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

	// Initialize cached markdown renderer for performance.
	// Creating glamour renderers is expensive, so we create one and reuse it.
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(DefaultViewportWidth-4),
		glamour.WithColorProfile(lipgloss.ColorProfile()),
		glamour.WithEmoji(),
	)
	if err != nil {
		log.Debug(fmt.Sprintf("Failed to create cached markdown renderer: %v", err))
		// Continue without renderer - will fallback to plain text
	}

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
		markdownRenderer:     renderer,
		renderedMessages:     make([]string, 0),
	}

	// Load existing messages from session if available.
	if manager != nil && sess != nil {
		if err := model.loadSessionMessages(); err != nil {
			log.Debug(fmt.Sprintf("Failed to load session messages: %v", err))
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
	// Use the session's provider for historical messages.
	sessionProvider := ""
	if m.sess != nil {
		sessionProvider = m.sess.Provider
	}

	for _, msg := range sessionMessages {
		// Preserve the provider for assistant messages.
		provider := ""
		if msg.Role == roleAssistant {
			provider = sessionProvider
		}

		m.messages = append(m.messages, ChatMessage{
			Role:     msg.Role,
			Content:  msg.Content,
			Time:     msg.CreatedAt,
			Provider: provider,
		})
		// Add user messages to history for navigation
		if msg.Role == roleUser {
			m.messageHistory = append(m.messageHistory, msg.Content)
		}
	}

	return nil
}

// switchProviderAsync initiates an asynchronous provider switch to avoid blocking the UI.
// PERFORMANCE: Creating AI clients can take 1-3 seconds, so we do it async.
func (m *ChatModel) switchProviderAsync(provider string) tea.Cmd {
	return func() tea.Msg {
		if m.atmosConfig == nil {
			return providerSwitchedMsg{
				provider: provider,
				err:      fmt.Errorf("cannot switch provider: atmosConfig is nil"),
			}
		}

		// Get provider-specific configuration to validate it exists.
		providerConfig, err := ai.GetProviderConfig(m.atmosConfig, provider)
		if err != nil {
			return providerSwitchedMsg{
				provider: provider,
				err:      fmt.Errorf("cannot switch to provider %s: %w", provider, err),
			}
		}

		// Store old provider for rollback on failure.
		oldDefaultProvider := m.atmosConfig.Settings.AI.DefaultProvider

		// Update atmosConfig to use the new provider.
		m.atmosConfig.Settings.AI.DefaultProvider = provider

		// Create new client with the updated provider.
		// PERFORMANCE: This can take 1-3 seconds, which is why we run it async.
		newClient, err := ai.NewClient(m.atmosConfig)
		if err != nil {
			// Restore old settings on failure.
			m.atmosConfig.Settings.AI.DefaultProvider = oldDefaultProvider
			return providerSwitchedMsg{
				provider: provider,
				err:      fmt.Errorf("failed to create new client for provider %s: %w", provider, err),
			}
		}

		return providerSwitchedMsg{
			provider:       provider,
			providerConfig: providerConfig,
			newClient:      newClient,
			err:            nil,
		}
	}
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

	// Recreate markdown renderer with new width for proper word wrapping.
	// Clear rendered message cache since width changed.
	width := msg.Width - 4
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
		log.Debug(fmt.Sprintf("Failed to recreate markdown renderer on resize: %v", err))
	} else {
		m.markdownRenderer = renderer
		m.renderedMessages = make([]string, 0) // Clear cache
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
	// Only handle left button presses
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return false, nil
	}

	// Handle mouse clicks in create session view
	if m.currentView == viewModeCreateSession {
		// Simple heuristic: clicks in upper area (rows 0-6) focus name field,
		// clicks in lower area (rows 7+) focus provider field
		if msg.Y <= 6 {
			// Click in name field area - focus it
			if m.createForm.focusedField != 0 {
				m.createForm.focusedField = 0
				m.createForm.nameInput.Focus()
			}
			return true, nil
		} else {
			// Click in provider area - focus provider field
			if m.createForm.focusedField != 1 {
				m.createForm.focusedField = 1
				m.createForm.nameInput.Blur()
			}
			return true, nil
		}
	}

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
					log.Debug(fmt.Sprintf("Failed to persist provider switch in session: %v", err))
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
	m.addMessage(roleSystem, fmt.Sprintf("🔄 Switched to %s (model: %s)\n\nStarting fresh conversation with this provider.", providerName, msg.providerConfig.Model))

	// Force viewport update to show the message immediately.
	m.updateViewportContent()
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

// detectOutputFormat detects the format of command output and returns the appropriate markdown language tag.
func detectOutputFormat(output string) string {
	trimmed := strings.TrimSpace(output)

	// Check for JSON
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return "json"
	}

	// Check for YAML (looks for key: value patterns at start of lines)
	lines := strings.Split(trimmed, "\n")
	yamlPatterns := 0
	for i, line := range lines {
		if i > 10 {
			break // Check first 10 lines
		}
		trimmedLine := strings.TrimSpace(line)
		// YAML key-value pattern or list item
		if strings.Contains(trimmedLine, ": ") || strings.HasPrefix(trimmedLine, "- ") {
			yamlPatterns++
		}
	}
	if yamlPatterns >= 3 { // If we see 3+ yaml patterns, it's probably YAML
		return "yaml"
	}

	// Check for HCL/Terraform
	if strings.Contains(trimmed, "resource \"") || strings.Contains(trimmed, "data \"") ||
		strings.Contains(trimmed, "module \"") || strings.Contains(trimmed, "variable \"") {
		return "hcl"
	}

	// Check for tables (multiple lines with consistent column separators)
	if len(lines) > 2 {
		pipeCount := strings.Count(lines[0], "|")
		if pipeCount > 1 {
			consistentPipes := true
			for i := 1; i < min(5, len(lines)); i++ {
				if strings.Count(lines[i], "|") != pipeCount {
					consistentPipes = false
					break
				}
			}
			if consistentPipes {
				return "text" // Tables render better as text in glamour
			}
		}
	}

	// Default to text
	return "text"
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (m *ChatModel) addMessage(role, content string) {
	// Capture the current provider for all non-system messages.
	// This ensures complete conversation isolation when switching providers.
	provider := ""
	if role != roleSystem && m.sess != nil && m.sess.Provider != "" {
		provider = m.sess.Provider
	}

	message := ChatMessage{
		Role:     role,
		Content:  content,
		Time:     time.Now(),
		Provider: provider,
	}
	m.messages = append(m.messages, message)

	// Save message to session if available.
	// IMPORTANT: This runs asynchronously to prevent UI freezes during database writes.
	// Database operations can take 3-5 seconds depending on disk speed and load.
	if m.manager != nil && m.sess != nil && role != roleSystem {
		// Capture values before goroutine to avoid race conditions.
		manager := m.manager
		sessionID := m.sess.ID
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := manager.AddMessage(ctx, sessionID, role, content); err != nil {
				log.Debug(fmt.Sprintf("Failed to save message to session: %v", err))
			}
		}()
	}
}

func (m *ChatModel) updateViewportContent() {
	// PERFORMANCE OPTIMIZATION: Only render new messages, reuse cached renders.
	// This dramatically improves performance with many messages.

	// Calculate how many messages need rendering.
	numCached := len(m.renderedMessages)
	numTotal := len(m.messages)

	// If cache is empty or invalid, render all messages.
	if numCached == 0 || numCached > numTotal {
		m.renderedMessages = make([]string, 0, numTotal*3) // Pre-allocate: header + content + empty line per message
		numCached = 0
	}

	// Render only new messages (from numCached to numTotal).
	for i := numCached; i < numTotal; i++ {
		msg := m.messages[i]

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
			// Include alien emoji and provider name in the prefix.
			provider := msg.Provider
			if provider == "" {
				provider = "unknown"
			}
			prefix = fmt.Sprintf("👽 Atmos AI (%s):", provider)
		case roleSystem:
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.ColorRed)).
				Italic(true)
			prefix = "System:"
		}

		timestamp := msg.Time.Format("15:04")
		timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		header := fmt.Sprintf("%s %s", style.Render(prefix), timeStyle.Render(timestamp))

		// Render content with markdown for assistant messages.
		var renderedContent string
		if msg.Role == roleAssistant {
			renderedContent = m.renderMarkdown(msg.Content)
		} else {
			// Plain text for user and system messages.
			contentStyle := lipgloss.NewStyle().
				PaddingLeft(2).
				Width(m.viewport.Width - 4)
			renderedContent = contentStyle.Render(msg.Content)
		}

		// Cache the rendered message parts.
		m.renderedMessages = append(m.renderedMessages, header)
		m.renderedMessages = append(m.renderedMessages, renderedContent)
		m.renderedMessages = append(m.renderedMessages, "") // Empty line between messages
	}

	// Build final content from cache with empty line at top.
	finalContent := append([]string{""}, m.renderedMessages...)
	m.viewport.SetContent(strings.Join(finalContent, newlineChar))
	m.viewport.GotoBottom()
}

// renderMarkdown renders markdown content with syntax highlighting using the cached glamour renderer.
// PERFORMANCE: Uses cached renderer instead of creating new one each time.
func (m *ChatModel) renderMarkdown(content string) string {
	// Fallback to plain text if no cached renderer available.
	if m.markdownRenderer == nil {
		return lipgloss.NewStyle().
			PaddingLeft(2).
			Width(m.viewport.Width - 4).
			Render(content)
	}

	// Use cached renderer for performance.
	rendered, err := m.markdownRenderer.Render(content)
	if err != nil {
		// Log the error and content length for debugging.
		log.Debug(fmt.Sprintf("Failed to render markdown (content length: %d): %v", len(content), err))
		// Fallback to plain text if rendering fails.
		return lipgloss.NewStyle().
			PaddingLeft(2).
			Width(m.viewport.Width - 4).
			Render(content)
	}

	// Add left padding to match other messages.
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

type providerSwitchedMsg struct {
	provider       string
	providerConfig *schema.AIProviderConfig
	newClient      ai.Client
	err            error
}

func (m *ChatModel) sendMessage(content string) tea.Cmd {
	return func() tea.Msg {
		return sendMessageMsg(content)
	}
}

func (m *ChatModel) getAIResponse(userMessage string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Build message history filtered by provider.
		// Only include messages from the current provider session for complete isolation.
		// This ensures clean separation when switching between providers.
		currentProvider := ""
		if m.sess != nil {
			currentProvider = m.sess.Provider
		}

		messages := make([]aiTypes.Message, 0, len(m.messages)+1)
		for _, msg := range m.messages {
			// Skip system messages (UI-only notifications).
			if msg.Role == roleSystem {
				continue
			}

			// Include only messages from the current provider session.
			// This provides complete conversation isolation when switching providers.
			if msg.Provider == currentProvider {
				messages = append(messages, aiTypes.Message{
					Role:    msg.Role,
					Content: msg.Content,
				})
			}
		}

		// Add current user message.
		messages = append(messages, aiTypes.Message{
			Role:    aiTypes.RoleUser,
			Content: userMessage,
		})

		// Apply memory context if available by prepending a system message.
		if m.memoryMgr != nil {
			memoryContext := m.memoryMgr.GetContext()
			if memoryContext != "" {
				// Prepend system message with context.
				messages = append([]aiTypes.Message{{
					Role:    aiTypes.RoleSystem,
					Content: memoryContext,
				}}, messages...)
			}
		}

		// Check if tools are available.
		var availableTools []tools.Tool
		if m.executor != nil {
			availableTools = m.executor.ListTools()
		}

		// Use tool calling if tools are available.
		if len(availableTools) > 0 {
			// Send messages with tools and full history.
			response, err := m.client.SendMessageWithToolsAndHistory(ctx, messages, availableTools)
			if err != nil {
				return aiErrorMsg(err.Error())
			}

			// Check if AI wants to use tools.
			if response.StopReason == aiTypes.StopReasonToolUse && len(response.ToolCalls) > 0 {
				// Execute tools and get results.
				toolResults := m.executeToolCalls(ctx, response.ToolCalls)

				// Multi-turn conversation: Send tool results back to AI for final response.
				// Build tool results message for the AI.
				var toolResultsContent string
				for i, result := range toolResults {
					if i > 0 {
						toolResultsContent += "\n\n"
					}

					// Determine what to send to AI: use Error if Output is empty or tool failed.
					toolOutput := result.Output
					if toolOutput == "" && result.Error != nil {
						toolOutput = fmt.Sprintf("Error: %v", result.Error)
					}
					if toolOutput == "" {
						toolOutput = "No output returned"
					}

					toolResultsContent += fmt.Sprintf("Tool: %s\nResult:\n%s", response.ToolCalls[i].Name, toolOutput)
				}

				// Add assistant's thinking/explanation to conversation history.
				if response.Content != "" {
					messages = append(messages, aiTypes.Message{
						Role:    aiTypes.RoleAssistant,
						Content: response.Content,
					})
				}

				// Add tool results as a user message (this is the standard pattern for tool results).
				messages = append(messages, aiTypes.Message{
					Role:    aiTypes.RoleUser,
					Content: fmt.Sprintf("Tool execution results:\n\n%s\n\nPlease provide your final response based on these results.", toolResultsContent),
				})

				// Call AI again with tool results to get final response.
				finalResponse, err := m.client.SendMessageWithToolsAndHistory(ctx, messages, availableTools)
				if err != nil {
					return aiErrorMsg(fmt.Sprintf("Error getting final response after tool execution: %v", err))
				}

				// Return the AI's final response (which should now include the table/analysis).
				return aiResponseMsg(finalResponse.Content)
			}

			// No tool use, return the text response.
			return aiResponseMsg(response.Content)
		}

		// Fallback to message with history but no tools.
		response, err := m.client.SendMessageWithHistory(ctx, messages)
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
		model.addMessage(roleAssistant, `I'm here to help you with your Atmos infrastructure management. I can:

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

// ProviderWithModel represents a provider with its configured model.
type ProviderWithModel struct {
	Name        string
	DisplayName string
	Model       string
}

// getConfiguredProvidersForCreate returns configured providers with their models from atmos.yaml.
func (m *ChatModel) getConfiguredProvidersForCreate() []ProviderWithModel {
	if m.atmosConfig == nil || m.atmosConfig.Settings.AI.Providers == nil {
		// Fallback to default providers with hardcoded models if no config
		return []ProviderWithModel{
			{Name: "anthropic", DisplayName: "Anthropic (Claude)", Model: "claude-sonnet-4-20250514"},
			{Name: "openai", DisplayName: "OpenAI (GPT)", Model: "gpt-4o"},
			{Name: "gemini", DisplayName: "Google (Gemini)", Model: "gemini-2.0-flash-exp"},
			{Name: "grok", DisplayName: "xAI (Grok)", Model: "grok-beta"},
			{Name: "ollama", DisplayName: "Ollama (Local)", Model: "llama3.3:70b"},
			{Name: "bedrock", DisplayName: "AWS Bedrock", Model: "anthropic.claude-sonnet-4-20250514-v2:0"},
			{Name: "azureopenai", DisplayName: "Azure OpenAI", Model: "gpt-4o"},
		}
	}

	// Map of display names
	displayNames := map[string]string{
		"anthropic":   "Anthropic (Claude)",
		"openai":      "OpenAI (GPT)",
		"gemini":      "Google (Gemini)",
		"grok":        "xAI (Grok)",
		"ollama":      "Ollama (Local)",
		"bedrock":     "AWS Bedrock",
		"azureopenai": "Azure OpenAI",
	}

	configured := make([]ProviderWithModel, 0)

	for _, provider := range availableProviders {
		// Check if this provider is configured
		if providerConfig, exists := m.atmosConfig.Settings.AI.Providers[provider.Name]; exists {
			displayName := displayNames[provider.Name]
			if displayName == "" {
				displayName = provider.Name // Fallback to name if no display name
			}

			configured = append(configured, ProviderWithModel{
				Name:        provider.Name,
				DisplayName: displayName,
				Model:       providerConfig.Model, // Use model from atmos.yaml
			})
		}
	}

	// If no providers are configured, return all with defaults
	if len(configured) == 0 {
		return []ProviderWithModel{
			{Name: "anthropic", DisplayName: "Anthropic (Claude)", Model: "claude-sonnet-4-20250514"},
			{Name: "openai", DisplayName: "OpenAI (GPT)", Model: "gpt-4o"},
			{Name: "gemini", DisplayName: "Google (Gemini)", Model: "gemini-2.0-flash-exp"},
			{Name: "grok", DisplayName: "xAI (Grok)", Model: "grok-beta"},
			{Name: "ollama", DisplayName: "Ollama (Local)", Model: "llama3.3:70b"},
			{Name: "bedrock", DisplayName: "AWS Bedrock", Model: "anthropic.claude-sonnet-4-20250514-v2:0"},
			{Name: "azureopenai", DisplayName: "Azure OpenAI", Model: "gpt-4o"},
		}
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
