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
