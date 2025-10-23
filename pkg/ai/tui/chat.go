package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/memory"
	"github.com/cloudposse/atmos/pkg/ai/session"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// DefaultViewportWidth is the default width for the chat viewport before window sizing.
	DefaultViewportWidth = 80
	// DefaultViewportHeight is the default height for the chat viewport before window sizing.
	DefaultViewportHeight = 20

	// Message roles.
	roleUser      = "user"
	roleAssistant = "assistant"
	roleSystem    = "system"
)

// viewMode represents the current view mode of the TUI.
type viewMode int

const (
	viewModeChat viewMode = iota
	viewModeSessionList
)

// ChatModel represents the state of the chat TUI.
type ChatModel struct {
	client               ai.Client
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
}

// ChatMessage represents a single message in the chat.
type ChatMessage struct {
	Role    string // "user" or "assistant"
	Content string
	Time    time.Time
}

// NewChatModel creates a new chat model with the provided AI client.
func NewChatModel(client ai.Client, manager *session.Manager, sess *session.Session, executor *tools.Executor, memoryMgr *memory.Manager) (*ChatModel, error) {
	if client == nil {
		return nil, errUtils.ErrAIClientNil
	}

	// Initialize viewport.
	vp := viewport.New(DefaultViewportWidth, DefaultViewportHeight)
	vp.SetContent("")

	// Initialize textarea.
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Enter to send, Shift+Enter for new line, Ctrl+C to quit)"
	ta.Focus()
	ta.ShowLineNumbers = false
	ta.CharLimit = 0 // No character limit

	// Initialize spinner.
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))

	model := &ChatModel{
		client:               client,
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

	// Convert session messages to chat messages.
	for _, msg := range sessionMessages {
		m.messages = append(m.messages, ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
			Time:    msg.CreatedAt,
		})
	}

	return nil
}

// Init initializes the chat model.
func (m *ChatModel) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
	)
}

// handleWindowResize processes window size changes and adjusts UI components.
func (m *ChatModel) handleWindowResize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height

	headerHeight := lipgloss.Height(m.headerView())
	footerHeight := lipgloss.Height(m.footerView())
	verticalMarginHeight := headerHeight + footerHeight

	if !m.ready {
		// Initialize viewport and textarea sizes.
		m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight-4) // -4 for textarea
		m.viewport.YPosition = headerHeight + 1
		m.textarea.SetWidth(msg.Width - 4)
		m.textarea.SetHeight(3)
		m.ready = true
	} else {
		// Adjust existing sizes.
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - verticalMarginHeight - 4
		m.textarea.SetWidth(msg.Width - 4)
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
	}

	// Update viewport.
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// handleMessage processes different message types and returns whether it was handled.
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
	if m.currentView == viewModeSessionList {
		return m.sessionListView()
	}

	return fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.viewport.View(), m.footerView())
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

		help := helpStyle.Render("Ctrl+L: Sessions | Ctrl+C: Quit")
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

		// Wrap content to viewport width
		contentStyle := lipgloss.NewStyle().
			PaddingLeft(2).
			Width(m.viewport.Width - 4)

		contentParts = append(contentParts, header)
		contentParts = append(contentParts, contentStyle.Render(msg.Content))
		contentParts = append(contentParts, "") // Empty line between messages
	}

	m.viewport.SetContent(strings.Join(contentParts, "\n"))
	m.viewport.GotoBottom()
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

		response, err := m.client.SendMessage(ctx, messageWithContext)
		if err != nil {
			return aiErrorMsg(err.Error())
		}

		return aiResponseMsg(response)
	}
}

// RunChat starts the chat TUI with the provided AI client.
func RunChat(client ai.Client, manager *session.Manager, sess *session.Session, executor *tools.Executor, memoryMgr *memory.Manager) error {
	model, err := NewChatModel(client, manager, sess, executor, memoryMgr)
	if err != nil {
		return fmt.Errorf("failed to create chat model: %w", err)
	}

	// Add welcome message only if this is a new session (no existing messages).
	if len(model.messages) == 0 {
		model.addMessage(roleAssistant, `Welcome to Atmos AI Assistant! 🚀

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

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
