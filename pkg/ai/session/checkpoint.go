package session

import (
	"time"
)

const (
	// CheckpointVersion is the current checkpoint format version.
	CheckpointVersion = "1.0"
)

// Checkpoint represents an exportable session snapshot.
// It contains all information needed to restore a session.
type Checkpoint struct {
	// Version is the checkpoint format version for compatibility.
	Version string `json:"version" yaml:"version"`

	// ExportedAt is when the checkpoint was created.
	ExportedAt time.Time `json:"exported_at" yaml:"exported_at"`

	// ExportedBy is the user who exported the checkpoint (optional).
	ExportedBy string `json:"exported_by,omitempty" yaml:"exported_by,omitempty"`

	// Session contains session metadata.
	Session CheckpointSession `json:"session" yaml:"session"`

	// Messages contains the complete conversation history.
	Messages []CheckpointMessage `json:"messages" yaml:"messages"`

	// Context contains project-specific context.
	Context *CheckpointContext `json:"context,omitempty" yaml:"context,omitempty"`

	// Statistics contains session statistics.
	Statistics CheckpointStatistics `json:"statistics" yaml:"statistics"`
}

// CheckpointSession contains session metadata for export.
type CheckpointSession struct {
	// Name is the session name.
	Name string `json:"name" yaml:"name"`

	// Provider is the AI provider (anthropic, openai, etc.).
	Provider string `json:"provider" yaml:"provider"`

	// Model is the AI model used.
	Model string `json:"model" yaml:"model"`

	// Agent is the AI agent name (optional).
	Agent string `json:"agent,omitempty" yaml:"agent,omitempty"`

	// ProjectPath is the project path where session was created.
	ProjectPath string `json:"project_path" yaml:"project_path"`

	// CreatedAt is when the session was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// UpdatedAt is when the session was last updated.
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`

	// Metadata contains custom metadata.
	Metadata map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// CheckpointMessage represents a message in the checkpoint.
type CheckpointMessage struct {
	// Role is the message role (user, assistant, system).
	Role string `json:"role" yaml:"role"`

	// Content is the message content.
	Content string `json:"content" yaml:"content"`

	// CreatedAt is when the message was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// Archived indicates if the message was compacted.
	Archived bool `json:"archived,omitempty" yaml:"archived,omitempty"`
}

// CheckpointContext contains project-specific context.
type CheckpointContext struct {
	// ProjectMemory is the ATMOS.md content at export time (optional).
	ProjectMemory string `json:"project_memory,omitempty" yaml:"project_memory,omitempty"`

	// FilesAccessed is a list of files accessed during the session (optional).
	FilesAccessed []string `json:"files_accessed,omitempty" yaml:"files_accessed,omitempty"`

	// WorkingDirectory is the working directory at export time (optional).
	WorkingDirectory string `json:"working_directory,omitempty" yaml:"working_directory,omitempty"`
}

// CheckpointStatistics contains session statistics.
type CheckpointStatistics struct {
	// MessageCount is the total number of messages.
	MessageCount int `json:"message_count" yaml:"message_count"`

	// UserMessages is the number of user messages.
	UserMessages int `json:"user_messages" yaml:"user_messages"`

	// AssistantMessages is the number of assistant messages.
	AssistantMessages int `json:"assistant_messages" yaml:"assistant_messages"`

	// TotalTokens is the total token count (if tracked).
	TotalTokens int64 `json:"total_tokens,omitempty" yaml:"total_tokens,omitempty"`

	// ToolCalls is the number of tool executions (if tracked).
	ToolCalls int `json:"tool_calls,omitempty" yaml:"tool_calls,omitempty"`
}

// ImportOptions contains options for importing a checkpoint.
type ImportOptions struct {
	// Name is the name for the imported session.
	// If empty, uses the checkpoint's session name.
	Name string

	// ProjectPath is the project path for the imported session.
	// If empty, uses the current project path.
	ProjectPath string

	// OverwriteExisting allows overwriting an existing session with the same name.
	OverwriteExisting bool

	// IncludeContext includes project context in the import.
	IncludeContext bool
}

// ExportOptions contains options for exporting a checkpoint.
type ExportOptions struct {
	// IncludeContext includes project memory and file access history.
	IncludeContext bool

	// IncludeMetadata includes session metadata.
	IncludeMetadata bool

	// Format is the export format (json, yaml, markdown).
	Format string
}
