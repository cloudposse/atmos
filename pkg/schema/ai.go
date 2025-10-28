package schema

// AISettings contains configuration for AI assistant.
type AISettings struct {
	Enabled            bool                         `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	DefaultProvider    string                       `yaml:"default_provider,omitempty" json:"default_provider,omitempty" mapstructure:"default_provider"` // Default provider for non-interactive commands
	DefaultAgent       string                       `yaml:"default_agent,omitempty" json:"default_agent,omitempty" mapstructure:"default_agent"`          // Default agent (defaults to "general")
	Providers          map[string]*AIProviderConfig `yaml:"providers,omitempty" json:"providers,omitempty" mapstructure:"providers"`                      // Per-provider configurations
	Agents             map[string]*AIAgentConfig    `yaml:"agents,omitempty" json:"agents,omitempty" mapstructure:"agents"`                               // Custom agent configurations
	SendContext        bool                         `yaml:"send_context,omitempty" json:"send_context,omitempty" mapstructure:"send_context"`
	PromptOnSend       bool                         `yaml:"prompt_on_send,omitempty" json:"prompt_on_send,omitempty" mapstructure:"prompt_on_send"`
	TimeoutSeconds     int                          `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty" mapstructure:"timeout_seconds"`
	MaxContextFiles    int                          `yaml:"max_context_files,omitempty" json:"max_context_files,omitempty" mapstructure:"max_context_files"`
	MaxContextLines    int                          `yaml:"max_context_lines,omitempty" json:"max_context_lines,omitempty" mapstructure:"max_context_lines"`
	MaxHistoryMessages int                          `yaml:"max_history_messages,omitempty" json:"max_history_messages,omitempty" mapstructure:"max_history_messages"` // Maximum conversation messages to keep in history (0 = unlimited)
	MaxHistoryTokens   int                          `yaml:"max_history_tokens,omitempty" json:"max_history_tokens,omitempty" mapstructure:"max_history_tokens"`       // Maximum tokens in conversation history (0 = unlimited). If both max_history_messages and max_history_tokens are set, whichever limit is hit first is applied
	Sessions           AISessionSettings            `yaml:"sessions,omitempty" json:"sessions,omitempty" mapstructure:"sessions"`
	Tools              AIToolSettings               `yaml:"tools,omitempty" json:"tools,omitempty" mapstructure:"tools"`
	Memory             AIMemorySettings             `yaml:"memory,omitempty" json:"memory,omitempty" mapstructure:"memory"`
	WebSearch          AIWebSearchSettings          `yaml:"web_search,omitempty" json:"web_search,omitempty" mapstructure:"web_search"`
	UseLSP             bool                         `yaml:"use_lsp,omitempty" json:"use_lsp,omitempty" mapstructure:"use_lsp"` // Enable LSP integration for diagnostics
}

// AIProviderConfig contains configuration for a specific AI provider.
type AIProviderConfig struct {
	Model     string `yaml:"model,omitempty" json:"model,omitempty" mapstructure:"model"`
	ApiKeyEnv string `yaml:"api_key_env,omitempty" json:"api_key_env,omitempty" mapstructure:"api_key_env"`
	MaxTokens int    `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty" mapstructure:"max_tokens"`
	BaseURL   string `yaml:"base_url,omitempty" json:"base_url,omitempty" mapstructure:"base_url"` // For Ollama or custom endpoints
}

// AISessionSettings contains session management configuration.
type AISessionSettings struct {
	Enabled       bool   `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	Storage       string `yaml:"storage,omitempty" json:"storage,omitempty" mapstructure:"storage"` // sqlite, json
	Path          string `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`          // Storage path
	MaxSessions   int    `yaml:"max_sessions,omitempty" json:"max_sessions,omitempty" mapstructure:"max_sessions"`
	AutoSave      bool   `yaml:"auto_save,omitempty" json:"auto_save,omitempty" mapstructure:"auto_save"`
	RetentionDays int    `yaml:"retention_days,omitempty" json:"retention_days,omitempty" mapstructure:"retention_days"`
}

// AIToolSettings contains tool execution configuration.
type AIToolSettings struct {
	Enabled             bool     `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	RequireConfirmation bool     `yaml:"require_confirmation,omitempty" json:"require_confirmation,omitempty" mapstructure:"require_confirmation"`
	AllowedTools        []string `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty" mapstructure:"allowed_tools"`
	RestrictedTools     []string `yaml:"restricted_tools,omitempty" json:"restricted_tools,omitempty" mapstructure:"restricted_tools"`
	BlockedTools        []string `yaml:"blocked_tools,omitempty" json:"blocked_tools,omitempty" mapstructure:"blocked_tools"`
	YOLOMode            bool     `yaml:"yolo_mode,omitempty" json:"yolo_mode,omitempty" mapstructure:"yolo_mode"`
}

// AIMemorySettings contains project memory configuration.
type AIMemorySettings struct {
	Enabled      bool     `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	FilePath     string   `yaml:"file,omitempty" json:"file,omitempty" mapstructure:"file"` // Path to ATMOS.md
	AutoUpdate   bool     `yaml:"auto_update,omitempty" json:"auto_update,omitempty" mapstructure:"auto_update"`
	CreateIfMiss bool     `yaml:"create_if_missing,omitempty" json:"create_if_missing,omitempty" mapstructure:"create_if_missing"`
	Sections     []string `yaml:"sections,omitempty" json:"sections,omitempty" mapstructure:"sections"` // Sections to include in context
}

// AIWebSearchSettings contains web search configuration.
type AIWebSearchSettings struct {
	Enabled        bool   `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	Engine         string `yaml:"engine,omitempty" json:"engine,omitempty" mapstructure:"engine"`                         // duckduckgo, google
	GoogleAPIKey   string `yaml:"google_api_key,omitempty" json:"google_api_key,omitempty" mapstructure:"google_api_key"` // For Google Custom Search
	GoogleCSEID    string `yaml:"google_cse_id,omitempty" json:"google_cse_id,omitempty" mapstructure:"google_cse_id"`    // Google Custom Search Engine ID
	MaxResults     int    `yaml:"max_results,omitempty" json:"max_results,omitempty" mapstructure:"max_results"`          // Maximum results to return
	TimeoutSeconds int    `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty" mapstructure:"timeout_seconds"`
}

// AIAgentConfig contains configuration for a custom AI agent.
type AIAgentConfig struct {
	DisplayName     string   `yaml:"display_name,omitempty" json:"display_name,omitempty" mapstructure:"display_name"`             // User-facing name
	Description     string   `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`                // What this agent does
	SystemPrompt    string   `yaml:"system_prompt,omitempty" json:"system_prompt,omitempty" mapstructure:"system_prompt"`          // Specialized instructions
	AllowedTools    []string `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty" mapstructure:"allowed_tools"`          // Tool names this agent can use (empty = all tools)
	RestrictedTools []string `yaml:"restricted_tools,omitempty" json:"restricted_tools,omitempty" mapstructure:"restricted_tools"` // Tools requiring extra confirmation
	Category        string   `yaml:"category,omitempty" json:"category,omitempty" mapstructure:"category"`                         // "analysis", "refactor", "security", etc.
}
