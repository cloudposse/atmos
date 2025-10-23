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
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// DefaultViewportWidth is the default width for the chat viewport before window sizing.
	DefaultViewportWidth = 80
	// DefaultViewportHeight is the default height for the chat viewport before window sizing.
	DefaultViewportHeight = 20
)

// ChatModel represents the state of the chat TUI.
type ChatModel struct {
	client    ai.Client
	messages  []ChatMessage
	viewport  viewport.Model
	textarea  textarea.Model
	spinner   spinner.Model
	isLoading bool
	width     int
	height    int
	ready     bool
}

// ChatMessage represents a single message in the chat.
type ChatMessage struct {
	Role    string // "user" or "assistant"
	Content string
	Time    time.Time
}

// NewChatModel creates a new chat model with the provided AI client.
func NewChatModel(client ai.Client) (*ChatModel, error) {
	if client == nil {
		return nil, errUtils.ErrAIClientNil
	}

	// Initialize viewport
	vp := viewport.New(DefaultViewportWidth, DefaultViewportHeight)
	vp.SetContent("")

	// Initialize textarea
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Ctrl+C to quit, Enter to send)"
	ta.Focus()

	// Initialize spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))

	return &ChatModel{
		client:    client,
		messages:  make([]ChatMessage, 0),
		viewport:  vp,
		textarea:  ta,
		spinner:   s,
		isLoading: false,
	}, nil
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

// handleKeyMsg processes keyboard input.
func (m *ChatModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.isLoading {
		// Only allow quitting while loading.
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		if strings.TrimSpace(m.textarea.Value()) != "" {
			return m, m.sendMessage(m.textarea.Value())
		}
	}

	return m, nil
}

// Update handles messages and updates the model state.
func (m *ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.handleWindowResize(msg)

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case sendMessageMsg:
		// Add user message
		m.addMessage("user", string(msg))
		m.textarea.Reset()
		m.isLoading = true
		m.updateViewportContent()
		return m, tea.Batch(
			m.spinner.Tick,
			m.getAIResponse(string(msg)),
		)

	case aiResponseMsg:
		// Add AI response
		m.addMessage("assistant", string(msg))
		m.isLoading = false
		m.updateViewportContent()

	case aiErrorMsg:
		// Handle AI error
		m.addMessage("system", fmt.Sprintf("Error: %s", string(msg)))
		m.isLoading = false
		m.updateViewportContent()

	case spinner.TickMsg:
		if m.isLoading {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Update textarea only if not loading
	if !m.isLoading {
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update viewport
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the chat interface.
func (m *ChatModel) View() string {
	if !m.ready {
		return "\n  Initializing Atmos AI Chat..."
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

	return lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render(title),
		subtitleStyle.Render(subtitle),
	)
}

func (m *ChatModel) footerView() string {
	var content string

	if m.isLoading {
		content = fmt.Sprintf("%s AI is thinking...", m.spinner.View())
	} else {
		content = m.textarea.View()
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
}

func (m *ChatModel) updateViewportContent() {
	var contentParts []string

	for _, msg := range m.messages {
		var style lipgloss.Style
		var prefix string

		switch msg.Role {
		case "user":
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.ColorGreen)).
				Bold(true)
			prefix = "You:"
		case "assistant":
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.ColorCyan))
			prefix = "Atmos AI:"
		case "system":
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

		response, err := m.client.SendMessage(ctx, userMessage)
		if err != nil {
			return aiErrorMsg(err.Error())
		}

		return aiResponseMsg(response)
	}
}

// RunChat starts the chat TUI with the provided AI client.
func RunChat(client ai.Client) error {
	model, err := NewChatModel(client)
	if err != nil {
		return fmt.Errorf("failed to create chat model: %w", err)
	}

	// Add welcome message
	model.addMessage("assistant", `Welcome to Atmos AI Assistant! 🚀

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

	model.updateViewportContent()

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
