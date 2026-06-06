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
	focusedField     int // 0 = name, 1 = provider.
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
		selectedProvider: 0, // Default to Anthropic.
		focusedField:     0, // Start with name input focused.
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
		return m.cancelCreateSession()
	case "tab", "shift+tab":
		m.toggleCreateFormFocus()
		return noopCmd()
	case "up", "k":
		m.navigateCreateFormProvider(-1)
		return noopCmd()
	case "down", "j":
		m.navigateCreateFormProvider(1)
		return noopCmd()
	case "enter":
		return m.submitCreateSession()
	}

	// Update name input if focused.
	if m.createForm.focusedField == 0 {
		var cmd tea.Cmd
		m.createForm.nameInput, cmd = m.createForm.nameInput.Update(msg)
		return cmd
	}

	return noopCmd()
}

// cancelCreateSession returns to the previous view.
func (m *ChatModel) cancelCreateSession() tea.Cmd {
	if m.manager != nil {
		m.currentView = viewModeSessionList
		return m.loadSessionList()
	}
	m.currentView = viewModeChat
	m.textarea.Focus()
	return noopCmd()
}

// toggleCreateFormFocus toggles between name input and provider selection.
func (m *ChatModel) toggleCreateFormFocus() {
	if m.createForm.focusedField == 0 {
		m.createForm.focusedField = 1
		m.createForm.nameInput.Blur()
	} else {
		m.createForm.focusedField = 0
		m.createForm.nameInput.Focus()
	}
}

// navigateCreateFormProvider moves the provider selection in the given direction.
func (m *ChatModel) navigateCreateFormProvider(direction int) {
	// If name field is focused, switch to provider field first.
	if m.createForm.focusedField == 0 {
		m.createForm.focusedField = 1
		m.createForm.nameInput.Blur()
	}

	configuredProviders := m.getConfiguredProvidersForCreate()
	if len(configuredProviders) == 0 {
		return
	}

	m.createForm.selectedProvider += direction
	if m.createForm.selectedProvider < 0 {
		m.createForm.selectedProvider = len(configuredProviders) - 1
	} else if m.createForm.selectedProvider >= len(configuredProviders) {
		m.createForm.selectedProvider = 0
	}
}

// submitCreateSession validates and submits the create session form.
func (m *ChatModel) submitCreateSession() tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return sessionCreatedMsg{err: errUtils.ErrAISessionManagerNotAvailable}
		}

		// Validate name.
		name := strings.TrimSpace(m.createForm.nameInput.Value())
		if name == "" {
			return sessionCreatedMsg{err: errUtils.ErrAISessionNameEmpty}
		}

		// Get selected provider from atmos.yaml configuration.
		configuredProviders := m.getConfiguredProvidersForCreate()
		if m.createForm.selectedProvider >= len(configuredProviders) {
			return sessionCreatedMsg{err: errUtils.ErrAIInvalidProviderSelection}
		}
		provider := configuredProviders[m.createForm.selectedProvider]

		// Get current skill name (default to empty string if not set).
		skillName := ""
		if m.currentSkill != nil {
			skillName = m.currentSkill.Name
		}

		// Create session.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		sess, err := m.manager.CreateSession(
			ctx,
			session.CreateSessionParams{
				Name:     name,
				Model:    provider.Model,
				Provider: provider.Name,
				Skill:    skillName,
			},
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
		return
	}

	// Switch to the newly created session.
	m.sess = msg.session
	m.messages = make([]ChatMessage, 0)
	m.updateViewportContent()
	m.currentView = viewModeChat
	m.textarea.Focus()
	m.createForm = newCreateSessionForm()

	// Add welcome message.
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

// createSessionView renders the create session form.
func (m *ChatModel) createSessionView() string {
	var content strings.Builder

	m.renderCreateFormTitle(&content)
	m.renderCreateFormError(&content)
	m.renderCreateFormNameInput(&content)
	m.renderCreateFormProviderList(&content)
	m.renderCreateFormHelp(&content)

	return content.String()
}

// renderCreateFormTitle renders the form title.
func (m *ChatModel) renderCreateFormTitle(content *strings.Builder) {
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorCyan)).
		Bold(true).
		Padding(1, 2)
	content.WriteString(titleStyle.Render("Create New Session"))
	content.WriteString(newlineChar + newlineChar)
}

// renderCreateFormError renders the error message if present.
func (m *ChatModel) renderCreateFormError(content *strings.Builder) {
	if m.createForm.error == "" {
		return
	}

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorRed)).
		Padding(0, 2)
	content.WriteString(errorStyle.Render(fmt.Sprintf("\u274c Error: %s", m.createForm.error)))
	content.WriteString(newlineChar + newlineChar)
}

// renderCreateFormNameInput renders the name input field.
func (m *ChatModel) renderCreateFormNameInput(content *strings.Builder) {
	nameLabel := "Session Name:"
	if m.createForm.focusedField == 0 {
		nameLabel = "\u2192 Session Name:"
	}

	labelStyle := lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(lipgloss.Color(theme.ColorGreen))

	content.WriteString(labelStyle.Render(nameLabel))
	content.WriteString(newlineChar)
	content.WriteString(lipgloss.NewStyle().Padding(0, 4).Render(m.createForm.nameInput.View()))
	content.WriteString(newlineChar + newlineChar)
}

// renderCreateFormProviderList renders the provider selection list.
func (m *ChatModel) renderCreateFormProviderList(content *strings.Builder) {
	providerLabel := "Provider:"
	if m.createForm.focusedField == 1 {
		providerLabel = "\u2192 Provider:"
	}

	labelStyle := lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(lipgloss.Color(theme.ColorGreen))

	content.WriteString(labelStyle.Render(providerLabel))
	content.WriteString(newlineChar)

	configuredProviders := m.getConfiguredProvidersForCreate()
	for i, provider := range configuredProviders {
		line := m.renderCreateFormProviderOption(i, provider)
		content.WriteString(line)
		content.WriteString(newlineChar)
	}

	content.WriteString(newlineChar)
}

// renderCreateFormProviderOption renders a single provider option in the create form.
func (m *ChatModel) renderCreateFormProviderOption(index int, provider ProviderWithModel) string {
	var style lipgloss.Style
	prefix := "\u25cb "

	if index == m.createForm.selectedProvider {
		prefix = "\u25cf "
		if m.createForm.focusedField == 1 {
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.ColorGreen)).
				Bold(true).
				Padding(0, 4)
		} else {
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.ColorCyan)).
				Padding(0, 4)
		}
	} else {
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 4)
	}

	modelInfo := fmt.Sprintf("%s%s (%s)", prefix, provider.DisplayName, provider.Model)
	return style.Render(modelInfo)
}

// renderCreateFormHelp renders the help text.
func (m *ChatModel) renderCreateFormHelp(content *strings.Builder) {
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		Padding(0, 2)

	help := "Tab: Switch field | \u2191/\u2193: Select provider | Enter: Create | Esc: Cancel | Ctrl+C: Quit"
	content.WriteString(helpStyle.Render(help))
}
