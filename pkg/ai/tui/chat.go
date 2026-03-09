package tui

import (
	"context"
	"fmt"
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
	"github.com/cloudposse/atmos/pkg/ai/instructions"
	"github.com/cloudposse/atmos/pkg/ai/session"
	"github.com/cloudposse/atmos/pkg/ai/skills"
	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	aiTypes "github.com/cloudposse/atmos/pkg/ai/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/version"
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

	// Layout constants.
	minViewportHeight = 10

	// Mouse click boundary for create session form fields.
	createFormNameFieldMaxRow = 6

	// Content detection constants.
	yamlDetectionMaxLines  = 10
	tablePipeCheckMaxLines = 5
	pipeChar               = "|"

	// Token formatting constants.
	tokensPerK            = 1000
	tokensPerM            = 1000000
	tokensPerKF           = 1000.0
	tokensPerMF           = 1000000.0
	tokenDisplayThreshold = 10

	// Token estimation multiplier (average tokens per word for English text).
	tokenEstimationMultiplier = 1.3

	// Display truncation constants.
	commandDisplayMaxLen = 80
	commandTruncatedLen  = 77
	valueDisplayMaxLen   = 50
	valueTruncatedLen    = 47

	// Markdown separator for combining response sections.
	markdownSeparator = "\n\n---\n\n"

	// Default provider/model constants.
	defaultOpenAIModel = "gpt-4o"
	providerAnthropic  = "anthropic"
)

// availableProviders lists all AI providers that can be switched between during a session.
var availableProviders = []struct {
	Name        string
	Description string
}{
	{providerAnthropic, "Anthropic (Claude) - Industry-leading reasoning and coding"},
	{"openai", "OpenAI (GPT) - Most popular, widely adopted models"},
	{"gemini", "Google (Gemini) - Strong multimodal capabilities"},
	{"grok", "xAI (Grok) - Real-time data access"},
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
	viewModeSkillSelect
)

// ChatModel represents the state of the chat TUI.
type ChatModel struct {
	client               ai.Client
	atmosConfig          *schema.AtmosConfiguration // Configuration for recreating clients when switching providers
	manager              *session.Manager
	sess                 *session.Session
	executor             *tools.Executor
	memoryMgr            *instructions.Manager
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
	selectedProviderIdx  int                   // Selected provider index in provider selection
	program              *tea.Program          // Reference to the tea program for sending messages from callbacks
	markdownRenderer     *glamour.TermRenderer // Cached markdown renderer for performance
	renderedMessages     []string              // Cache of rendered messages to avoid re-rendering
	cancelFunc           context.CancelFunc    // Function to cancel ongoing AI request
	isCancelling         bool                  // Whether we're in the process of cancelling
	cumulativeUsage      aiTypes.Usage         // Cumulative token usage for the session
	lastUsage            *aiTypes.Usage        // Usage from the last AI response
	maxHistoryMessages   int                   // Maximum conversation messages to keep in history (0 = unlimited)
	maxHistoryTokens     int                   // Maximum tokens in conversation history (0 = unlimited)
	skillRegistry        *skills.Registry      // Registry of available skills
	currentSkill         *skills.Skill         // Currently active skill
	selectedSkillIdx     int                   // Selected skill index in skill selection UI
	loadingText          string                // Text to display next to spinner
}

// ChatMessage represents a single message in the chat.
type ChatMessage struct {
	Role     string // "user" or "assistant"
	Content  string
	Time     time.Time
	Provider string // The AI provider that generated this message (for assistant messages)
}

// ChatModelParams holds the parameters for creating a new ChatModel.
type ChatModelParams struct {
	Client      ai.Client
	AtmosConfig *schema.AtmosConfiguration
	Manager     *session.Manager
	Session     *session.Session
	Executor    *tools.Executor
	MemoryMgr   *instructions.Manager
}

// NewChatModel creates a new chat model with the provided parameters.
func NewChatModel(p ChatModelParams) (*ChatModel, error) {
	// Backwards-compatible alias.
	client := p.Client
	atmosConfig := p.AtmosConfig
	manager := p.Manager
	sess := p.Session
	executor := p.Executor
	memoryMgr := p.MemoryMgr
	if client == nil {
		return nil, errUtils.ErrAIClientNil
	}

	renderer := initMarkdownRenderer()
	skillRegistry, currentSkill, err := initSkills(atmosConfig)
	if err != nil {
		return nil, err
	}

	maxHistoryMessages, maxHistoryTokens := getHistoryLimits(atmosConfig)

	model := &ChatModel{
		client:               client,
		atmosConfig:          atmosConfig,
		manager:              manager,
		sess:                 sess,
		executor:             executor,
		memoryMgr:            memoryMgr,
		messages:             make([]ChatMessage, 0),
		viewport:             initViewport(),
		textarea:             initTextarea(),
		spinner:              initSpinner(),
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
		skillRegistry:        skillRegistry,
		currentSkill:         currentSkill,
		loadingText:          "AI is thinking...",
	}

	registerCompactionCallback(model, manager)

	// Load existing messages from session if available.
	if manager != nil && sess != nil {
		if loadErr := model.loadSessionMessages(); loadErr != nil {
			log.Debugf("Failed to load session messages: %v", loadErr)
		}
	}

	return model, nil
}

// initViewport creates a new viewport with default dimensions.
func initViewport() viewport.Model {
	vp := viewport.New(DefaultViewportWidth, DefaultViewportHeight)
	vp.SetContent("")
	return vp
}

// initTextarea creates a new textarea with default settings.
func initTextarea() textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Enter to send, Ctrl+J for new line, Ctrl+C to quit)"
	ta.Focus()
	ta.ShowLineNumbers = false
	ta.CharLimit = 0 // No character limit.
	return ta
}

// initSpinner creates a new spinner with default styling.
func initSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))
	return s
}

// initMarkdownRenderer creates a cached markdown renderer.
func initMarkdownRenderer() *glamour.TermRenderer {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(DefaultViewportWidth-4),
		glamour.WithColorProfile(lipgloss.ColorProfile()),
		glamour.WithEmoji(),
	)
	if err != nil {
		log.Debugf("Failed to create cached markdown renderer: %v", err)
		return nil
	}
	return renderer
}

// getHistoryLimits returns the max history messages and tokens from configuration.
func getHistoryLimits(atmosConfig *schema.AtmosConfiguration) (int, int) {
	if atmosConfig == nil {
		return 0, 0
	}
	return atmosConfig.AI.MaxHistoryMessages, atmosConfig.AI.MaxHistoryTokens
}

// initSkills loads the skill registry and determines the current skill.
func initSkills(atmosConfig *schema.AtmosConfiguration) (*skills.Registry, *skills.Skill, error) {
	var loader skills.SkillLoader
	installer, installerErr := marketplace.NewInstaller(version.Version)
	if installerErr == nil {
		loader = installer
	}

	skillRegistry, err := skills.LoadSkills(atmosConfig, loader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load skills: %w", err)
	}

	currentSkill := resolveCurrentSkill(atmosConfig, skillRegistry)

	return skillRegistry, currentSkill, nil
}

// resolveCurrentSkill determines which skill should be active.
func resolveCurrentSkill(atmosConfig *schema.AtmosConfiguration, registry *skills.Registry) *skills.Skill {
	// Try the configured default skill.
	defaultSkillName := skills.GetDefaultSkill(atmosConfig)
	if defaultSkillName != "" {
		if skill, err := registry.Get(defaultSkillName); err == nil {
			return skill
		}
	}

	// Pick the first available skill from the registry.
	allSkills := registry.List()
	if len(allSkills) > 0 {
		return allSkills[0]
	}

	// Registry is completely empty, use the fallback skill.
	fallback := skills.NewFallbackSkill()
	_ = registry.Register(fallback)
	return fallback
}

// registerCompactionCallback sets up the compaction status callback on the session manager.
func registerCompactionCallback(model *ChatModel, manager *session.Manager) {
	if manager == nil {
		return
	}

	manager.SetCompactStatusCallback(func(status session.CompactStatus) {
		if model.program != nil {
			model.program.Send(compactStatusMsg{
				stage:        status.Stage,
				messageCount: status.MessageCount,
				savings:      status.EstimatedSavings,
				err:          status.Error,
			})
		}
	})
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
				err:      errUtils.ErrAIConfigNil,
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
		oldDefaultProvider := m.atmosConfig.AI.DefaultProvider

		// Update atmosConfig to use the new provider.
		m.atmosConfig.AI.DefaultProvider = provider

		// Create new client with the updated provider.
		// PERFORMANCE: This can take 1-3 seconds, which is why we run it async.
		newClient, err := ai.NewClient(m.atmosConfig)
		if err != nil {
			// Restore old settings on failure.
			m.atmosConfig.AI.DefaultProvider = oldDefaultProvider
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
				log.Debugf("Failed to save message to session: %v", err)
			}
		}()
	}
}
