package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/instructions"
	"github.com/cloudposse/atmos/pkg/ai/session"
	"github.com/cloudposse/atmos/pkg/ai/skills"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// ChatOptions holds all parameters needed to start a chat session.
type ChatOptions struct {
	Client      ai.Client
	AtmosConfig *schema.AtmosConfiguration
	Manager     *session.Manager
	Session     *session.Session
	Executor    *tools.Executor
	MemoryMgr   *instructions.Manager
}

// RunChat starts the chat TUI with the provided options.
func RunChat(opts ChatOptions) error {
	model, err := NewChatModel(ChatModelParams(opts))
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
		if opts.Session != nil {
			sessionName = opts.Session.Name
		}
		model.addMessage(roleSystem, fmt.Sprintf("Resumed session: %s (%d messages)", sessionName, len(model.messages)))
	}

	model.updateViewportContent()

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // Enable mouse wheel scrolling.
	)

	// Store program reference for sending messages from callbacks.
	model.program = p

	_, err = p.Run()
	return err
}

// getConfiguredProviders returns only the providers that are configured in atmos.yaml.
func (m *ChatModel) getConfiguredProviders() []struct {
	Name        string
	Description string
} {
	if m.atmosConfig == nil || m.atmosConfig.AI.Providers == nil {
		return availableProviders
	}

	configured := make([]struct {
		Name        string
		Description string
	}, 0)

	for _, provider := range availableProviders {
		// Check if this provider is configured.
		if _, exists := m.atmosConfig.AI.Providers[provider.Name]; exists {
			configured = append(configured, provider)
		}
	}

	// If no providers are configured, show all (fallback for backward compatibility).
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

// providerDisplayNames maps provider names to their display names.
var providerDisplayNames = map[string]string{
	providerAnthropic: "Anthropic (Claude)",
	"openai":          "OpenAI (GPT)",
	"gemini":          "Google (Gemini)",
	"grok":            "xAI (Grok)",
	"ollama":          "Ollama (Local)",
	"bedrock":         "AWS Bedrock",
	"azureopenai":     "Azure OpenAI",
}

// defaultProviderModels is the fallback list when no providers are configured.
var defaultProviderModels = []ProviderWithModel{
	{Name: providerAnthropic, DisplayName: "Anthropic (Claude)", Model: "claude-sonnet-4-5-20250929"},
	{Name: "openai", DisplayName: "OpenAI (GPT)", Model: defaultOpenAIModel},
	{Name: "gemini", DisplayName: "Google (Gemini)", Model: "gemini-2.5-flash"},
	{Name: "grok", DisplayName: "xAI (Grok)", Model: "grok-4"},
	{Name: "ollama", DisplayName: "Ollama (Local)", Model: "llama3.3:70b"},
	{Name: "bedrock", DisplayName: "AWS Bedrock", Model: "anthropic.claude-sonnet-4-5-20250929-v1:0"},
	{Name: "azureopenai", DisplayName: "Azure OpenAI", Model: defaultOpenAIModel},
}

// getConfiguredProvidersForCreate returns configured providers with their models from atmos.yaml.
func (m *ChatModel) getConfiguredProvidersForCreate() []ProviderWithModel {
	if m.atmosConfig == nil || m.atmosConfig.AI.Providers == nil {
		return defaultProviderModels
	}

	configured := make([]ProviderWithModel, 0)

	for _, provider := range availableProviders {
		providerConfig, exists := m.atmosConfig.AI.Providers[provider.Name]
		if !exists {
			continue
		}

		displayName := providerDisplayNames[provider.Name]
		if displayName == "" {
			displayName = provider.Name
		}

		configured = append(configured, ProviderWithModel{
			Name:        provider.Name,
			DisplayName: displayName,
			Model:       providerConfig.Model,
		})
	}

	// If no providers are configured, return all with defaults.
	if len(configured) == 0 {
		return defaultProviderModels
	}

	return configured
}

// providerSelectView renders the provider selection interface.
func (m *ChatModel) providerSelectView() string {
	var content strings.Builder

	// Title.
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan)).
		MarginBottom(1)
	content.WriteString(titleStyle.Render("Switch AI Provider"))
	content.WriteString(newlineChar)

	// Help text.
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGray)).
		Margin(0, 0, 1, 0)
	content.WriteString(helpStyle.Render("\u2191/\u2193: Navigate | Enter: Select | Esc/q: Cancel"))
	content.WriteString(doubleNewline)

	// Determine current provider.
	currentProvider := m.getCurrentProvider()

	// Render provider list.
	configuredProviders := m.getConfiguredProviders()
	for i, provider := range configuredProviders {
		line := m.renderProviderLine(i, provider.Name, provider.Description, currentProvider)
		content.WriteString(line)
		content.WriteString(doubleNewline)
	}

	return content.String()
}

// getCurrentProvider returns the name of the currently active provider.
func (m *ChatModel) getCurrentProvider() string {
	switch {
	case m.sess != nil && m.sess.Provider != "":
		return m.sess.Provider
	case m.atmosConfig.AI.DefaultProvider != "":
		return m.atmosConfig.AI.DefaultProvider
	default:
		return providerAnthropic
	}
}

// renderProviderLine renders a single provider entry in the selection list.
func (m *ChatModel) renderProviderLine(index int, name, description, currentProvider string) string {
	prefix := "  "
	if index == m.selectedProviderIdx {
		prefix = "\u25b6 "
	}

	providerInfo := fmt.Sprintf("%s%s", prefix, name)
	if name == currentProvider {
		providerInfo += " (current)"
	}
	providerInfo += fmt.Sprintf("\n    %s", description)

	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan)).
		Background(lipgloss.Color(theme.ColorGray))
	currentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGreen))
	normalStyle := lipgloss.NewStyle()

	switch {
	case index == m.selectedProviderIdx:
		return selectedStyle.Render(providerInfo)
	case name == currentProvider:
		return currentStyle.Render(providerInfo)
	default:
		return normalStyle.Render(providerInfo)
	}
}

// getSkillIcon returns the icon/emoji for a given skill.
func getSkillIcon(skillName string) string {
	icons := map[string]string{
		"general":            "\U0001f916", // Robot - general purpose.
		"stack-analyzer":     "\U0001f4ca", // Chart - data analysis.
		"component-refactor": "\U0001f527", // Wrench - fixing/building.
		"security-auditor":   "\U0001f512", // Lock - security.
		"config-validator":   "\u2705",     // Checkmark - validation.
	}

	if icon, ok := icons[skillName]; ok {
		return icon
	}
	return "\U0001f916" // Default to robot icon.
}

// skillSelectView renders the skill selection interface.
func (m *ChatModel) skillSelectView() string {
	var content strings.Builder

	// Title.
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan)).
		MarginBottom(1)
	content.WriteString(titleStyle.Render("Switch AI Skill"))
	content.WriteString(newlineChar)

	// Help text.
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGray)).
		Margin(0, 0, 1, 0)
	content.WriteString(helpStyle.Render("\u2191/\u2193: Navigate | Enter: Select | Esc/q: Cancel"))
	content.WriteString(doubleNewline)

	// Current skill indicator.
	currentSkillName := ""
	if m.currentSkill != nil {
		currentSkillName = m.currentSkill.Name
	}

	// Render skill list.
	availableSkills := m.skillRegistry.List()
	for i, skill := range availableSkills {
		line := m.renderSkillLine(i, skill, currentSkillName)
		content.WriteString(line)
		content.WriteString(doubleNewline)
	}

	return content.String()
}

// renderSkillLine renders a single skill entry in the selection list.
func (m *ChatModel) renderSkillLine(index int, skill *skills.Skill, currentSkillName string) string {
	prefix := "  "
	if index == m.selectedSkillIdx {
		prefix = "\u25b6 "
	}

	skillIcon := getSkillIcon(skill.Name)
	skillInfo := fmt.Sprintf("%s%s %s", prefix, skillIcon, skill.DisplayName)
	if skill.Name == currentSkillName {
		skillInfo += " (current)"
	}
	if skill.IsBuiltIn {
		skillInfo += " [built-in]"
	}
	skillInfo += fmt.Sprintf("\n    %s", skill.Description)

	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan)).
		Background(lipgloss.Color(theme.ColorGray))
	currentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGreen))
	normalStyle := lipgloss.NewStyle()

	switch {
	case index == m.selectedSkillIdx:
		return selectedStyle.Render(skillInfo)
	case skill.Name == currentSkillName:
		return currentStyle.Render(skillInfo)
	default:
		return normalStyle.Render(skillInfo)
	}
}
