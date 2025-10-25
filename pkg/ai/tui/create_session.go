package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/session"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// Form field constants.
	sessionNameCharLimit = 100
	sessionNameWidth     = 50
)

// createSessionForm holds the state of the create session form.
type createSessionForm struct {
	nameInput        textinput.Model
	selectedProvider int
	error            string
	focusedField     int // 0 = name, 1 = provider
}

// newCreateSessionForm creates a new create session form.
func newCreateSessionForm() createSessionForm {
	ti := textinput.New()
	ti.Placeholder = "Enter session name"
	ti.Focus()
	ti.CharLimit = sessionNameCharLimit
	ti.Width = sessionNameWidth

	return createSessionForm{
		nameInput:        ti,
		selectedProvider: 0, // Default to Anthropic
		focusedField:     0, // Start with name input focused
	}
}

// sessionCreatedMsg is sent when a new session has been created.
type sessionCreatedMsg struct {
	session *session.Session
	err     error
}

// handleCreateSessionKeys processes keyboard input for the create session form.
//
//revive:disable:cyclomatic // TUI keyboard handlers naturally have high complexity.
func (m *ChatModel) handleCreateSessionKeys(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc":
		// Cancel and return to session list or chat
		if m.manager != nil {
			m.currentView = viewModeSessionList
			return m.loadSessionList()
		}
		m.currentView = viewModeChat
		m.textarea.Focus()
		// Return empty command to consume the key event and prevent it from reaching the textarea
		return func() tea.Msg { return nil }
	case "tab", "shift+tab":
		// Toggle focus between name input and provider selection
		if m.createForm.focusedField == 0 {
			m.createForm.focusedField = 1
			m.createForm.nameInput.Blur()
		} else {
			m.createForm.focusedField = 0
			m.createForm.nameInput.Focus()
		}
		return func() tea.Msg { return nil }
	case "up", "k":
		// If name field is focused, switch to provider field first
		if m.createForm.focusedField == 0 {
			m.createForm.focusedField = 1
			m.createForm.nameInput.Blur()
		}
		// Navigate provider selection up with wraparound
		configuredProviders := m.getConfiguredProvidersForCreate()
		if m.createForm.selectedProvider > 0 {
			m.createForm.selectedProvider--
		} else if len(configuredProviders) > 0 {
			m.createForm.selectedProvider = len(configuredProviders) - 1
		}
		return func() tea.Msg { return nil }
	case "down", "j":
		// If name field is focused, switch to provider field first
		if m.createForm.focusedField == 0 {
			m.createForm.focusedField = 1
			m.createForm.nameInput.Blur()
		}
		// Navigate provider selection down with wraparound
		configuredProviders := m.getConfiguredProvidersForCreate()
		if m.createForm.selectedProvider < len(configuredProviders)-1 {
			m.createForm.selectedProvider++
		} else if len(configuredProviders) > 0 {
			m.createForm.selectedProvider = 0
		}
		return func() tea.Msg { return nil }
	case "enter":
		// Submit form
		return m.submitCreateSession()
	}

	// Update name input if focused
	if m.createForm.focusedField == 0 {
		var cmd tea.Cmd
		m.createForm.nameInput, cmd = m.createForm.nameInput.Update(msg)
		return cmd
	}

	return func() tea.Msg { return nil }
}

// submitCreateSession validates and submits the create session form.
func (m *ChatModel) submitCreateSession() tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return sessionCreatedMsg{err: errUtils.ErrAISessionManagerNotAvailable}
		}

		// Validate name
		name := strings.TrimSpace(m.createForm.nameInput.Value())
		if name == "" {
			return sessionCreatedMsg{err: errUtils.ErrAISessionNameEmpty}
		}

		// Get selected provider from atmos.yaml configuration
		configuredProviders := m.getConfiguredProvidersForCreate()
		if m.createForm.selectedProvider >= len(configuredProviders) {
			return sessionCreatedMsg{err: fmt.Errorf("invalid provider selection")}
		}
		provider := configuredProviders[m.createForm.selectedProvider]

		// Create session
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		sess, err := m.manager.CreateSession(
			ctx,
			name,
			provider.Model, // Use model from atmos.yaml
			provider.Name,
			nil, // metadata
		)
		if err != nil {
			return sessionCreatedMsg{err: fmt.Errorf("failed to create session: %w", err)}
		}

		return sessionCreatedMsg{session: sess}
	}
}

// handleSessionCreated processes the session created message.
func (m *ChatModel) handleSessionCreated(msg sessionCreatedMsg) {
	if msg.err != nil {
		m.createForm.error = msg.err.Error()
	} else {
		// Switch to the newly created session
		m.sess = msg.session
		m.messages = make([]ChatMessage, 0)
		m.updateViewportContent()
		m.currentView = viewModeChat
		m.textarea.Focus()
		m.createForm = newCreateSessionForm() // Reset form

		// Add welcome message
		m.addMessage(roleAssistant, `I'm here to help you with your Atmos infrastructure management. I can:

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
		m.updateViewportContent()
	}
}

// createSessionView renders the create session form.
//
//nolint:funlen // TUI rendering functions require detailed styling.
//revive:disable:function-length
func (m *ChatModel) createSessionView() string {
	var content strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorCyan)).
		Bold(true).
		Padding(1, 2)

	content.WriteString(titleStyle.Render("Create New Session"))
	content.WriteString(newlineChar + newlineChar)

	// Error message if any
	if m.createForm.error != "" {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.ColorRed)).
			Padding(0, 2)
		content.WriteString(errorStyle.Render(fmt.Sprintf("❌ Error: %s", m.createForm.error)))
		content.WriteString(newlineChar + newlineChar)
	}

	// Name input
	nameLabel := "Session Name:"
	if m.createForm.focusedField == 0 {
		nameLabel = "→ Session Name:"
	}

	labelStyle := lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(lipgloss.Color(theme.ColorGreen))

	content.WriteString(labelStyle.Render(nameLabel))
	content.WriteString(newlineChar)
	content.WriteString(lipgloss.NewStyle().Padding(0, 4).Render(m.createForm.nameInput.View()))
	content.WriteString(newlineChar + newlineChar)

	// Provider selection
	providerLabel := "Provider:"
	if m.createForm.focusedField == 1 {
		providerLabel = "→ Provider:"
	}

	content.WriteString(labelStyle.Render(providerLabel))
	content.WriteString(newlineChar)

	// Render provider options from atmos.yaml configuration
	configuredProviders := m.getConfiguredProvidersForCreate()
	for i, provider := range configuredProviders {
		var style lipgloss.Style
		var prefix string

		if i == m.createForm.selectedProvider {
			if m.createForm.focusedField == 1 {
				style = lipgloss.NewStyle().
					Foreground(lipgloss.Color(theme.ColorGreen)).
					Bold(true).
					Padding(0, 4)
				prefix = "● "
			} else {
				style = lipgloss.NewStyle().
					Foreground(lipgloss.Color(theme.ColorCyan)).
					Padding(0, 4)
				prefix = "● "
			}
		} else {
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Padding(0, 4)
			prefix = "○ "
		}

		modelInfo := fmt.Sprintf("%s%s (%s)", prefix, provider.DisplayName, provider.Model)
		content.WriteString(style.Render(modelInfo))
		content.WriteString(newlineChar)
	}

	content.WriteString(newlineChar)

	// Help text
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		Padding(0, 2)

	help := "Tab: Switch field | ↑/↓: Select provider | Enter: Create | Esc: Cancel | Ctrl+C: Quit"
	content.WriteString(helpStyle.Render(help))

	return content.String()
}
