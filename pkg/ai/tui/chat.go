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
	"github.com/charmbracelet/lipgloss/table"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/agents"
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
	viewModeAgentSelect
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
	cancelFunc           context.CancelFunc    // Function to cancel ongoing AI request
	isCancelling         bool                  // Whether we're in the process of cancelling
	cumulativeUsage      aiTypes.Usage         // Cumulative token usage for the session
	lastUsage            *aiTypes.Usage        // Usage from the last AI response
	maxHistoryMessages   int                   // Maximum conversation messages to keep in history (0 = unlimited)
	maxHistoryTokens     int                   // Maximum tokens in conversation history (0 = unlimited)
	agentRegistry        *agents.Registry      // Registry of available agents
	currentAgent         *agents.Agent         // Currently active agent
	agentSelectMode      bool                  // Whether we're in agent selection mode
	selectedAgentIdx     int                   // Selected agent index in agent selection UI
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

	// Get max history messages and tokens from configuration (0 = unlimited).
	maxHistoryMessages := 0
	maxHistoryTokens := 0
	if atmosConfig != nil {
		maxHistoryMessages = atmosConfig.Settings.AI.MaxHistoryMessages
		maxHistoryTokens = atmosConfig.Settings.AI.MaxHistoryTokens
	}

	// Load agent registry and set default agent.
	agentRegistry, err := agents.LoadAgents(atmosConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load agents: %w", err)
	}

	defaultAgentName := agents.GetDefaultAgent(atmosConfig)
	currentAgent, err := agentRegistry.Get(defaultAgentName)
	if err != nil {
		// Fall back to general agent if default not found.
		log.Debug(fmt.Sprintf("Default agent %q not found, falling back to general: %v", defaultAgentName, err))
		currentAgent, _ = agentRegistry.Get(agents.GeneralAgent)
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
		maxHistoryMessages:   maxHistoryMessages,
		maxHistoryTokens:     maxHistoryTokens,
		agentRegistry:        agentRegistry,
		currentAgent:         currentAgent,
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
	// Add to message history for navigation.
	m.messageHistory = append(m.messageHistory, string(msg))
	m.historyIndex = -1  // Reset history navigation.
	m.historyBuffer = "" // Clear history buffer.
	m.textarea.Reset()
	m.isLoading = true
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
	case aiErrorMsg:
		errorMsg := string(msg)
		// Don't show "Request cancelled" if user cancelled.
		if !m.isCancelling || errorMsg != "Request cancelled" {
			m.addMessage(roleSystem, fmt.Sprintf("Error: %s", errorMsg))
		} else if m.isCancelling {
			m.addMessage(roleSystem, "Request cancelled by user")
		}
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
	case viewModeAgentSelect:
		return m.agentSelectView()
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
		cancelHint := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true).
			Render("(Press Esc to cancel)")

		// Format cumulative usage if available.
		usageStr := ""
		if m.cumulativeUsage.TotalTokens > 0 {
			usageStr = fmt.Sprintf(" · %s tokens", formatTokenCount(m.cumulativeUsage.TotalTokens))
		}

		content = fmt.Sprintf("%s AI is thinking...%s %s", m.spinner.View(), usageStr, cancelHint)
	} else {
		helpStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)

		help := helpStyle.Render("Enter: Send | Ctrl+J: Newline | Ctrl+L: Sessions | Ctrl+N: New | Ctrl+P: Provider | Ctrl+A: Agent | Alt+Drag: Select Text | Ctrl+C: Quit")
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
			// Include alien emoji, agent icon, and provider name in the prefix.
			provider := msg.Provider
			if provider == "" {
				provider = "unknown"
			}
			// Add agent icon if we have a current agent
			agentIcon := ""
			if m.currentAgent != nil {
				agentIcon = getAgentIcon(m.currentAgent.Name) + " "
			}
			prefix = fmt.Sprintf("Atmos AI %s👽 (%s):", agentIcon, provider)
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
// Tables are detected and rendered using lipgloss.Table for better formatting.
func (m *ChatModel) renderMarkdown(content string) string {
	// Detect and extract markdown tables for special rendering.
	if hasMarkdownTable(content) {
		return m.renderMarkdownWithTables(content)
	}

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

// hasMarkdownTable detects if content contains a markdown table.
func hasMarkdownTable(content string) bool {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Look for separator line like |---|---|---|
		if strings.HasPrefix(trimmed, "|") && strings.Contains(trimmed, "---") {
			return true
		}
	}
	return false
}

// renderMarkdownWithTables renders markdown content with special handling for tables.
func (m *ChatModel) renderMarkdownWithTables(content string) string {
	lines := strings.Split(content, "\n")
	var result strings.Builder
	var tableLines []string
	inTable := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Check if this is a table line.
		isTableLine := strings.HasPrefix(trimmed, "|") && strings.Contains(trimmed, "|")

		if isTableLine {
			if !inTable {
				inTable = true
				tableLines = []string{}
			}
			tableLines = append(tableLines, line)
		} else {
			// End of table - render it.
			if inTable {
				table := m.renderTable(tableLines)
				result.WriteString(table)
				result.WriteString("\n")
				inTable = false
				tableLines = nil
			}

			// Render non-table content with glamour.
			if trimmed != "" {
				if m.markdownRenderer != nil {
					rendered, err := m.markdownRenderer.Render(line)
					if err == nil {
						result.WriteString("  " + strings.TrimSpace(rendered) + "\n")
					} else {
						result.WriteString("  " + line + "\n")
					}
				} else {
					result.WriteString("  " + line + "\n")
				}
			} else {
				result.WriteString("\n")
			}
		}
	}

	// Handle table at end of content.
	if inTable && len(tableLines) > 0 {
		table := m.renderTable(tableLines)
		result.WriteString(table)
	}

	return strings.TrimRight(result.String(), "\n")
}

// renderTable renders a markdown table using lipgloss.Table for better formatting.
func (m *ChatModel) renderTable(lines []string) string {
	if len(lines) < 2 {
		// Not a valid table.
		return strings.Join(lines, "\n")
	}

	// Parse table structure.
	var headers []string
	var rows [][]string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Split by | and clean up.
		parts := strings.Split(trimmed, "|")
		var cells []string
		for _, part := range parts {
			cell := strings.TrimSpace(part)
			if cell != "" && cell != "---" && !strings.Contains(cell, "---") {
				cells = append(cells, cell)
			}
		}

		if len(cells) == 0 {
			continue
		}

		if i == 0 {
			// Header row.
			headers = cells
		} else if i == 1 {
			// Separator row - skip.
			continue
		} else {
			// Data row.
			rows = append(rows, cells)
		}
	}

	// Create lipgloss table.
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				// Header style.
				return lipgloss.NewStyle().
					Foreground(lipgloss.Color(theme.ColorCyan)).
					Bold(true).
					Padding(0, 1)
			}
			// Data cell style.
			return lipgloss.NewStyle().Padding(0, 1)
		})

	// Set headers.
	if len(headers) > 0 {
		t.Headers(headers...)
	}

	// Add rows.
	for _, row := range rows {
		t.Row(row...)
	}

	// Render and add left padding.
	rendered := t.Render()
	paddedLines := make([]string, 0)
	for _, line := range strings.Split(rendered, "\n") {
		paddedLines = append(paddedLines, "  "+line)
	}

	return strings.Join(paddedLines, "\n")
}

// Custom message types.
type sendMessageMsg string

type aiResponseMsg struct {
	content string
	usage   *aiTypes.Usage
}

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

func (m *ChatModel) getAIResponseWithContext(userMessage string, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// Check if context is already cancelled before starting.
		if ctx.Err() != nil {
			return aiErrorMsg("Request cancelled")
		}

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

		// Apply sliding window to limit conversation history if configured.
		// This helps prevent rate limiting and reduces token usage for long conversations.
		// Two limits can be configured:
		// 1. Message-based: Keep only last N messages (simple, easy to configure)
		// 2. Token-based: Keep messages up to N tokens (more precise for rate limits)
		// If both are set, whichever limit is hit first is applied.

		pruneIndex := 0 // Start of messages to keep (0 = keep all)

		// Apply message-based limit if configured.
		if m.maxHistoryMessages > 0 && len(messages) > m.maxHistoryMessages {
			pruneIndex = len(messages) - m.maxHistoryMessages
		}

		// Apply token-based limit if configured.
		// Count backwards from most recent message and stop when token limit is exceeded.
		if m.maxHistoryTokens > 0 {
			totalTokens := 0
			tokenPruneIndex := len(messages)

			// Count backwards from most recent.
			for i := len(messages) - 1; i >= 0; i-- {
				msgTokens := estimateTokens(messages[i].Content)
				if totalTokens+msgTokens > m.maxHistoryTokens {
					tokenPruneIndex = i + 1 // Keep from i+1 onwards
					break
				}
				totalTokens += msgTokens
			}

			// Use whichever prune index is more restrictive (further right/more pruning).
			if tokenPruneIndex > pruneIndex {
				pruneIndex = tokenPruneIndex
			}
		}

		// Apply the pruning if needed.
		if pruneIndex > 0 && pruneIndex < len(messages) {
			messages = messages[pruneIndex:]
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
			// Get system prompt from current agent, or use default if no agent is set.
			systemPrompt := `You are an AI assistant for Atmos infrastructure management. You have access to tools that allow you to perform actions.

IMPORTANT: When you need to perform an action (read files, edit files, search, execute commands, etc.), you MUST use the available tools. Do NOT just describe what you would do - actually use the tools to do it.

For example:
- If you need to read a file, use the read_file tool immediately
- If you need to edit a file, use the edit_file tool immediately
- If you need to search for files, use the search_files tool immediately
- If you need to execute an Atmos command, use the execute_atmos_command tool immediately

Always take action using tools rather than describing what action you would take.`

			if m.currentAgent != nil && m.currentAgent.SystemPrompt != "" {
				systemPrompt = m.currentAgent.SystemPrompt
			}

			// Prepend system prompt.
			messages = append([]aiTypes.Message{{
				Role:    aiTypes.RoleSystem,
				Content: systemPrompt,
			}}, messages...)

			// Send messages with tools and full history.
			response, err := m.client.SendMessageWithToolsAndHistory(ctx, messages, availableTools)
			if err != nil {
				return aiErrorMsg(formatAPIError(err))
			}

			// Handle empty initial response.
			if response == nil {
				return aiErrorMsg("Received nil response from AI provider")
			}

			// Check if AI wants to use tools.
			if response.StopReason == aiTypes.StopReasonToolUse && len(response.ToolCalls) > 0 {
				return m.handleToolExecutionFlow(ctx, response, messages, availableTools)
			}

			// No tool use - check if AI expressed intent to take action but didn't use tools.
			if detectActionIntent(response.Content) {
				// AI said it would do something but didn't use tools. Prompt it to actually use them.
				messages = append(messages, aiTypes.Message{
					Role:    aiTypes.RoleAssistant,
					Content: response.Content,
				})
				messages = append(messages, aiTypes.Message{
					Role:    aiTypes.RoleUser,
					Content: "Please use the available tools to perform that action now, rather than just describing what you would do.",
				})

				// Send the prompt again.
				retryResponse, err := m.client.SendMessageWithToolsAndHistory(ctx, messages, availableTools)
				if err != nil {
					return aiErrorMsg(formatAPIError(err))
				}

				// Check if AI now uses tools.
				if retryResponse.StopReason == aiTypes.StopReasonToolUse && len(retryResponse.ToolCalls) > 0 {
					return m.handleToolExecutionFlow(ctx, retryResponse, messages, availableTools)
				}

				// Handle empty retry response.
				if retryResponse == nil || retryResponse.Content == "" {
					// Retry returned nil or empty - return original response.
					return aiResponseMsg{content: response.Content, usage: response.Usage}
				}

				// If still no tool use after retry, combine both responses.
				combinedContent := response.Content
				if retryResponse.Content != "" {
					if combinedContent != "" {
						combinedContent += "\n\n"
					}
					combinedContent += retryResponse.Content
				}
				return aiResponseMsg{content: combinedContent, usage: combineUsage(response.Usage, retryResponse.Usage)}
			}

			// No action intent detected, return the text response.
			return aiResponseMsg{content: response.Content, usage: response.Usage}
		}

		// Fallback to message with history but no tools.
		response, err := m.client.SendMessageWithHistory(ctx, messages)
		if err != nil {
			return aiErrorMsg(formatAPIError(err))
		}

		return aiResponseMsg{content: response, usage: nil}
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

// handleToolExecutionFlow executes tools, sends results back to AI, and returns the combined response.
func (m *ChatModel) handleToolExecutionFlow(ctx context.Context, response *aiTypes.Response, messages []aiTypes.Message, availableTools []tools.Tool) tea.Msg {
	return m.handleToolExecutionFlowWithAccumulator(ctx, response, messages, availableTools, "")
}

// handleToolExecutionFlowWithAccumulator executes tools, sends results back to AI, and returns the combined response.
// The accumulatedContent parameter preserves intermediate AI thinking across recursive tool calls.
func (m *ChatModel) handleToolExecutionFlowWithAccumulator(ctx context.Context, response *aiTypes.Response, messages []aiTypes.Message, availableTools []tools.Tool, accumulatedContent string) tea.Msg {
	// Execute tools and get results.
	toolResults := m.executeToolCalls(ctx, response.ToolCalls)

	// Build display output for user showing tool execution results.
	var resultText string
	if response.Content != "" {
		resultText = response.Content + "\n\n"
	}

	for i, result := range toolResults {
		if i > 0 {
			resultText += "\n\n"
		}

		// Determine what to display: use Error if Output is empty or tool failed.
		displayOutput := result.Output
		if displayOutput == "" && result.Error != nil {
			displayOutput = fmt.Sprintf("Error: %v", result.Error)
		}

		// Handle completely empty results.
		if displayOutput == "" {
			displayOutput = "No output returned"
		}

		// Build tool header with name and parameters.
		toolHeader := fmt.Sprintf("**Tool:** `%s`", response.ToolCalls[i].Name)

		// Show the actual command/parameters being executed for better visibility.
		if toolParams := formatToolParameters(response.ToolCalls[i]); toolParams != "" {
			toolHeader += "\n" + toolParams
		}

		// Detect output format and wrap in appropriate code block for syntax highlighting.
		format := detectOutputFormat(displayOutput)
		resultText += fmt.Sprintf("%s\n\n```%s\n%s\n```", toolHeader, format, displayOutput)
	}

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
		return aiErrorMsg(formatAPIError(err))
	}

	// Handle empty or truncated response from AI.
	if finalResponse == nil || (finalResponse.Content == "" && finalResponse.StopReason != aiTypes.StopReasonToolUse) {
		// AI returned empty response - this might indicate rate limiting or truncation.
		// Return what we have so far (tool results) with a note.
		combinedResponse := accumulatedContent
		if combinedResponse != "" {
			combinedResponse += "\n\n"
		}
		combinedResponse += resultText + "\n\n---\n\n*Note: AI response was empty. This might indicate rate limiting or a timeout.*"
		return aiResponseMsg{content: combinedResponse, usage: finalResponse.Usage}
	}

	// Check if the final response wants to use more tools.
	if finalResponse.StopReason == aiTypes.StopReasonToolUse && len(finalResponse.ToolCalls) > 0 {
		// AI wants to use more tools after seeing the results. Execute them recursively.
		// Accumulate current response so intermediate thinking is preserved.
		newAccumulated := accumulatedContent
		if newAccumulated != "" {
			newAccumulated += "\n\n"
		}
		newAccumulated += resultText
		return m.handleToolExecutionFlowWithAccumulator(ctx, finalResponse, messages, availableTools, newAccumulated)
	}

	// Check if AI expressed intent to take more action in the final response.
	if detectActionIntent(finalResponse.Content) {
		// AI said it would do something else but didn't use tools. Prompt it again.
		messages = append(messages, aiTypes.Message{
			Role:    aiTypes.RoleAssistant,
			Content: finalResponse.Content,
		})
		messages = append(messages, aiTypes.Message{
			Role:    aiTypes.RoleUser,
			Content: "Please use the available tools to perform that action now, rather than just describing what you would do.",
		})

		// Retry with the prompt.
		retryResponse, err := m.client.SendMessageWithToolsAndHistory(ctx, messages, availableTools)
		if err != nil {
			return aiErrorMsg(formatAPIError(err))
		}

		// Handle empty retry response.
		if retryResponse == nil || (retryResponse.Content == "" && retryResponse.StopReason != aiTypes.StopReasonToolUse) {
			// Retry also returned empty - return what we have.
			combinedResponse := accumulatedContent
			if combinedResponse != "" {
				combinedResponse += "\n\n"
			}
			combinedResponse += resultText + "\n\n---\n\n" + finalResponse.Content + "\n\n*Note: AI retry response was empty.*"
			return aiResponseMsg{content: combinedResponse, usage: combineUsage(finalResponse.Usage, retryResponse.Usage)}
		}

		// Check if AI now uses tools.
		if retryResponse.StopReason == aiTypes.StopReasonToolUse && len(retryResponse.ToolCalls) > 0 {
			// Recursively handle the new tool execution.
			// Accumulate current response so intermediate thinking is preserved.
			newAccumulated := accumulatedContent
			if newAccumulated != "" {
				newAccumulated += "\n\n"
			}
			newAccumulated += resultText
			return m.handleToolExecutionFlowWithAccumulator(ctx, retryResponse, messages, availableTools, newAccumulated)
		}

		// If still no tool use, combine all responses.
		combinedResponse := accumulatedContent
		if combinedResponse != "" {
			combinedResponse += "\n\n"
		}
		combinedResponse += resultText + "\n\n---\n\n" + finalResponse.Content + "\n\n" + retryResponse.Content
		return aiResponseMsg{content: combinedResponse, usage: combineUsage(finalResponse.Usage, retryResponse.Usage)}
	}

	// Combine accumulated content + tool execution display + final AI response.
	combinedResponse := accumulatedContent
	if combinedResponse != "" {
		combinedResponse += "\n\n"
	}
	combinedResponse += resultText + "\n\n---\n\n" + finalResponse.Content

	// Return the combined response (accumulated + tool results + AI's final analysis).
	return aiResponseMsg{content: combinedResponse, usage: finalResponse.Usage}
}

// combineUsage combines two Usage structs by adding their token counts.
func combineUsage(u1, u2 *aiTypes.Usage) *aiTypes.Usage {
	if u1 == nil && u2 == nil {
		return nil
	}
	if u1 == nil {
		return u2
	}
	if u2 == nil {
		return u1
	}

	return &aiTypes.Usage{
		InputTokens:         u1.InputTokens + u2.InputTokens,
		OutputTokens:        u1.OutputTokens + u2.OutputTokens,
		TotalTokens:         u1.TotalTokens + u2.TotalTokens,
		CacheReadTokens:     u1.CacheReadTokens + u2.CacheReadTokens,
		CacheCreationTokens: u1.CacheCreationTokens + u2.CacheCreationTokens,
	}
}

// formatTokenCount formats a token count into a human-readable string (e.g., "7.1k").
func formatTokenCount(count int64) string {
	if count == 0 {
		return "0"
	}
	if count < 1000 {
		return fmt.Sprintf("%d", count)
	}
	if count < 1000000 {
		// Format as k (thousands) with one decimal place.
		k := float64(count) / 1000.0
		if k < 10 {
			return fmt.Sprintf("%.1fk", k)
		}
		return fmt.Sprintf("%.0fk", k)
	}
	// Format as M (millions) with one decimal place.
	m := float64(count) / 1000000.0
	if m < 10 {
		return fmt.Sprintf("%.1fM", m)
	}
	return fmt.Sprintf("%.0fM", m)
}

// formatUsage formats usage information for display.
func formatUsage(usage *aiTypes.Usage) string {
	if usage == nil || usage.TotalTokens == 0 {
		return ""
	}

	var parts []string

	// Show input/output breakdown.
	if usage.InputTokens > 0 {
		parts = append(parts, fmt.Sprintf("↑ %s", formatTokenCount(usage.InputTokens)))
	}
	if usage.OutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("↓ %s", formatTokenCount(usage.OutputTokens)))
	}

	// Show cache info if available.
	if usage.CacheReadTokens > 0 {
		parts = append(parts, fmt.Sprintf("cache: %s", formatTokenCount(usage.CacheReadTokens)))
	}

	return strings.Join(parts, " · ")
}

// estimateTokens provides an approximate token count for text using a heuristic approach.
// This uses a simple word-based estimation: tokens ≈ words × 1.3
// This is accurate enough (±10-20%) for rate limit prevention without requiring
// provider-specific tokenizers. More sophisticated approaches could use:
//   - characters / 4 (simpler but less accurate)
//   - word count × 1.5 (more conservative)
//   - tiktoken library (accurate but adds 15MB+ dependency and only works for OpenAI)
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}

	// Count words (split on whitespace).
	words := strings.Fields(text)
	wordCount := len(words)

	// Apply multiplier to estimate tokens.
	// Based on empirical observation:
	// - English text: ~1.3 tokens per word on average
	// - Code: ~1.5 tokens per word (more punctuation)
	// - Technical text: ~1.4 tokens per word
	// We use 1.3 as a reasonable middle ground.
	estimatedTokens := float64(wordCount) * 1.3

	return int(estimatedTokens)
}

// formatAPIError formats API errors in a user-friendly way.
func formatAPIError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// Detect function calling not supported errors (Gemini image generation models).
	if strings.Contains(errStr, "Function calling is not enabled") ||
		(strings.Contains(errStr, "function calling") && (strings.Contains(errStr, "not enabled") || strings.Contains(errStr, "not supported"))) {
		return "This model doesn't support function calling (tool use). Please switch to a different model using Ctrl+P. Try gemini-2.0-flash-exp or gemini-1.5-pro."
	}

	// Detect rate limit errors (429 Too Many Requests).
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "Too Many Requests") ||
		strings.Contains(errStr, "rate_limit_error") {
		return "Rate limit exceeded. Please wait a moment and try again, or contact your provider to increase your rate limit."
	}

	// Detect authentication errors (401 Unauthorized).
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "Unauthorized") ||
		strings.Contains(errStr, "authentication_error") {
		return "Authentication failed. Please check your API key configuration."
	}

	// Detect permission errors (403 Forbidden).
	if strings.Contains(errStr, "403") || strings.Contains(errStr, "Forbidden") ||
		strings.Contains(errStr, "permission_error") {
		return "Permission denied. Your API key may not have access to this model or feature."
	}

	// Detect model errors (404 Not Found).
	if strings.Contains(errStr, "404") || strings.Contains(errStr, "Not Found") ||
		strings.Contains(errStr, "model not found") {
		return "Model not found. Please check your model configuration."
	}

	// Detect timeout errors.
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
		return "Request timed out. The AI provider took too long to respond. Please try again."
	}

	// Detect context length errors.
	if strings.Contains(errStr, "context_length_exceeded") || strings.Contains(errStr, "maximum context length") {
		return "Context length exceeded. Your conversation is too long. Try starting a new session."
	}

	// For other errors, clean up the message by removing technical details.
	// Remove request IDs.
	if idx := strings.Index(errStr, "(Request-ID:"); idx != -1 {
		errStr = strings.TrimSpace(errStr[:idx])
	}
	if idx := strings.Index(errStr, "(request-id:"); idx != -1 {
		errStr = strings.TrimSpace(errStr[:idx])
	}

	// Remove JSON response bodies.
	if idx := strings.Index(errStr, `{"type":"error"`); idx != -1 {
		errStr = strings.TrimSpace(errStr[:idx])
	}

	// Remove HTTP method and URL from error messages (e.g., "POST https://api.example.com: 500 Error").
	// Look for pattern: METHOD "URL": error message
	if strings.HasPrefix(errStr, "POST ") || strings.HasPrefix(errStr, "GET ") ||
		strings.HasPrefix(errStr, "PUT ") || strings.HasPrefix(errStr, "DELETE ") {
		// Find the closing quote and colon after the URL.
		if idx := strings.Index(errStr, `":`); idx != -1 {
			// Skip past the quote and colon, keep the rest.
			errStr = strings.TrimSpace(errStr[idx+2:])
		}
	}

	// Clean up nested error prefixes.
	errStr = strings.TrimPrefix(errStr, "failed to send message: ")
	errStr = strings.TrimPrefix(errStr, "failed to send message with tools: ")
	errStr = strings.TrimPrefix(errStr, "failed to send messages with history: ")
	errStr = strings.TrimPrefix(errStr, "failed to send messages with history and tools: ")

	return errStr
}

// formatToolParameters formats tool call parameters for display in the UI.
func formatToolParameters(toolCall aiTypes.ToolCall) string {
	if len(toolCall.Input) == 0 {
		return ""
	}

	// Special formatting for common tools.
	switch toolCall.Name {
	case "execute_atmos_command":
		if cmd, ok := toolCall.Input["command"].(string); ok {
			return fmt.Sprintf("**Command:** `atmos %s`", cmd)
		}
	case "read_file", "read_component_file", "read_stack_file":
		if path, ok := toolCall.Input["path"].(string); ok {
			return fmt.Sprintf("**Path:** `%s`", path)
		}
		if component, ok := toolCall.Input["component"].(string); ok {
			return fmt.Sprintf("**Component:** `%s`", component)
		}
	case "edit_file", "write_component_file", "write_stack_file":
		if path, ok := toolCall.Input["path"].(string); ok {
			return fmt.Sprintf("**Path:** `%s`", path)
		}
		if component, ok := toolCall.Input["component"].(string); ok {
			return fmt.Sprintf("**Component:** `%s`", component)
		}
	case "search_files":
		if pattern, ok := toolCall.Input["pattern"].(string); ok {
			return fmt.Sprintf("**Pattern:** `%s`", pattern)
		}
	case "execute_bash":
		if command, ok := toolCall.Input["command"].(string); ok {
			// Truncate long commands for display.
			if len(command) > 80 {
				command = command[:77] + "..."
			}
			return fmt.Sprintf("**Command:** `%s`", command)
		}
	case "describe_component":
		parts := []string{}
		if component, ok := toolCall.Input["component"].(string); ok {
			parts = append(parts, component)
		}
		if stack, ok := toolCall.Input["stack"].(string); ok {
			parts = append(parts, "-s "+stack)
		}
		if len(parts) > 0 {
			return fmt.Sprintf("**Args:** `%s`", strings.Join(parts, " "))
		}
	}

	// For other tools, show all parameters in a generic format.
	var params []string
	for key, value := range toolCall.Input {
		// Format value based on type.
		var valueStr string
		switch v := value.(type) {
		case string:
			valueStr = v
		case bool:
			valueStr = fmt.Sprintf("%v", v)
		case float64, int:
			valueStr = fmt.Sprintf("%v", v)
		default:
			valueStr = fmt.Sprintf("%v", v)
		}

		// Truncate long values.
		if len(valueStr) > 50 {
			valueStr = valueStr[:47] + "..."
		}

		params = append(params, fmt.Sprintf("%s=`%s`", key, valueStr))
	}

	if len(params) > 0 {
		return fmt.Sprintf("**Parameters:** %s", strings.Join(params, ", "))
	}

	return ""
}

// detectActionIntent checks if the AI response contains phrases indicating intent to take action.
// Returns true if the AI says it will do something but didn't use tools.
func detectActionIntent(content string) bool {
	contentLower := strings.ToLower(content)

	// Phrases that indicate the AI intends to take action.
	actionPhrases := []string{
		"i'll",
		"i will",
		"let me",
		"i'm going to",
		"i am going to",
		"now i'll",
		"now i will",
		"first, i'll",
		"first, i will",
		"i'll now",
		"i will now",
	}

	// Check if content contains any action phrase.
	hasActionPhrase := false
	for _, phrase := range actionPhrases {
		if strings.Contains(contentLower, phrase) {
			hasActionPhrase = true
			break
		}
	}

	if !hasActionPhrase {
		return false
	}

	// Action verbs that indicate the AI is about to perform an action.
	actionVerbs := []string{
		"read",
		"edit",
		"fix",
		"update",
		"modify",
		"change",
		"create",
		"delete",
		"search",
		"find",
		"execute",
		"run",
		"check",
		"validate",
		"describe",
		"list",
		"use",      // "I will use the tool"
		"start",    // "I will start by describing"
		"begin",    // "I will begin by trying"
		"try",      // "I will try common names"
		"call",     // "I will call the API"
		"invoke",   // "I will invoke the function"
		"get",      // "I will get the data"
		"fetch",    // "I will fetch the results"
		"retrieve", // "I will retrieve the information"
		"query",    // "I will query the database"
	}

	// Check if any action verb appears anywhere in the content after the action phrase.
	// This is more flexible than requiring immediate adjacency.
	for _, verb := range actionVerbs {
		// Look for the verb as a separate word (with word boundaries).
		// Match patterns like " use ", " use.", " use,", etc.
		if strings.Contains(contentLower, " "+verb+" ") ||
			strings.Contains(contentLower, " "+verb+".") ||
			strings.Contains(contentLower, " "+verb+",") ||
			strings.HasPrefix(contentLower, verb+" ") ||
			strings.HasSuffix(contentLower, " "+verb) {
			return true
		}
	}

	return false
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

// getAgentIcon returns the icon/emoji for a given agent.
func getAgentIcon(agentName string) string {
	icons := map[string]string{
		"general":            "🤖", // Robot - general purpose
		"stack-analyzer":     "📊", // Chart - data analysis
		"component-refactor": "🔧", // Wrench - fixing/building
		"security-auditor":   "🔒", // Lock - security
		"config-validator":   "✅", // Checkmark - validation
	}

	if icon, ok := icons[agentName]; ok {
		return icon
	}
	return "🤖" // Default to robot icon
}

// agentSelectView renders the agent selection interface.
func (m *ChatModel) agentSelectView() string {
	var content strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan)).
		MarginBottom(1)
	content.WriteString(titleStyle.Render("Switch AI Agent"))
	content.WriteString(newlineChar)

	// Help text
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGray)).
		Margin(0, 0, 1, 0)
	content.WriteString(helpStyle.Render("↑/↓: Navigate | Enter: Select | Esc/q: Cancel"))
	content.WriteString(doubleNewline)

	// Current agent indicator
	currentAgentName := ""
	if m.currentAgent != nil {
		currentAgentName = m.currentAgent.Name
	}

	// Agent list
	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan)).
		Background(lipgloss.Color(theme.ColorGray))
	normalStyle := lipgloss.NewStyle()
	currentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGreen))

	// Get all available agents
	availableAgents := m.agentRegistry.List()

	for i, agent := range availableAgents {
		var line string
		prefix := "  "
		if i == m.selectedAgentIdx {
			prefix = "▶ "
		}

		// Add agent icon to display name
		agentIcon := getAgentIcon(agent.Name)
		agentInfo := fmt.Sprintf("%s%s %s", prefix, agentIcon, agent.DisplayName)
		if agent.Name == currentAgentName {
			agentInfo += " (current)"
		}
		if agent.IsBuiltIn {
			agentInfo += " [built-in]"
		}
		agentInfo += fmt.Sprintf("\n    %s", agent.Description)

		if i == m.selectedAgentIdx {
			line = selectedStyle.Render(agentInfo)
		} else if agent.Name == currentAgentName {
			line = currentStyle.Render(agentInfo)
		} else {
			line = normalStyle.Render(agentInfo)
		}

		content.WriteString(line)
		content.WriteString(doubleNewline)
	}

	return content.String()
}
