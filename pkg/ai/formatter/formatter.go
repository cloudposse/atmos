package formatter

import (
	"io"
	"time"

	"github.com/cloudposse/atmos/pkg/ai/types"
)

// Formatter defines the interface for formatting AI responses in different output formats.
type Formatter interface {
	// Format writes the formatted response to the writer.
	Format(w io.Writer, result *ExecutionResult) error
}

// ExecutionResult contains the complete result of an AI execution.
type ExecutionResult struct {
	// Success indicates if the execution completed successfully.
	Success bool `json:"success"`

	// Response is the final text response from the AI.
	Response string `json:"response"`

	// ToolCalls contains information about any tools that were executed.
	ToolCalls []ToolCallResult `json:"tool_calls,omitempty"`

	// Tokens contains token usage information.
	Tokens TokenUsage `json:"tokens"`

	// Metadata contains execution metadata.
	Metadata ExecutionMetadata `json:"metadata"`

	// Error contains error information if the execution failed.
	Error *ErrorInfo `json:"error,omitempty"`
}

// ToolCallResult contains the result of a tool execution.
type ToolCallResult struct {
	// Tool is the name of the tool that was called.
	Tool string `json:"tool"`

	// Args contains the arguments passed to the tool.
	Args map[string]interface{} `json:"args,omitempty"`

	// DurationMs is the execution time in milliseconds.
	DurationMs int64 `json:"duration_ms"`

	// Success indicates if the tool executed successfully.
	Success bool `json:"success"`

	// Result contains the tool's output (if successful).
	Result interface{} `json:"result,omitempty"`

	// Error contains error information (if failed).
	Error string `json:"error,omitempty"`
}

// TokenUsage contains token usage statistics.
type TokenUsage struct {
	// Prompt tokens used in the request.
	Prompt int64 `json:"prompt"`

	// Completion tokens used in the response.
	Completion int64 `json:"completion"`

	// Total tokens used (prompt + completion).
	Total int64 `json:"total"`

	// Cached tokens read from prompt cache (if supported).
	Cached int64 `json:"cached,omitempty"`

	// CacheCreation tokens used to create cache (if supported).
	CacheCreation int64 `json:"cache_creation,omitempty"`
}

// ExecutionMetadata contains metadata about the execution.
type ExecutionMetadata struct {
	// Model is the AI model that was used.
	Model string `json:"model"`

	// Provider is the AI provider (anthropic, openai, etc.).
	Provider string `json:"provider"`

	// SessionID is the session identifier (if using sessions).
	SessionID string `json:"session_id,omitempty"`

	// DurationMs is the total execution time in milliseconds.
	DurationMs int64 `json:"duration_ms"`

	// Timestamp is when the execution started.
	Timestamp time.Time `json:"timestamp"`

	// ToolsEnabled indicates if tool execution was enabled.
	ToolsEnabled bool `json:"tools_enabled"`

	// StopReason indicates why the model stopped generating.
	StopReason types.StopReason `json:"stop_reason,omitempty"`
}

// ErrorInfo contains error information.
type ErrorInfo struct {
	// Message is the error message.
	Message string `json:"message"`

	// Type categorizes the error (ai_error, tool_error, config_error, etc.).
	Type string `json:"type"`

	// Details contains additional error context.
	Details map[string]interface{} `json:"details,omitempty"`
}

// Format represents the output format type.
type Format string

const (
	// FormatJSON outputs structured JSON.
	FormatJSON Format = "json"

	// FormatText outputs plain text (default).
	FormatText Format = "text"

	// FormatMarkdown outputs formatted markdown.
	FormatMarkdown Format = "markdown"
)

// NewFormatter creates a formatter for the specified format.
func NewFormatter(format Format) Formatter {
	switch format {
	case FormatJSON:
		return &JSONFormatter{}
	case FormatMarkdown:
		return &MarkdownFormatter{}
	default:
		return &TextFormatter{}
	}
}
