package session

import (
	"time"
)

// Session represents an AI chat session with persistent state.
type Session struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	ProjectPath  string                 `json:"project_path"`
	Model        string                 `json:"model"`
	Provider     string                 `json:"provider"`
	Agent        string                 `json:"agent,omitempty"` // AI agent name (e.g., "general", "stack-analyzer")
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	MessageCount int                    `json:"message_count,omitempty"`
}

// Message represents a single message in a session.
type Message struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"` // user, assistant, system
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	Archived  bool      `json:"archived"`   // True if message has been compacted into a summary
	IsSummary bool      `json:"is_summary"` // True if this is a summary message (for display purposes)
}

// ContextItem represents a piece of context associated with a session.
type ContextItem struct {
	SessionID    string `json:"session_id"`
	ContextType  string `json:"context_type"`  // stack_file, component, setting
	ContextKey   string `json:"context_key"`   // e.g., "vpc", "prod-use1"
	ContextValue string `json:"context_value"` // JSON or text value
}

// Role constants for messages.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// ContextType constants.
const (
	ContextTypeStackFile = "stack_file"
	ContextTypeComponent = "component"
	ContextTypeSetting   = "setting"
	ContextTypeQuery     = "query"
	ContextTypeATMOSMD   = "atmos_md"
)

// Summary represents a compacted summary of multiple messages.
type Summary struct {
	ID                 string    `json:"id"`
	SessionID          string    `json:"session_id"`
	Provider           string    `json:"provider"`
	OriginalMessageIDs []int64   `json:"original_message_ids"`
	MessageRange       string    `json:"message_range"` // Human-readable: "Messages 1-20"
	SummaryContent     string    `json:"summary_content"`
	TokenCount         int       `json:"token_count"`
	CompactedAt        time.Time `json:"compacted_at"`
}

// CompactConfig holds configuration for auto-compact behavior.
type CompactConfig struct {
	Enabled            bool    `json:"enabled" yaml:"enabled"`
	TriggerThreshold   float64 `json:"trigger_threshold" yaml:"trigger_threshold"`       // 0.0-1.0, default 0.75
	CompactRatio       float64 `json:"compact_ratio" yaml:"compact_ratio"`               // 0.0-1.0, default 0.4
	PreserveRecent     int     `json:"preserve_recent" yaml:"preserve_recent"`           // Default 10
	UseAISummary       bool    `json:"use_ai_summary" yaml:"use_ai_summary"`             // Default true
	SummaryProvider    string  `json:"summary_provider" yaml:"summary_provider"`         // Optional
	SummaryModel       string  `json:"summary_model" yaml:"summary_model"`               // Optional
	SummaryMaxTokens   int     `json:"summary_max_tokens" yaml:"summary_max_tokens"`     // Default 2048
	ShowSummaryMarkers bool    `json:"show_summary_markers" yaml:"show_summary_markers"` // Default false
	CompactOnResume    bool    `json:"compact_on_resume" yaml:"compact_on_resume"`       // Default false
}

// CompactPlan describes what will be compacted.
type CompactPlan struct {
	SessionID         string
	TotalMessages     int
	MessagesToCompact []*Message
	MessagesToKeep    []*Message
	EstimatedSavings  int // Token savings estimate
	Reason            string
}

// CompactResult contains the outcome of compaction.
type CompactResult struct {
	SummaryID          string
	OriginalMessageIDs []int64
	SummaryContent     string
	TokenCount         int
	CompactedAt        time.Time
	Success            bool
	Error              error
}
