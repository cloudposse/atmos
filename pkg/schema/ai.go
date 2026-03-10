package schema

// AISettings contains configuration for AI assistant.
type AISettings struct {
	Enabled            bool                         `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	DefaultProvider    string                       `yaml:"default_provider,omitempty" json:"default_provider,omitempty" mapstructure:"default_provider"` // Default provider for non-interactive commands
	DefaultSkill       string                       `yaml:"default_skill,omitempty" json:"default_skill,omitempty" mapstructure:"default_skill"`          // Default skill (defaults to "general")
	Providers          map[string]*AIProviderConfig `yaml:"providers,omitempty" json:"providers,omitempty" mapstructure:"providers"`                      // Per-provider configurations
	Skills             map[string]*AISkillConfig    `yaml:"skills,omitempty" json:"skills,omitempty" mapstructure:"skills"`                               // Custom skill configurations
	SendContext        bool                         `yaml:"send_context,omitempty" json:"send_context,omitempty" mapstructure:"send_context"`
	PromptOnSend       bool                         `yaml:"prompt_on_send,omitempty" json:"prompt_on_send,omitempty" mapstructure:"prompt_on_send"`
	TimeoutSeconds     int                          `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty" mapstructure:"timeout_seconds"`
	MaxContextFiles    int                          `yaml:"max_context_files,omitempty" json:"max_context_files,omitempty" mapstructure:"max_context_files"`
	MaxContextLines    int                          `yaml:"max_context_lines,omitempty" json:"max_context_lines,omitempty" mapstructure:"max_context_lines"`
	MaxHistoryMessages int                          `yaml:"max_history_messages,omitempty" json:"max_history_messages,omitempty" mapstructure:"max_history_messages"` // Maximum conversation messages to keep in history (0 = unlimited)
	MaxHistoryTokens   int                          `yaml:"max_history_tokens,omitempty" json:"max_history_tokens,omitempty" mapstructure:"max_history_tokens"`       // Maximum tokens in conversation history (0 = unlimited). If both max_history_messages and max_history_tokens are set, whichever limit is hit first is applied
	Sessions           AISessionSettings            `yaml:"sessions,omitempty" json:"sessions,omitempty" mapstructure:"sessions"`
	Tools              AIToolSettings               `yaml:"tools,omitempty" json:"tools,omitempty" mapstructure:"tools"`
	Instructions       AIInstructionsSettings       `yaml:"instructions,omitempty" json:"instructions,omitempty" mapstructure:"instructions"`
	WebSearch          AIWebSearchSettings          `yaml:"web_search,omitempty" json:"web_search,omitempty" mapstructure:"web_search"`
	Context            AIContextSettings            `yaml:"context,omitempty" json:"context,omitempty" mapstructure:"context"`
	// Security settings have been moved to AWS.Security (see aws.go).
}

// AIProviderConfig contains configuration for a specific AI provider.
type AIProviderConfig struct {
	Model     string           `yaml:"model,omitempty" json:"model,omitempty" mapstructure:"model"`
	ApiKey    string           `yaml:"api_key,omitempty" json:"api_key,omitempty" mapstructure:"api_key"` //nolint:gosec // G117: not a hardcoded credential, populated from !env YAML function
	MaxTokens int              `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty" mapstructure:"max_tokens"`
	BaseURL   string           `yaml:"base_url,omitempty" json:"base_url,omitempty" mapstructure:"base_url"` // For Ollama or custom endpoints
	Cache     *AICacheSettings `yaml:"cache,omitempty" json:"cache,omitempty" mapstructure:"cache,squash"`   // Token caching settings
}

// AICacheSettings contains token caching configuration.
type AICacheSettings struct {
	Enabled                  bool `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`                                                          // Enable token caching
	CacheSystemPrompt        bool `yaml:"cache_system_prompt,omitempty" json:"cache_system_prompt,omitempty" mapstructure:"cache_system_prompt"`                      // Cache system prompt (Anthropic only)
	CacheProjectInstructions bool `yaml:"cache_project_instructions,omitempty" json:"cache_project_instructions,omitempty" mapstructure:"cache_project_instructions"` // Cache ATMOS.md content (Anthropic only)
}

// AISessionSettings contains session management configuration.
type AISessionSettings struct {
	Enabled     bool                `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	Path        string              `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"` // Storage path
	MaxSessions int                 `yaml:"max_sessions,omitempty" json:"max_sessions,omitempty" mapstructure:"max_sessions"`
	AutoCompact AIAutoCompactConfig `yaml:"auto_compact,omitempty" json:"auto_compact,omitempty" mapstructure:"auto_compact"`
}

// AIAutoCompactConfig contains auto-compact configuration for session history.
type AIAutoCompactConfig struct {
	Enabled            bool    `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	TriggerThreshold   float64 `yaml:"trigger_threshold,omitempty" json:"trigger_threshold,omitempty" mapstructure:"trigger_threshold"`
	CompactRatio       float64 `yaml:"compact_ratio,omitempty" json:"compact_ratio,omitempty" mapstructure:"compact_ratio"`
	PreserveRecent     int     `yaml:"preserve_recent,omitempty" json:"preserve_recent,omitempty" mapstructure:"preserve_recent"`
	UseAISummary       bool    `yaml:"use_ai_summary,omitempty" json:"use_ai_summary,omitempty" mapstructure:"use_ai_summary"`
	ShowSummaryMarkers bool    `yaml:"show_summary_markers,omitempty" json:"show_summary_markers,omitempty" mapstructure:"show_summary_markers"`
}

// AIToolSettings contains tool execution configuration.
type AIToolSettings struct {
	Enabled             bool     `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	RequireConfirmation *bool    `yaml:"require_confirmation,omitempty" json:"require_confirmation,omitempty" mapstructure:"require_confirmation"`
	AllowedTools        []string `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty" mapstructure:"allowed_tools"`
	RestrictedTools     []string `yaml:"restricted_tools,omitempty" json:"restricted_tools,omitempty" mapstructure:"restricted_tools"`
	BlockedTools        []string `yaml:"blocked_tools,omitempty" json:"blocked_tools,omitempty" mapstructure:"blocked_tools"`
	YOLOMode            bool     `yaml:"yolo_mode,omitempty" json:"yolo_mode,omitempty" mapstructure:"yolo_mode"`
}

// AIInstructionsSettings contains project instructions configuration.
type AIInstructionsSettings struct {
	Enabled  bool   `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	FilePath string `yaml:"file,omitempty" json:"file,omitempty" mapstructure:"file"` // Path to ATMOS.md
}

// AIWebSearchSettings contains web search configuration.
type AIWebSearchSettings struct {
	Enabled      bool   `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	GoogleAPIKey string `yaml:"google_api_key,omitempty" json:"google_api_key,omitempty" mapstructure:"google_api_key"` // For Google Custom Search
	GoogleCSEID  string `yaml:"google_cse_id,omitempty" json:"google_cse_id,omitempty" mapstructure:"google_cse_id"`    // Google Custom Search Engine ID
	MaxResults   int    `yaml:"max_results,omitempty" json:"max_results,omitempty" mapstructure:"max_results"`          // Maximum results to return
}

// AIContextSettings contains automatic context discovery configuration.
type AIContextSettings struct {
	Enabled         bool     `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`                               // Enable automatic context loading
	AutoInclude     []string `yaml:"auto_include,omitempty" json:"auto_include,omitempty" mapstructure:"auto_include"`                // Glob patterns for files to auto-include
	Exclude         []string `yaml:"exclude,omitempty" json:"exclude,omitempty" mapstructure:"exclude"`                               // Glob patterns for files to exclude
	MaxFiles        int      `yaml:"max_files,omitempty" json:"max_files,omitempty" mapstructure:"max_files"`                         // Maximum number of files to include (default: 100)
	MaxSizeMB       int      `yaml:"max_size_mb,omitempty" json:"max_size_mb,omitempty" mapstructure:"max_size_mb"`                   // Maximum total size in MB (default: 10)
	FollowGitignore bool     `yaml:"follow_gitignore,omitempty" json:"follow_gitignore,omitempty" mapstructure:"follow_gitignore"`    // Respect .gitignore files (default: true)
	ShowFiles       bool     `yaml:"show_files,omitempty" json:"show_files,omitempty" mapstructure:"show_files"`                      // Show list of included files in UI (default: false)
	CacheEnabled    bool     `yaml:"cache_enabled,omitempty" json:"cache_enabled,omitempty" mapstructure:"cache_enabled"`             // Cache discovered files (default: true)
	CacheTTL        int      `yaml:"cache_ttl_seconds,omitempty" json:"cache_ttl_seconds,omitempty" mapstructure:"cache_ttl_seconds"` // Cache TTL in seconds (default: 300)
}

// AISkillConfig contains configuration for a custom AI skill.
// Skills follow the Agent Skills open standard (https://agentskills.io).
type AISkillConfig struct {
	DisplayName     string   `yaml:"display_name,omitempty" json:"display_name,omitempty" mapstructure:"display_name"`             // User-facing name
	Description     string   `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`                // What this skill does
	SystemPrompt    string   `yaml:"system_prompt,omitempty" json:"system_prompt,omitempty" mapstructure:"system_prompt"`          // Specialized instructions
	AllowedTools    []string `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty" mapstructure:"allowed_tools"`          // Tool names this skill can use (empty = all tools)
	RestrictedTools []string `yaml:"restricted_tools,omitempty" json:"restricted_tools,omitempty" mapstructure:"restricted_tools"` // Tools requiring extra confirmation
	Category        string   `yaml:"category,omitempty" json:"category,omitempty" mapstructure:"category"`                         // "analysis", "refactor", "security", etc.
}
