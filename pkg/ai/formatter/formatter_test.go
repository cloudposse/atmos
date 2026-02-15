package formatter

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/types"
)

// failingWriter is a mock writer that always returns an error.
type failingWriter struct {
	failAfter int
	written   int
}

func (fw *failingWriter) Write(p []byte) (n int, err error) {
	if fw.failAfter <= fw.written {
		return 0, errors.New("write error")
	}
	fw.written += len(p)
	return len(p), nil
}

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
					Model:        "claude-sonnet-4-5-20250929",
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
				assert.Equal(t, "claude-sonnet-4-5-20250929", result["metadata"].(map[string]interface{})["model"])
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
					Model:      "claude-sonnet-4-5-20250929",
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
		{
			name: "Success response without tool calls",
			result: &ExecutionResult{
				Success:   true,
				Response:  "Here is your answer without using any tools.",
				ToolCalls: []ToolCallResult{},
			},
			contains: []string{
				"Here is your answer without using any tools.",
			},
		},
		{
			name: "Success response with nil tool calls",
			result: &ExecutionResult{
				Success:   true,
				Response:  "Simple response.",
				ToolCalls: nil,
			},
			contains: []string{
				"Simple response.",
			},
		},
		{
			name: "Error with empty message",
			result: &ExecutionResult{
				Success: false,
				Error: &ErrorInfo{
					Message: "",
				},
			},
			contains: []string{
				"# Error",
			},
		},
		{
			name: "Response with single successful tool",
			result: &ExecutionResult{
				Success:  true,
				Response: "Executed one tool successfully.",
				ToolCalls: []ToolCallResult{
					{
						Tool:       "atmos_list_components",
						DurationMs: 100,
						Success:    true,
					},
				},
			},
			contains: []string{
				"Executed one tool successfully.",
				"## Tool Executions (1)",
				"✅ **atmos_list_components** (100ms)",
			},
		},
		{
			name: "Response with multiple tools mixed success",
			result: &ExecutionResult{
				Success:  true,
				Response: "Mixed results from tools.",
				ToolCalls: []ToolCallResult{
					{
						Tool:       "tool1",
						DurationMs: 50,
						Success:    true,
					},
					{
						Tool:       "tool2",
						DurationMs: 75,
						Success:    false,
					},
					{
						Tool:       "tool3",
						DurationMs: 120,
						Success:    true,
					},
				},
			},
			contains: []string{
				"Mixed results from tools.",
				"## Tool Executions (3)",
				"1. ✅ **tool1** (50ms)",
				"2. ❌ **tool2** (75ms)",
				"3. ✅ **tool3** (120ms)",
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

func TestMarkdownFormatter_WriteErrors(t *testing.T) {
	tests := []struct {
		name      string
		result    *ExecutionResult
		failAfter int
	}{
		{
			name: "Error writing error message",
			result: &ExecutionResult{
				Success: false,
				Error: &ErrorInfo{
					Message: "API error occurred",
				},
			},
			failAfter: 0,
		},
		{
			name: "Error writing response",
			result: &ExecutionResult{
				Success:  true,
				Response: "This is a successful response.",
			},
			failAfter: 0,
		},
		{
			name: "Error writing separator before tool executions",
			result: &ExecutionResult{
				Success:  true,
				Response: "Response with tools.",
				ToolCalls: []ToolCallResult{
					{
						Tool:       "test_tool",
						DurationMs: 100,
						Success:    true,
					},
				},
			},
			failAfter: 50,
		},
		{
			name: "Error writing tool executions header",
			result: &ExecutionResult{
				Success:  true,
				Response: "Short",
				ToolCalls: []ToolCallResult{
					{
						Tool:       "test_tool",
						DurationMs: 100,
						Success:    true,
					},
				},
			},
			failAfter: 20,
		},
		{
			name: "Error writing individual tool call",
			result: &ExecutionResult{
				Success:  true,
				Response: "R",
				ToolCalls: []ToolCallResult{
					{
						Tool:       "test_tool",
						DurationMs: 100,
						Success:    true,
					},
				},
			},
			failAfter: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &MarkdownFormatter{}
			fw := &failingWriter{failAfter: tt.failAfter}

			err := formatter.Format(fw, tt.result)
			assert.Error(t, err, "Expected error when writer fails")
			assert.Contains(t, err.Error(), "write error")
		})
	}
}

func TestMarkdownFormatter_OutputFormat(t *testing.T) {
	tests := []struct {
		name           string
		result         *ExecutionResult
		expectedOutput string
	}{
		{
			name: "Simple error formatting",
			result: &ExecutionResult{
				Success: false,
				Error: &ErrorInfo{
					Message: "Test error",
				},
			},
			expectedOutput: "# Error\n\nTest error\n",
		},
		{
			name: "Simple response without newline handling",
			result: &ExecutionResult{
				Success:  true,
				Response: "Simple response",
			},
			expectedOutput: "Simple response\n",
		},
		{
			name: "Response with newlines preserved",
			result: &ExecutionResult{
				Success:  true,
				Response: "Line 1\nLine 2\nLine 3",
			},
			expectedOutput: "Line 1\nLine 2\nLine 3\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &MarkdownFormatter{}
			var buf bytes.Buffer

			err := formatter.Format(&buf, tt.result)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedOutput, buf.String())
		})
	}
}

//nolint:dupl
func TestMarkdownFormatter_ToolCallsZeroValue(t *testing.T) {
	// Test that we properly handle the boundary between no tools and some tools.
	tests := []struct {
		name          string
		toolCallCount int
		expectToolSec bool
	}{
		{
			name:          "Zero tools - no tool section",
			toolCallCount: 0,
			expectToolSec: false,
		},
		{
			name:          "One tool - show tool section",
			toolCallCount: 1,
			expectToolSec: true,
		},
		{
			name:          "Multiple tools - show tool section",
			toolCallCount: 5,
			expectToolSec: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolCalls := make([]ToolCallResult, tt.toolCallCount)
			for i := 0; i < tt.toolCallCount; i++ {
				toolCalls[i] = ToolCallResult{
					Tool:       "test_tool",
					DurationMs: 100,
					Success:    true,
				}
			}

			result := &ExecutionResult{
				Success:   true,
				Response:  "Test response",
				ToolCalls: toolCalls,
			}

			formatter := &MarkdownFormatter{}
			var buf bytes.Buffer

			err := formatter.Format(&buf, result)
			require.NoError(t, err)

			output := buf.String()
			if tt.expectToolSec {
				assert.Contains(t, output, "## Tool Executions")
			} else {
				assert.NotContains(t, output, "## Tool Executions")
			}
		})
	}
}

func TestMarkdownFormatter_CompleteToolExecutionSection(t *testing.T) {
	// Ensure we test all parts of the tool execution rendering.
	result := &ExecutionResult{
		Success:  true,
		Response: "Complete test",
		ToolCalls: []ToolCallResult{
			{
				Tool:       "first_tool",
				DurationMs: 10,
				Success:    true,
			},
			{
				Tool:       "second_tool",
				DurationMs: 20,
				Success:    false,
			},
			{
				Tool:       "third_tool",
				DurationMs: 30,
				Success:    true,
			},
		},
	}

	formatter := &MarkdownFormatter{}
	var buf bytes.Buffer

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()

	// Verify separator is included.
	assert.Contains(t, output, "\n---\n\n")

	// Verify header with count.
	assert.Contains(t, output, "## Tool Executions (3)")

	// Verify all three tool calls are numbered correctly.
	assert.Contains(t, output, "1. ✅ **first_tool** (10ms)")
	assert.Contains(t, output, "2. ❌ **second_tool** (20ms)")
	assert.Contains(t, output, "3. ✅ **third_tool** (30ms)")
}

func TestMarkdownFormatter_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		result *ExecutionResult
	}{
		{
			name: "Empty response string",
			result: &ExecutionResult{
				Success:  true,
				Response: "",
			},
		},
		{
			name: "Response with special markdown characters",
			result: &ExecutionResult{
				Success:  true,
				Response: "# Header\n## Subheader\n**bold** *italic* `code`",
			},
		},
		{
			name: "Tool with zero duration",
			result: &ExecutionResult{
				Success:  true,
				Response: "Fast tool",
				ToolCalls: []ToolCallResult{
					{
						Tool:       "instant_tool",
						DurationMs: 0,
						Success:    true,
					},
				},
			},
		},
		{
			name: "Tool with large duration",
			result: &ExecutionResult{
				Success:  true,
				Response: "Slow tool",
				ToolCalls: []ToolCallResult{
					{
						Tool:       "slow_tool",
						DurationMs: 999999,
						Success:    true,
					},
				},
			},
		},
		{
			name: "Error with very long message",
			result: &ExecutionResult{
				Success: false,
				Error: &ErrorInfo{
					Message: "This is a very long error message that might span multiple lines and contains a lot of detail about what went wrong during the execution of the command. It includes stack traces, error codes, and various other debugging information that might be useful for troubleshooting.",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &MarkdownFormatter{}
			var buf bytes.Buffer

			err := formatter.Format(&buf, tt.result)
			require.NoError(t, err)
			assert.NotEmpty(t, buf.String())
		})
	}
}

func TestMarkdownFormatter_MultipleToolsWithErrors(t *testing.T) {
	// Test that we properly iterate through all tools and handle each one.
	result := &ExecutionResult{
		Success:  true,
		Response: "Multiple tools executed",
		ToolCalls: []ToolCallResult{
			{
				Tool:       "tool_a",
				DurationMs: 50,
				Success:    true,
			},
			{
				Tool:       "tool_b",
				DurationMs: 100,
				Success:    false,
			},
			{
				Tool:       "tool_c",
				DurationMs: 75,
				Success:    true,
			},
			{
				Tool:       "tool_d",
				DurationMs: 200,
				Success:    false,
			},
		},
	}

	formatter := &MarkdownFormatter{}
	var buf bytes.Buffer

	err := formatter.Format(&buf, result)
	require.NoError(t, err)

	output := buf.String()

	// Verify all tools are present with correct status icons.
	assert.Contains(t, output, "1. ✅ **tool_a** (50ms)")
	assert.Contains(t, output, "2. ❌ **tool_b** (100ms)")
	assert.Contains(t, output, "3. ✅ **tool_c** (75ms)")
	assert.Contains(t, output, "4. ❌ **tool_d** (200ms)")
}

func TestMarkdownFormatter_ErrorReturnPaths(t *testing.T) {
	// Test different error return paths with precise failing points.
	tests := []struct {
		name      string
		result    *ExecutionResult
		failAfter int
		expectErr bool
	}{
		{
			name: "Fail on second tool write",
			result: &ExecutionResult{
				Success:  true,
				Response: "X",
				ToolCalls: []ToolCallResult{
					{Tool: "a", DurationMs: 1, Success: true},
					{Tool: "b", DurationMs: 2, Success: true},
				},
			},
			failAfter: 50,
			expectErr: true,
		},
		{
			name: "Fail on tool execution header",
			result: &ExecutionResult{
				Success:  true,
				Response: "Y",
				ToolCalls: []ToolCallResult{
					{Tool: "c", DurationMs: 3, Success: false},
				},
			},
			failAfter: 15,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &MarkdownFormatter{}
			fw := &failingWriter{failAfter: tt.failAfter}

			err := formatter.Format(fw, tt.result)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMarkdownFormatter_AllBranches(t *testing.T) {
	// Ensure we hit all branches in the Format function.
	t.Run("Error path returns early", func(t *testing.T) {
		result := &ExecutionResult{
			Success: false,
			Error: &ErrorInfo{
				Message: "test error",
			},
		}
		formatter := &MarkdownFormatter{}
		var buf bytes.Buffer

		err := formatter.Format(&buf, result)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "# Error")
		assert.NotContains(t, buf.String(), "Tool Executions")
	})

	t.Run("Success path with no tools", func(t *testing.T) {
		result := &ExecutionResult{
			Success:  true,
			Response: "no tools used",
		}
		formatter := &MarkdownFormatter{}
		var buf bytes.Buffer

		err := formatter.Format(&buf, result)
		require.NoError(t, err)
		assert.Equal(t, "no tools used\n", buf.String())
	})

	t.Run("Success path with empty tool slice", func(t *testing.T) {
		result := &ExecutionResult{
			Success:   true,
			Response:  "empty slice",
			ToolCalls: []ToolCallResult{},
		}
		formatter := &MarkdownFormatter{}
		var buf bytes.Buffer

		err := formatter.Format(&buf, result)
		require.NoError(t, err)
		assert.Equal(t, "empty slice\n", buf.String())
	})

	t.Run("Success with tools - successful tool", func(t *testing.T) {
		result := &ExecutionResult{
			Success:  true,
			Response: "with tool",
			ToolCalls: []ToolCallResult{
				{Tool: "test", DurationMs: 100, Success: true},
			},
		}
		formatter := &MarkdownFormatter{}
		var buf bytes.Buffer

		err := formatter.Format(&buf, result)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "✅")
		assert.Contains(t, buf.String(), "test")
	})

	t.Run("Success with tools - failed tool", func(t *testing.T) {
		result := &ExecutionResult{
			Success:  true,
			Response: "with failed tool",
			ToolCalls: []ToolCallResult{
				{Tool: "failed_test", DurationMs: 50, Success: false},
			},
		}
		formatter := &MarkdownFormatter{}
		var buf bytes.Buffer

		err := formatter.Format(&buf, result)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "❌")
		assert.Contains(t, buf.String(), "failed_test")
	})
}
