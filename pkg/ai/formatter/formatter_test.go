package formatter

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/types"
)

func TestJSONFormatter(t *testing.T) {
	tests := []struct {
		name   string
		result *ExecutionResult
		verify func(t *testing.T, output string)
	}{
		{
			name: "Success with no tools",
			result: &ExecutionResult{
				Success:  true,
				Response: "Here is the answer to your question.",
				Tokens: TokenUsage{
					Prompt:     100,
					Completion: 50,
					Total:      150,
				},
				Metadata: ExecutionMetadata{
					Model:        "claude-sonnet-4-20250514",
					Provider:     "anthropic",
					DurationMs:   1234,
					Timestamp:    time.Date(2025, 10, 31, 10, 0, 0, 0, time.UTC),
					ToolsEnabled: false,
				},
			},
			verify: func(t *testing.T, output string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				require.NoError(t, err, "Should produce valid JSON")

				assert.True(t, result["success"].(bool))
				assert.Equal(t, "Here is the answer to your question.", result["response"])
				assert.Equal(t, "claude-sonnet-4-20250514", result["metadata"].(map[string]interface{})["model"])
			},
		},
		{
			name: "Success with tool calls",
			result: &ExecutionResult{
				Success:  true,
				Response: "I executed the tools and here are the results.",
				ToolCalls: []ToolCallResult{
					{
						Tool:       "atmos_list_stacks",
						Args:       map[string]interface{}{"filter": "prod"},
						DurationMs: 45,
						Success:    true,
						Result:     []string{"prod-vpc", "prod-eks"},
					},
				},
				Tokens: TokenUsage{
					Prompt:     200,
					Completion: 80,
					Total:      280,
					Cached:     100,
				},
				Metadata: ExecutionMetadata{
					Model:        "gpt-4o",
					Provider:     "openai",
					DurationMs:   2000,
					Timestamp:    time.Date(2025, 10, 31, 10, 0, 0, 0, time.UTC),
					ToolsEnabled: true,
					StopReason:   types.StopReasonEndTurn,
				},
			},
			verify: func(t *testing.T, output string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				require.NoError(t, err, "Should produce valid JSON")

				assert.True(t, result["success"].(bool))
				assert.NotEmpty(t, result["tool_calls"])

				toolCalls := result["tool_calls"].([]interface{})
				assert.Len(t, toolCalls, 1)

				tool := toolCalls[0].(map[string]interface{})
				assert.Equal(t, "atmos_list_stacks", tool["tool"])
				assert.Equal(t, float64(45), tool["duration_ms"])
			},
		},
		{
			name: "Error result",
			result: &ExecutionResult{
				Success:  false,
				Response: "",
				Error: &ErrorInfo{
					Message: "API rate limit exceeded",
					Type:    "ai_error",
					Details: map[string]interface{}{
						"retry_after": 60,
					},
				},
				Metadata: ExecutionMetadata{
					Model:      "claude-sonnet-4-20250514",
					Provider:   "anthropic",
					DurationMs: 100,
					Timestamp:  time.Date(2025, 10, 31, 10, 0, 0, 0, time.UTC),
				},
			},
			verify: func(t *testing.T, output string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output), &result)
				require.NoError(t, err, "Should produce valid JSON")

				assert.False(t, result["success"].(bool))
				assert.NotNil(t, result["error"])

				errorInfo := result["error"].(map[string]interface{})
				assert.Equal(t, "API rate limit exceeded", errorInfo["message"])
				assert.Equal(t, "ai_error", errorInfo["type"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &JSONFormatter{}
			var buf bytes.Buffer

			err := formatter.Format(&buf, tt.result)
			require.NoError(t, err)

			output := buf.String()
			assert.NotEmpty(t, output)

			tt.verify(t, output)
		})
	}
}

func TestTextFormatter(t *testing.T) {
	tests := []struct {
		name           string
		result         *ExecutionResult
		expectedOutput string
	}{
		{
			name: "Success response",
			result: &ExecutionResult{
				Success:  true,
				Response: "This is the AI response.",
			},
			expectedOutput: "This is the AI response.\n",
		},
		{
			name: "Error response",
			result: &ExecutionResult{
				Success: false,
				Error: &ErrorInfo{
					Message: "Failed to connect",
					Type:    "ai_error",
				},
			},
			expectedOutput: "Error: Failed to connect\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &TextFormatter{}
			var buf bytes.Buffer

			err := formatter.Format(&buf, tt.result)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedOutput, buf.String())
		})
	}
}

func TestMarkdownFormatter(t *testing.T) {
	tests := []struct {
		name     string
		result   *ExecutionResult
		contains []string
	}{
		{
			name: "Response with tool executions",
			result: &ExecutionResult{
				Success:  true,
				Response: "I found the following stacks.",
				ToolCalls: []ToolCallResult{
					{
						Tool:       "atmos_list_stacks",
						DurationMs: 45,
						Success:    true,
					},
					{
						Tool:       "atmos_describe_component",
						DurationMs: 120,
						Success:    false,
						Error:      "Component not found",
					},
				},
			},
			contains: []string{
				"I found the following stacks.",
				"## Tool Executions (2)",
				"✅ **atmos_list_stacks**",
				"❌ **atmos_describe_component**",
			},
		},
		{
			name: "Error response",
			result: &ExecutionResult{
				Success: false,
				Error: &ErrorInfo{
					Message: "API error",
				},
			},
			contains: []string{
				"# Error",
				"API error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &MarkdownFormatter{}
			var buf bytes.Buffer

			err := formatter.Format(&buf, tt.result)
			require.NoError(t, err)

			output := buf.String()
			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestNewFormatter(t *testing.T) {
	tests := []struct {
		format Format
	}{
		{FormatJSON},
		{FormatText},
		{FormatMarkdown},
		{Format("unknown")}, // Default to text
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			formatter := NewFormatter(tt.format)
			assert.NotNil(t, formatter)

			// Verify it can format.
			result := &ExecutionResult{
				Success:  true,
				Response: "test",
				Metadata: ExecutionMetadata{
					Model:    "test",
					Provider: "test",
				},
			}

			var buf bytes.Buffer
			err := formatter.Format(&buf, result)
			assert.NoError(t, err)
			assert.NotEmpty(t, buf.String())
		})
	}
}
