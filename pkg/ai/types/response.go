package types

// Response represents an AI response that may include tool calls.
type Response struct {
	// Content is the text response from the AI.
	Content string

	// ToolCalls are any tool invocations requested by the AI.
	ToolCalls []ToolCall

	// StopReason indicates why the model stopped generating.
	StopReason StopReason

	// Usage contains token usage information from the API.
	Usage *Usage
}

// Usage contains token usage statistics from the AI provider.
type Usage struct {
	// InputTokens is the number of tokens in the request/prompt.
	InputTokens int64

	// OutputTokens is the number of tokens in the response/completion.
	OutputTokens int64

	// TotalTokens is the sum of input and output tokens.
	TotalTokens int64

	// CacheReadTokens is the number of tokens read from prompt cache (if supported).
	CacheReadTokens int64

	// CacheCreationTokens is the number of tokens used to create cache (if supported).
	CacheCreationTokens int64
}

// ToolCall represents a request from the AI to execute a tool.
type ToolCall struct {
	// ID is a unique identifier for this tool call.
	ID string

	// Name is the tool name to execute.
	Name string

	// Input contains the parameters for the tool.
	Input map[string]interface{}
}

// StopReason indicates why the model stopped generating.
type StopReason string

const (
	// StopReasonEndTurn indicates the model naturally finished its response.
	StopReasonEndTurn StopReason = "end_turn"

	// StopReasonToolUse indicates the model wants to use a tool.
	StopReasonToolUse StopReason = "tool_use"

	// StopReasonMaxTokens indicates the max token limit was reached.
	StopReasonMaxTokens StopReason = "max_tokens"
)
