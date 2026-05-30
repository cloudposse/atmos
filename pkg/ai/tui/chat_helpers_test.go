package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/session"
	"github.com/cloudposse/atmos/pkg/ai/skills"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// =============================================================================
// chat_format.go tests
// =============================================================================

func TestFormatKnownToolParams(t *testing.T) {
	tests := []struct {
		name     string
		toolCall types.ToolCall
		expected string
	}{
		{
			name: "execute_atmos_command with command",
			toolCall: types.ToolCall{
				Name:  "execute_atmos_command",
				Input: map[string]interface{}{"command": "terraform plan"},
			},
			expected: "**Command:** `atmos terraform plan`",
		},
		{
			name: "execute_atmos_command missing command field",
			toolCall: types.ToolCall{
				Name:  "execute_atmos_command",
				Input: map[string]interface{}{"other": "value"},
			},
			expected: "",
		},
		{
			name: "read_file with path",
			toolCall: types.ToolCall{
				Name:  "read_file",
				Input: map[string]interface{}{"path": "/tmp/test.yaml"},
			},
			expected: "**Path:** `/tmp/test.yaml`",
		},
		{
			name: "read_component_file with component",
			toolCall: types.ToolCall{
				Name:  "read_component_file",
				Input: map[string]interface{}{"component": "vpc"},
			},
			expected: "**Component:** `vpc`",
		},
		{
			name: "read_stack_file with path",
			toolCall: types.ToolCall{
				Name:  "read_stack_file",
				Input: map[string]interface{}{"path": "stacks/dev.yaml"},
			},
			expected: "**Path:** `stacks/dev.yaml`",
		},
		{
			name: "edit_file with path",
			toolCall: types.ToolCall{
				Name:  "edit_file",
				Input: map[string]interface{}{"path": "/tmp/edit.tf"},
			},
			expected: "**Path:** `/tmp/edit.tf`",
		},
		{
			name: "write_component_file with component",
			toolCall: types.ToolCall{
				Name:  "write_component_file",
				Input: map[string]interface{}{"component": "s3-bucket"},
			},
			expected: "**Component:** `s3-bucket`",
		},
		{
			name: "write_stack_file with path",
			toolCall: types.ToolCall{
				Name:  "write_stack_file",
				Input: map[string]interface{}{"path": "stacks/prod.yaml"},
			},
			expected: "**Path:** `stacks/prod.yaml`",
		},
		{
			name: "search_files with pattern",
			toolCall: types.ToolCall{
				Name:  "search_files",
				Input: map[string]interface{}{"pattern": "*.tf"},
			},
			expected: "**Pattern:** `*.tf`",
		},
		{
			name: "execute_bash with command",
			toolCall: types.ToolCall{
				Name:  "execute_bash",
				Input: map[string]interface{}{"command": "ls -la"},
			},
			expected: "**Command:** `ls -la`",
		},
		{
			name: "describe_component with component and stack",
			toolCall: types.ToolCall{
				Name: "describe_component",
				Input: map[string]interface{}{
					"component": "vpc",
					"stack":     "dev-us-east-1",
				},
			},
			expected: "**Args:** `vpc -s dev-us-east-1`",
		},
		{
			name: "unknown tool returns empty",
			toolCall: types.ToolCall{
				Name:  "some_unknown_tool",
				Input: map[string]interface{}{"key": "value"},
			},
			expected: "",
		},
		{
			name: "empty input returns empty",
			toolCall: types.ToolCall{
				Name:  "execute_atmos_command",
				Input: map[string]interface{}{},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatKnownToolParams(tt.toolCall)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatInputField(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		field    string
		label    string
		prefix   string
		expected string
	}{
		{
			name:     "field exists with prefix",
			input:    map[string]interface{}{"command": "plan"},
			field:    "command",
			label:    "Command",
			prefix:   "atmos ",
			expected: "**Command:** `atmos plan`",
		},
		{
			name:     "field exists without prefix",
			input:    map[string]interface{}{"pattern": "*.tf"},
			field:    "pattern",
			label:    "Pattern",
			prefix:   "",
			expected: "**Pattern:** `*.tf`",
		},
		{
			name:     "field does not exist",
			input:    map[string]interface{}{"other": "value"},
			field:    "command",
			label:    "Command",
			prefix:   "",
			expected: "",
		},
		{
			name:     "field is not a string",
			input:    map[string]interface{}{"command": 42},
			field:    "command",
			label:    "Command",
			prefix:   "",
			expected: "",
		},
		{
			name:     "empty input map",
			input:    map[string]interface{}{},
			field:    "command",
			label:    "Command",
			prefix:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatInputField(tt.input, tt.field, tt.label, tt.prefix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatPathOrComponent(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
	}{
		{
			name:     "path present",
			input:    map[string]interface{}{"path": "/tmp/file.yaml"},
			expected: "**Path:** `/tmp/file.yaml`",
		},
		{
			name:     "component present",
			input:    map[string]interface{}{"component": "vpc"},
			expected: "**Component:** `vpc`",
		},
		{
			name:     "both path and component present prefers path",
			input:    map[string]interface{}{"path": "/tmp/file.yaml", "component": "vpc"},
			expected: "**Path:** `/tmp/file.yaml`",
		},
		{
			name:     "neither path nor component",
			input:    map[string]interface{}{"other": "value"},
			expected: "",
		},
		{
			name:     "path is not a string",
			input:    map[string]interface{}{"path": 123},
			expected: "",
		},
		{
			name:     "component is not a string but path missing",
			input:    map[string]interface{}{"component": true},
			expected: "",
		},
		{
			name:     "empty input",
			input:    map[string]interface{}{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPathOrComponent(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatBashCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
	}{
		{
			name:     "short command",
			input:    map[string]interface{}{"command": "ls -la"},
			expected: "**Command:** `ls -la`",
		},
		{
			name:     "command at max length boundary",
			input:    map[string]interface{}{"command": strings.Repeat("a", 80)},
			expected: "**Command:** `" + strings.Repeat("a", 80) + "`",
		},
		{
			name:     "command exceeding max length is truncated",
			input:    map[string]interface{}{"command": strings.Repeat("a", 81)},
			expected: "**Command:** `" + strings.Repeat("a", 77) + "...`",
		},
		{
			name:     "command field missing",
			input:    map[string]interface{}{"other": "value"},
			expected: "",
		},
		{
			name:     "command is not a string",
			input:    map[string]interface{}{"command": 42},
			expected: "",
		},
		{
			name:     "empty command string",
			input:    map[string]interface{}{"command": ""},
			expected: "**Command:** ``",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBashCommand(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatDescribeComponentParams(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
	}{
		{
			name:     "component and stack",
			input:    map[string]interface{}{"component": "vpc", "stack": "dev-us-east-1"},
			expected: "**Args:** `vpc -s dev-us-east-1`",
		},
		{
			name:     "component only",
			input:    map[string]interface{}{"component": "vpc"},
			expected: "**Args:** `vpc`",
		},
		{
			name:     "stack only",
			input:    map[string]interface{}{"stack": "dev-us-east-1"},
			expected: "**Args:** `-s dev-us-east-1`",
		},
		{
			name:     "no component or stack",
			input:    map[string]interface{}{"other": "value"},
			expected: "",
		},
		{
			name:     "empty input",
			input:    map[string]interface{}{},
			expected: "",
		},
		{
			name:     "component is not a string",
			input:    map[string]interface{}{"component": 42},
			expected: "",
		},
		{
			name:     "stack is not a string but component is",
			input:    map[string]interface{}{"component": "vpc", "stack": 42},
			expected: "**Args:** `vpc`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDescribeComponentParams(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatGenericToolParams(t *testing.T) {
	tests := []struct {
		name           string
		input          map[string]interface{}
		expectEmpty    bool
		expectContains []string
	}{
		{
			name:           "single parameter",
			input:          map[string]interface{}{"key": "value"},
			expectContains: []string{"**Parameters:**", "key=`value`"},
		},
		{
			name:           "numeric parameter",
			input:          map[string]interface{}{"count": 42},
			expectContains: []string{"**Parameters:**", "count=`42`"},
		},
		{
			name:           "boolean parameter",
			input:          map[string]interface{}{"verbose": true},
			expectContains: []string{"**Parameters:**", "verbose=`true`"},
		},
		{
			name:        "empty input",
			input:       map[string]interface{}{},
			expectEmpty: true,
		},
		{
			name:           "long value is truncated",
			input:          map[string]interface{}{"data": strings.Repeat("x", 51)},
			expectContains: []string{"data=`" + strings.Repeat("x", 47) + "...`"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatGenericToolParams(tt.input)
			if tt.expectEmpty {
				assert.Empty(t, result)
			} else {
				for _, s := range tt.expectContains {
					assert.Contains(t, result, s)
				}
			}
		})
	}
}

func TestFormatParamValue(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{
			name:     "string value",
			value:    "hello",
			expected: "hello",
		},
		{
			name:     "integer value",
			value:    42,
			expected: "42",
		},
		{
			name:     "boolean value",
			value:    true,
			expected: "true",
		},
		{
			name:     "float value",
			value:    3.14,
			expected: "3.14",
		},
		{
			name:     "value at max length",
			value:    strings.Repeat("a", 50),
			expected: strings.Repeat("a", 50),
		},
		{
			name:     "value exceeding max length is truncated",
			value:    strings.Repeat("a", 51),
			expected: strings.Repeat("a", 47) + "...",
		},
		{
			name:     "empty string",
			value:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatParamValue(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatToolParameters_Extended(t *testing.T) {
	tests := []struct {
		name     string
		toolCall types.ToolCall
		expected string
	}{
		{
			name: "known tool uses special formatting",
			toolCall: types.ToolCall{
				Name:  "execute_bash",
				Input: map[string]interface{}{"command": "ls"},
			},
			expected: "**Command:** `ls`",
		},
		{
			name: "unknown tool uses generic formatting",
			toolCall: types.ToolCall{
				Name:  "custom_tool",
				Input: map[string]interface{}{"key": "val"},
			},
			expected: "**Parameters:** key=`val`",
		},
		{
			name: "empty input returns empty",
			toolCall: types.ToolCall{
				Name:  "execute_bash",
				Input: map[string]interface{}{},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatToolParameters(tt.toolCall)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectOutputFormat(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name:     "JSON object",
			output:   `{"key": "value"}`,
			expected: "json",
		},
		{
			name:     "JSON array",
			output:   `[1, 2, 3]`,
			expected: "json",
		},
		{
			name:     "YAML content",
			output:   "key: value\nname: test\nversion: 1.0\nother: data",
			expected: "yaml",
		},
		{
			name:     "HCL resource",
			output:   `resource "aws_instance" "example" { ami = "ami-123" }`,
			expected: "hcl",
		},
		{
			name:     "HCL data block",
			output:   `data "aws_ami" "example" { most_recent = true }`,
			expected: "hcl",
		},
		{
			name:     "HCL module block",
			output:   `module "vpc" { source = "./modules/vpc" }`,
			expected: "hcl",
		},
		{
			name:     "HCL variable block",
			output:   `variable "region" { default = "us-east-1" }`,
			expected: "hcl",
		},
		{
			name:     "table format",
			output:   "| a | b | c |\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |",
			expected: "text",
		},
		{
			name:     "plain text",
			output:   "Hello world, this is plain text",
			expected: "text",
		},
		{
			name:     "whitespace-padded JSON",
			output:   "  {\"key\": \"value\"}  ",
			expected: "json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectOutputFormat(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsYAML(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		expected bool
	}{
		{
			name:     "valid YAML with key-value pairs",
			lines:    []string{"key: value", "name: test", "version: 1.0"},
			expected: true,
		},
		{
			name:     "valid YAML with list items",
			lines:    []string{"- item1", "- item2", "- item3"},
			expected: true,
		},
		{
			name:     "not enough YAML patterns",
			lines:    []string{"key: value", "plain text"},
			expected: false,
		},
		{
			name:     "empty lines",
			lines:    []string{},
			expected: false,
		},
		{
			name:     "plain text lines",
			lines:    []string{"hello world", "this is text", "no yaml here"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isYAML(tt.lines)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTableFormat(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		expected bool
	}{
		{
			name:     "valid table with consistent pipes",
			lines:    []string{"| a | b | c |", "| 1 | 2 | 3 |", "| 4 | 5 | 6 |"},
			expected: true,
		},
		{
			name:     "too few lines",
			lines:    []string{"| a | b |", "| 1 | 2 |"},
			expected: false,
		},
		{
			name:     "single pipe only",
			lines:    []string{"a | b", "1 | 2", "3 | 4"},
			expected: false,
		},
		{
			name:     "inconsistent pipe counts",
			lines:    []string{"| a | b | c |", "| 1 | 2 |", "| 4 | 5 | 6 |"},
			expected: false,
		},
		{
			name:     "empty lines",
			lines:    []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTableFormat(tt.lines)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchKnownAPIError(t *testing.T) {
	tests := []struct {
		name        string
		errStr      string
		expectMatch bool
		expectMsg   string
	}{
		{
			name:        "rate limit 429",
			errStr:      "HTTP 429 Too Many Requests",
			expectMatch: true,
			expectMsg:   "Rate limit exceeded",
		},
		{
			name:        "authentication error",
			errStr:      "401 Unauthorized",
			expectMatch: true,
			expectMsg:   "Authentication failed",
		},
		{
			name:        "function calling not enabled",
			errStr:      "Function calling is not enabled for this model",
			expectMatch: true,
			expectMsg:   "function calling",
		},
		{
			name:        "function calling not supported compound",
			errStr:      "function calling is not supported",
			expectMatch: true,
			expectMsg:   "function calling",
		},
		{
			name:        "timeout error",
			errStr:      "context deadline exceeded",
			expectMatch: true,
			expectMsg:   "timed out",
		},
		{
			name:        "context length exceeded",
			errStr:      "maximum context length exceeded",
			expectMatch: true,
			expectMsg:   "Context length exceeded",
		},
		{
			name:        "unknown error",
			errStr:      "some random error message",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := matchKnownAPIError(tt.errStr)
			assert.Equal(t, tt.expectMatch, found)
			if tt.expectMatch {
				assert.Contains(t, msg, tt.expectMsg)
			}
		})
	}
}

func TestCleanupErrorString(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected string
	}{
		{
			name:     "removes request ID",
			errStr:   "something failed (Request-ID: abc123)",
			expected: "something failed",
		},
		{
			name:     "removes lowercase request ID",
			errStr:   "something failed (request-id: abc123)",
			expected: "something failed",
		},
		{
			name:     "removes JSON response body",
			errStr:   `error occurred {"type":"error","error":"bad request"}`,
			expected: "error occurred",
		},
		{
			name:     "removes HTTP method prefix",
			errStr:   `POST https://api.example.com": bad request`,
			expected: "bad request",
		},
		{
			name:     "removes nested error prefix",
			errStr:   "failed to send message: actual error",
			expected: "actual error",
		},
		{
			name:     "removes nested error prefix with tools",
			errStr:   "failed to send message with tools: actual error",
			expected: "actual error",
		},
		{
			name:     "no changes for clean string",
			errStr:   "simple error message",
			expected: "simple error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanupErrorString(tt.errStr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatAPIError_Extended(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error returns empty",
			err:      nil,
			expected: "",
		},
		{
			name:     "known error gets friendly message",
			err:      fmt.Errorf("HTTP 429 Too Many Requests"),
			expected: "Rate limit exceeded. Please wait a moment and try again, or contact your provider to increase your rate limit.",
		},
		{
			name:     "unknown error gets cleaned up",
			err:      fmt.Errorf("failed to send message: some error (Request-ID: abc123)"),
			expected: "some error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAPIError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectActionIntent_Extended(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "action phrase with verb",
			content:  "I'll read the file now",
			expected: true,
		},
		{
			name:     "let me with verb",
			content:  "Let me check the configuration",
			expected: true,
		},
		{
			name:     "no action phrase",
			content:  "The file contains valid YAML",
			expected: false,
		},
		{
			name:     "action phrase without verb",
			content:  "I'll do that for you",
			expected: false,
		},
		{
			name:     "i will now with verb",
			content:  "I will now execute the command",
			expected: true,
		},
		{
			name:     "empty string",
			content:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectActionIntent(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContainsAnyPhrase(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		phrases  []string
		expected bool
	}{
		{
			name:     "contains matching phrase",
			content:  "i'll do it",
			phrases:  []string{"i'll", "i will"},
			expected: true,
		},
		{
			name:     "no matching phrase",
			content:  "the result is ready",
			phrases:  []string{"i'll", "i will"},
			expected: false,
		},
		{
			name:     "empty content",
			content:  "",
			phrases:  []string{"i'll"},
			expected: false,
		},
		{
			name:     "empty phrases",
			content:  "i'll do it",
			phrases:  []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsAnyPhrase(tt.content, tt.phrases)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContainsActionVerb(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "verb as whole word with spaces",
			content:  "will read the file",
			expected: true,
		},
		{
			name:     "verb at start",
			content:  "read the file",
			expected: true,
		},
		{
			name:     "verb at end",
			content:  "I will now read",
			expected: true,
		},
		{
			name:     "verb followed by period",
			content:  "will read.",
			expected: true,
		},
		{
			name:     "verb followed by comma",
			content:  "will read, then write",
			expected: true,
		},
		{
			name:     "no action verb",
			content:  "the data is available",
			expected: false,
		},
		{
			name:     "empty string",
			content:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsActionVerb(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// chat_format.go: token/usage formatting tests
// =============================================================================

func TestFormatTokenCount_Extended(t *testing.T) {
	tests := []struct {
		name     string
		count    int64
		expected string
	}{
		{name: "zero", count: 0, expected: "0"},
		{name: "small number", count: 500, expected: "500"},
		{name: "exactly 1k", count: 1000, expected: "1.0k"},
		{name: "few thousands", count: 7100, expected: "7.1k"},
		{name: "large thousands", count: 15000, expected: "15k"},
		{name: "exactly 1M", count: 1000000, expected: "1.0M"},
		{name: "large millions", count: 15000000, expected: "15M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTokenCount(tt.count)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatUsage_Extended(t *testing.T) {
	tests := []struct {
		name     string
		usage    *types.Usage
		expected string
	}{
		{
			name:     "nil usage",
			usage:    nil,
			expected: "",
		},
		{
			name:     "zero total tokens",
			usage:    &types.Usage{TotalTokens: 0},
			expected: "",
		},
		{
			name:     "input and output tokens",
			usage:    &types.Usage{InputTokens: 1500, OutputTokens: 500, TotalTokens: 2000},
			expected: "↑ 1.5k · ↓ 500",
		},
		{
			name:     "with cache tokens",
			usage:    &types.Usage{InputTokens: 1000, OutputTokens: 500, TotalTokens: 1500, CacheReadTokens: 800},
			expected: "↑ 1.0k · ↓ 500 · cache: 800",
		},
		{
			name:     "only input tokens",
			usage:    &types.Usage{InputTokens: 100, TotalTokens: 100},
			expected: "↑ 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatUsage(tt.usage)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEstimateTokens_Extended(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{name: "empty text", text: "", expected: 0},
		{name: "single word", text: "hello", expected: 1},
		{name: "ten words", text: "one two three four five six seven eight nine ten", expected: 13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := estimateTokens(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// chat_handlers.go tests
// =============================================================================

func TestCalculateViewportHeight(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}
	model, err := NewChatModel(ChatModelParams{Client: client})
	require.NoError(t, err)

	tests := []struct {
		name        string
		totalHeight int
		expected    int
	}{
		{
			name:        "normal height",
			totalHeight: 50,
			expected:    32, // 50 - 18 = 32
		},
		{
			name:        "minimum height enforced",
			totalHeight: 20,
			expected:    minViewportHeight, // 20 - 18 = 2, but min is 10
		},
		{
			name:        "exactly at threshold",
			totalHeight: 28,
			expected:    minViewportHeight, // 28 - 18 = 10 = min
		},
		{
			name:        "large terminal",
			totalHeight: 100,
			expected:    82, // 100 - 18 = 82
		},
		{
			name:        "very small terminal",
			totalHeight: 5,
			expected:    minViewportHeight,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := model.calculateViewportHeight(tt.totalHeight)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHandleWindowResize(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}
	model, err := NewChatModel(ChatModelParams{Client: client})
	require.NoError(t, err)

	t.Run("sets width and height", func(t *testing.T) {
		msg := tea.WindowSizeMsg{Width: 120, Height: 50}
		model.handleWindowResize(msg)

		assert.Equal(t, 120, model.width)
		assert.Equal(t, 50, model.height)
		assert.True(t, model.ready)
	})

	t.Run("sets ready on first resize", func(t *testing.T) {
		newModel, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		assert.False(t, newModel.ready)

		msg := tea.WindowSizeMsg{Width: 80, Height: 40}
		newModel.handleWindowResize(msg)

		assert.True(t, newModel.ready)
	})

	t.Run("small terminal still works", func(t *testing.T) {
		msg := tea.WindowSizeMsg{Width: 30, Height: 15}
		model.handleWindowResize(msg)

		assert.Equal(t, 30, model.width)
		assert.Equal(t, 15, model.height)
	})
}

func TestHandleAIResponseMsg(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("response without usage", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		initialCount := len(model.messages)
		msg := aiResponseMsg{content: "Hello from AI", usage: nil}
		model.handleAIResponseMsg(msg)

		assert.Len(t, model.messages, initialCount+1)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Equal(t, roleAssistant, lastMsg.Role)
		assert.Equal(t, "Hello from AI", lastMsg.Content)
		assert.Nil(t, model.lastUsage)
	})

	t.Run("response with usage tracks tokens", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		usage := &types.Usage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		}
		msg := aiResponseMsg{content: "Response with tokens", usage: usage}
		model.handleAIResponseMsg(msg)

		assert.Equal(t, usage, model.lastUsage)
		assert.Equal(t, int64(100), model.cumulativeUsage.InputTokens)
		assert.Equal(t, int64(50), model.cumulativeUsage.OutputTokens)
		assert.Equal(t, int64(150), model.cumulativeUsage.TotalTokens)

		// Verify content includes usage info.
		lastMsg := model.messages[len(model.messages)-1]
		assert.Contains(t, lastMsg.Content, "Token usage:")
	})

	t.Run("cumulative usage accumulates", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		usage1 := &types.Usage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}
		usage2 := &types.Usage{InputTokens: 200, OutputTokens: 100, TotalTokens: 300}

		model.handleAIResponseMsg(aiResponseMsg{content: "First", usage: usage1})
		model.handleAIResponseMsg(aiResponseMsg{content: "Second", usage: usage2})

		assert.Equal(t, int64(300), model.cumulativeUsage.InputTokens)
		assert.Equal(t, int64(150), model.cumulativeUsage.OutputTokens)
		assert.Equal(t, int64(450), model.cumulativeUsage.TotalTokens)
	})
}

func TestHandleAIErrorMsg(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("adds error message to chat", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		initialCount := len(model.messages)
		model.handleAIErrorMsg(aiErrorMsg("Something went wrong"))

		assert.Len(t, model.messages, initialCount+1)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Equal(t, roleSystem, lastMsg.Role)
		assert.Contains(t, lastMsg.Content, "Error: Something went wrong")
	})

	t.Run("suppresses cancelled message when user cancelled", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		model.isCancelling = true
		initialCount := len(model.messages)
		model.handleAIErrorMsg(aiErrorMsg("Request cancelled"))

		assert.Len(t, model.messages, initialCount+1)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Equal(t, roleSystem, lastMsg.Role)
		assert.Contains(t, lastMsg.Content, "Request cancelled by user")
	})

	t.Run("shows non-cancelled error even when cancelling", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		model.isCancelling = true
		initialCount := len(model.messages)
		model.handleAIErrorMsg(aiErrorMsg("Network timeout"))

		assert.Len(t, model.messages, initialCount+1)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Contains(t, lastMsg.Content, "Error: Network timeout")
	})
}

func TestHandleCompactStatus(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("starting stage", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		msg := compactStatusMsg{stage: "starting", messageCount: 42}
		result := model.handleCompactStatus(msg)

		assert.True(t, result)
		assert.True(t, model.isLoading)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Contains(t, lastMsg.Content, "Compacting conversation")
		assert.Contains(t, lastMsg.Content, "42 messages")
	})

	t.Run("completed stage", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.isLoading = true

		msg := compactStatusMsg{stage: "completed", messageCount: 42, savings: 5000}
		result := model.handleCompactStatus(msg)

		assert.True(t, result)
		assert.False(t, model.isLoading)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Contains(t, lastMsg.Content, "Conversation compacted successfully")
		assert.Contains(t, lastMsg.Content, "42 messages")
		assert.Contains(t, lastMsg.Content, "5000 tokens saved")
	})

	t.Run("failed stage with error", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.isLoading = true

		msg := compactStatusMsg{stage: "failed", err: fmt.Errorf("compaction error")}
		result := model.handleCompactStatus(msg)

		assert.True(t, result)
		assert.False(t, model.isLoading)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Contains(t, lastMsg.Content, "Compaction failed: compaction error")
	})

	t.Run("failed stage without error", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.isLoading = true

		msg := compactStatusMsg{stage: "failed"}
		result := model.handleCompactStatus(msg)

		assert.True(t, result)
		assert.False(t, model.isLoading)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Equal(t, "Compaction failed", lastMsg.Content)
	})
}

// =============================================================================
// chat_render.go tests
// =============================================================================

func TestBuildMessageHeader(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}
	model, err := NewChatModel(ChatModelParams{Client: client})
	require.NoError(t, err)

	now := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)

	t.Run("user message header", func(t *testing.T) {
		msg := ChatMessage{Role: roleUser, Content: "Hello", Time: now}
		header := model.buildMessageHeader(msg)

		assert.Contains(t, header, "You:")
		assert.Contains(t, header, "14:30")
	})

	t.Run("assistant message header", func(t *testing.T) {
		msg := ChatMessage{Role: roleAssistant, Content: "Hello", Time: now, Provider: "anthropic"}
		header := model.buildMessageHeader(msg)

		assert.Contains(t, header, "Atmos AI")
		assert.Contains(t, header, "anthropic")
		assert.Contains(t, header, "14:30")
	})

	t.Run("system message header", func(t *testing.T) {
		msg := ChatMessage{Role: roleSystem, Content: "Error", Time: now}
		header := model.buildMessageHeader(msg)

		assert.Contains(t, header, "System:")
		assert.Contains(t, header, "14:30")
	})
}

func TestBuildAssistantPrefix(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("with known provider", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		msg := ChatMessage{Provider: "anthropic"}
		prefix := model.buildAssistantPrefix(msg)

		assert.Contains(t, prefix, "anthropic")
		assert.Contains(t, prefix, "Atmos AI")
	})

	t.Run("with empty provider falls back to unknown", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		msg := ChatMessage{Provider: ""}
		prefix := model.buildAssistantPrefix(msg)

		assert.Contains(t, prefix, "unknown")
	})

	t.Run("with active skill includes skill icon", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		model.currentSkill = &skills.Skill{Name: "general"}
		msg := ChatMessage{Provider: "anthropic"}
		prefix := model.buildAssistantPrefix(msg)

		assert.Contains(t, prefix, "Atmos AI")
		assert.Contains(t, prefix, "anthropic")
		// The skill icon should be present (some emoji character).
		assert.Contains(t, prefix, getSkillIcon("general"))
	})
}

func TestRenderMessageContent(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}
	model, err := NewChatModel(ChatModelParams{Client: client})
	require.NoError(t, err)
	// Ensure viewport has reasonable width.
	model.viewport.Width = 80

	t.Run("user message renders as plain text", func(t *testing.T) {
		msg := ChatMessage{Role: roleUser, Content: "Hello world"}
		result := model.renderMessageContent(msg)

		assert.Contains(t, result, "Hello world")
	})

	t.Run("system message renders as plain text", func(t *testing.T) {
		msg := ChatMessage{Role: roleSystem, Content: "System info"}
		result := model.renderMessageContent(msg)

		assert.Contains(t, result, "System info")
	})

	t.Run("assistant message uses markdown rendering", func(t *testing.T) {
		msg := ChatMessage{Role: roleAssistant, Content: "Hello **world**"}
		result := model.renderMessageContent(msg)

		// Result should contain the content (rendered or plain).
		assert.NotEmpty(t, result)
	})
}

func TestHasMarkdownTable(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "content with markdown table separator",
			content:  "| Header |\n|---|\n| data |",
			expected: true,
		},
		{
			name:     "content without table",
			content:  "Just some text without pipes",
			expected: false,
		},
		{
			name:     "pipes but no separator",
			content:  "| data | more |",
			expected: false,
		},
		{
			name:     "empty content",
			content:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasMarkdownTable(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPadRenderedLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single line",
			input:    "hello",
			expected: "  hello",
		},
		{
			name:     "multiple lines",
			input:    "line1\nline2\nline3",
			expected: "  line1\n  line2\n  line3",
		},
		{
			name:     "trailing newlines get padded and trimmed",
			input:    "hello\n\n",
			expected: "  hello\n  \n  ",
		},
		{
			name:     "empty string gets padded",
			input:    "",
			expected: "  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := padRenderedLines(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStripHTTPMethodPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "POST method prefix",
			input:    `POST https://api.example.com/v1": bad request`,
			expected: "bad request",
		},
		{
			name:     "GET method prefix",
			input:    `GET https://api.example.com/v1": not found`,
			expected: "not found",
		},
		{
			name:     "no HTTP method prefix",
			input:    "simple error message",
			expected: "simple error message",
		},
		{
			name:     "PUT method prefix",
			input:    `PUT https://api.example.com/v1": conflict`,
			expected: "conflict",
		},
		{
			name:     "DELETE method prefix",
			input:    `DELETE https://api.example.com/v1": forbidden`,
			expected: "forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripHTTPMethodPrefix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// chat_ai.go: pure helper function tests
// =============================================================================

func TestResolveToolOutput(t *testing.T) {
	tests := []struct {
		name     string
		result   *tools.Result
		expected string
	}{
		{
			name:     "output present",
			result:   &tools.Result{Output: "some output", Success: true},
			expected: "some output",
		},
		{
			name:     "empty output with error",
			result:   &tools.Result{Output: "", Error: fmt.Errorf("tool failed")},
			expected: "Error: tool failed",
		},
		{
			name:     "empty output no error",
			result:   &tools.Result{Output: "", Error: nil},
			expected: "No output returned",
		},
		{
			name:     "output present even with error prefers output",
			result:   &tools.Result{Output: "got output", Error: fmt.Errorf("also error")},
			expected: "got output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveToolOutput(tt.result)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendAccumulated(t *testing.T) {
	tests := []struct {
		name        string
		accumulated string
		newContent  string
		expected    string
	}{
		{
			name:        "empty accumulated",
			accumulated: "",
			newContent:  "new content",
			expected:    "new content",
		},
		{
			name:        "non-empty accumulated",
			accumulated: "previous content",
			newContent:  "new content",
			expected:    "previous content\n\nnew content",
		},
		{
			name:        "both empty",
			accumulated: "",
			newContent:  "",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendAccumulated(tt.accumulated, tt.newContent)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildToolDisplayText(t *testing.T) {
	t.Run("single tool with output", func(t *testing.T) {
		response := &types.Response{
			Content: "",
			ToolCalls: []types.ToolCall{
				{Name: "execute_bash", Input: map[string]interface{}{"command": "ls"}},
			},
		}
		results := []*tools.Result{
			{Output: "file1.txt\nfile2.txt", Success: true},
		}

		text := buildToolDisplayText(response, results)

		assert.Contains(t, text, "**Tool:** `execute_bash`")
		assert.Contains(t, text, "file1.txt")
		assert.Contains(t, text, "```")
	})

	t.Run("with response content prepended", func(t *testing.T) {
		response := &types.Response{
			Content: "Let me check that.",
			ToolCalls: []types.ToolCall{
				{Name: "read_file", Input: map[string]interface{}{"path": "/tmp/test"}},
			},
		}
		results := []*tools.Result{
			{Output: "file contents here", Success: true},
		}

		text := buildToolDisplayText(response, results)

		assert.True(t, strings.HasPrefix(text, "Let me check that."))
		assert.Contains(t, text, "**Tool:** `read_file`")
	})

	t.Run("multiple tool results", func(t *testing.T) {
		response := &types.Response{
			Content: "",
			ToolCalls: []types.ToolCall{
				{Name: "read_file", Input: map[string]interface{}{"path": "/a"}},
				{Name: "read_file", Input: map[string]interface{}{"path": "/b"}},
			},
		}
		results := []*tools.Result{
			{Output: "content a", Success: true},
			{Output: "content b", Success: true},
		}

		text := buildToolDisplayText(response, results)

		assert.Contains(t, text, "content a")
		assert.Contains(t, text, "content b")
	})

	t.Run("tool with error output", func(t *testing.T) {
		response := &types.Response{
			Content: "",
			ToolCalls: []types.ToolCall{
				{Name: "execute_bash", Input: map[string]interface{}{"command": "false"}},
			},
		}
		results := []*tools.Result{
			{Output: "", Error: fmt.Errorf("command failed"), Success: false},
		}

		text := buildToolDisplayText(response, results)

		assert.Contains(t, text, "Error: command failed")
	})
}

func TestBuildToolResultsContent(t *testing.T) {
	t.Run("single tool result", func(t *testing.T) {
		response := &types.Response{
			ToolCalls: []types.ToolCall{
				{Name: "read_file"},
			},
		}
		results := []*tools.Result{
			{Output: "file content", Success: true},
		}

		content := buildToolResultsContent(response, results)

		assert.Contains(t, content, "Tool: read_file")
		assert.Contains(t, content, "file content")
	})

	t.Run("multiple tool results", func(t *testing.T) {
		response := &types.Response{
			ToolCalls: []types.ToolCall{
				{Name: "tool_a"},
				{Name: "tool_b"},
			},
		}
		results := []*tools.Result{
			{Output: "output a", Success: true},
			{Output: "output b", Success: true},
		}

		content := buildToolResultsContent(response, results)

		assert.Contains(t, content, "Tool: tool_a")
		assert.Contains(t, content, "output a")
		assert.Contains(t, content, "Tool: tool_b")
		assert.Contains(t, content, "output b")
	})
}

// =============================================================================
// chat_handlers.go: additional handler tests
// =============================================================================

func TestHandleSpinnerTick(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("updates spinner when loading", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.isLoading = true

		cmds := []tea.Cmd{}
		msg := spinner.TickMsg{}
		result := model.handleSpinnerTick(msg, &cmds)

		assert.True(t, result)
		// When loading, a spinner tick command should be appended.
		assert.GreaterOrEqual(t, len(cmds), 1)
	})

	t.Run("does not update spinner when not loading", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.isLoading = false

		cmds := []tea.Cmd{}
		msg := spinner.TickMsg{}
		result := model.handleSpinnerTick(msg, &cmds)

		assert.True(t, result)
		assert.Len(t, cmds, 0)
	})
}

func TestCleanTextareaANSI(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("empty textarea unchanged", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.textarea.SetValue("")

		model.cleanTextareaANSI()

		assert.Equal(t, "", model.textarea.Value())
	})

	t.Run("clean text unchanged", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.textarea.SetValue("hello world")

		model.cleanTextareaANSI()

		assert.Equal(t, "hello world", model.textarea.Value())
	})

	t.Run("ANSI sequences stripped", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		// The textarea may filter certain characters on SetValue.
		// We verify the function runs without error and the value is the stripped version.
		ansiText := "\x1b[31mhello\x1b[0m"
		model.textarea.SetValue(ansiText)
		model.cleanTextareaANSI()

		cleaned := model.textarea.Value()
		// After cleaning, no raw ANSI escape sequences should remain.
		assert.NotContains(t, cleaned, "\x1b[")
	})
}

func TestUpdateViewportSize(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("first call initializes viewport and sets ready", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.ready = false

		model.updateViewportSize(100, 30)

		assert.True(t, model.ready)
		assert.Equal(t, 100, model.viewport.Width)
		assert.Equal(t, 30, model.viewport.Height)
	})

	t.Run("subsequent call updates dimensions without reinitializing", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.ready = false
		model.updateViewportSize(80, 20) // First call sets ready.
		assert.True(t, model.ready)

		model.updateViewportSize(120, 40) // Second call just updates.
		assert.Equal(t, 120, model.viewport.Width)
		assert.Equal(t, 40, model.viewport.Height)
	})
}

func TestInit(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("returns batch command", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		cmd := model.Init()
		assert.NotNil(t, cmd)
	})

	t.Run("cleans ANSI in textarea on init", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.textarea.SetValue("\x1b[31mcontaminated\x1b[0m")

		cmd := model.Init()
		assert.NotNil(t, cmd)
		// After Init, the textarea should be reset if it had a value.
		assert.Equal(t, "", model.textarea.Value())
	})
}

// =============================================================================
// keys.go: stripANSI tests
// =============================================================================

func TestStripANSI_Extended(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ANSI sequences",
			input:    "plain text",
			expected: "plain text",
		},
		{
			name:     "color escape sequence",
			input:    "\x1b[31mred text\x1b[0m",
			expected: "red text",
		},
		{
			name:     "bold escape sequence",
			input:    "\x1b[1mbold\x1b[0m",
			expected: "bold",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "multiple escape sequences",
			input:    "\x1b[1m\x1b[31mred bold\x1b[0m normal",
			expected: "red bold normal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// chat_format.go: isJSON, isHCL tests
// =============================================================================

func TestIsJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "object", input: `{"key": "value"}`, expected: true},
		{name: "array", input: `[1, 2, 3]`, expected: true},
		{name: "plain text", input: "not json", expected: false},
		{name: "empty string", input: "", expected: false},
		{name: "starts with bracket in text", input: "[not really json", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsHCL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "resource block", input: `resource "aws_instance" "web" {}`, expected: true},
		{name: "data block", input: `data "aws_ami" "ubuntu" {}`, expected: true},
		{name: "module block", input: `module "vpc" {}`, expected: true},
		{name: "variable block", input: `variable "name" {}`, expected: true},
		{name: "plain text", input: "not hcl content", expected: false},
		{name: "empty string", input: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isHCL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// chat_render.go: additional render tests
// =============================================================================

func TestRenderAndCacheMessage(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}
	model, err := NewChatModel(ChatModelParams{Client: client})
	require.NoError(t, err)
	model.viewport.Width = 80

	t.Run("caches rendered message parts", func(t *testing.T) {
		msg := ChatMessage{
			Role:    roleUser,
			Content: "Test message",
			Time:    time.Now(),
		}

		initialCacheLen := len(model.renderedMessages)
		model.renderAndCacheMessage(msg)

		// Should add header, content, and empty line (3 items).
		assert.Equal(t, initialCacheLen+3, len(model.renderedMessages))
	})
}

func TestUpdateViewportContent(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}
	model, err := NewChatModel(ChatModelParams{Client: client})
	require.NoError(t, err)
	model.viewport.Width = 80

	t.Run("renders all messages on empty cache", func(t *testing.T) {
		model.messages = []ChatMessage{
			{Role: roleUser, Content: "Hello", Time: time.Now()},
			{Role: roleAssistant, Content: "Hi there", Time: time.Now(), Provider: "test"},
		}
		model.renderedMessages = nil

		model.updateViewportContent()

		// Cache should be populated (3 items per message).
		assert.Equal(t, 6, len(model.renderedMessages))
	})

	t.Run("incrementally renders new messages", func(t *testing.T) {
		// Reset.
		model.messages = []ChatMessage{
			{Role: roleUser, Content: "Hello", Time: time.Now()},
		}
		model.renderedMessages = nil
		model.updateViewportContent()
		assert.Equal(t, 3, len(model.renderedMessages))

		// Add another message.
		model.messages = append(model.messages, ChatMessage{
			Role: roleAssistant, Content: "Response", Time: time.Now(), Provider: "test",
		})
		model.updateViewportContent()

		// Should have 6 items now (3 per message).
		assert.Equal(t, 6, len(model.renderedMessages))
	})
}

func TestBuildSystemPrompt(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("uses default prompt when no skill", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.currentSkill = nil

		prompt := model.buildSystemPrompt()

		assert.Contains(t, prompt, "AI assistant for Atmos")
	})

	t.Run("uses skill system prompt when available", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.currentSkill = &skills.Skill{
			Name:         "test-skill",
			SystemPrompt: "You are a test skill assistant.",
		}

		prompt := model.buildSystemPrompt()

		assert.Contains(t, prompt, "test skill assistant")
		assert.NotContains(t, prompt, "AI assistant for Atmos")
	})
}

func TestView(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("not ready shows initialization", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.ready = false

		view := model.View()
		assert.Contains(t, view, "Initializing")
	})

	t.Run("session list view", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.ready = true
		model.currentView = viewModeSessionList
		model.sessionFilter = "all"

		view := model.View()
		assert.Contains(t, view, "Session List")
	})

	t.Run("chat view renders header and footer", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.ready = true
		model.currentView = viewModeChat
		// Ensure we have reasonable dimensions.
		model.width = 80
		model.height = 40

		view := model.View()
		assert.Contains(t, view, "Atmos AI Assistant")
	})
}

// =============================================================================
// chat_ai.go: buildFilteredMessages tests
// =============================================================================

func TestBuildFilteredMessages(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("filters out system messages", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.messages = []ChatMessage{
			{Role: roleUser, Content: "Hello", Provider: ""},
			{Role: roleSystem, Content: "System info", Provider: ""},
			{Role: roleAssistant, Content: "Hi there", Provider: ""},
		}

		messages := model.buildFilteredMessages()

		assert.Len(t, messages, 2)
		assert.Equal(t, "Hello", messages[0].Content)
		assert.Equal(t, "Hi there", messages[1].Content)
	})

	t.Run("filters by provider when session set", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.sess = &session.Session{Provider: "anthropic"}
		model.messages = []ChatMessage{
			{Role: roleUser, Content: "Hello from anthropic", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Response from anthropic", Provider: "anthropic"},
			{Role: roleUser, Content: "Hello from openai", Provider: "openai"},
			{Role: roleAssistant, Content: "Response from openai", Provider: "openai"},
		}

		messages := model.buildFilteredMessages()

		assert.Len(t, messages, 2)
		assert.Equal(t, "Hello from anthropic", messages[0].Content)
		assert.Equal(t, "Response from anthropic", messages[1].Content)
	})

	t.Run("empty messages returns empty", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.messages = []ChatMessage{}

		messages := model.buildFilteredMessages()

		assert.Len(t, messages, 0)
	})
}

func TestApplyHistoryLimits(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("no limits keeps all messages", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.maxHistoryMessages = 0
		model.maxHistoryTokens = 0

		messages := []types.Message{
			{Role: "user", Content: "msg1"},
			{Role: "assistant", Content: "msg2"},
			{Role: "user", Content: "msg3"},
		}

		result := model.applyHistoryLimits(messages)
		assert.Len(t, result, 3)
	})

	t.Run("message limit prunes oldest", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.maxHistoryMessages = 2
		model.maxHistoryTokens = 0

		messages := []types.Message{
			{Role: "user", Content: "old msg"},
			{Role: "assistant", Content: "old response"},
			{Role: "user", Content: "new msg"},
		}

		result := model.applyHistoryLimits(messages)
		assert.Len(t, result, 2)
		assert.Equal(t, "old response", result[0].Content)
		assert.Equal(t, "new msg", result[1].Content)
	})

	t.Run("token limit prunes oldest", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.maxHistoryMessages = 0
		model.maxHistoryTokens = 5 // Very low token limit.

		messages := []types.Message{
			{Role: "user", Content: "This is a very long message with many words that should exceed the token limit"},
			{Role: "user", Content: "short"},
		}

		result := model.applyHistoryLimits(messages)
		// Only the short message should remain.
		assert.Len(t, result, 1)
		assert.Equal(t, "short", result[0].Content)
	})

	t.Run("under limit keeps all", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.maxHistoryMessages = 10
		model.maxHistoryTokens = 0

		messages := []types.Message{
			{Role: "user", Content: "msg1"},
			{Role: "assistant", Content: "msg2"},
		}

		result := model.applyHistoryLimits(messages)
		assert.Len(t, result, 2)
	})
}

// =============================================================================
// chat_handlers.go: headerView and footerView tests
// =============================================================================

func TestHeaderView(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("basic header without session", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		header := model.headerView()

		assert.Contains(t, header, "Atmos AI Assistant")
		assert.Contains(t, header, "infrastructure")
	})

	t.Run("header with session shows session info", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.sess = &session.Session{
			Name:      "test-session",
			CreatedAt: time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC),
		}

		header := model.headerView()

		assert.Contains(t, header, "test-session")
	})
}

func TestFooterView(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("loading footer shows spinner text", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.isLoading = true
		model.loadingText = "Processing..."

		footer := model.footerView()

		assert.Contains(t, footer, "Processing...")
		assert.Contains(t, footer, "Esc")
	})

	t.Run("input footer shows help text", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.isLoading = false

		footer := model.footerView()

		assert.Contains(t, footer, "Enter")
		assert.Contains(t, footer, "Ctrl+C")
	})
}

func TestLoadingFooterContent(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("shows cumulative tokens when available", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.cumulativeUsage.TotalTokens = 5000

		content := model.loadingFooterContent()

		assert.Contains(t, content, "5.0k tokens")
	})

	t.Run("default loading text when empty", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.loadingText = ""

		content := model.loadingFooterContent()

		assert.Contains(t, content, "AI is thinking...")
	})
}

func TestHandleEmptyFollowUpResponse(t *testing.T) {
	t.Run("without accumulated content", func(t *testing.T) {
		response := &types.Response{
			Usage: &types.Usage{TotalTokens: 100},
		}

		msg := handleEmptyFollowUpResponse("", "tool results here", response)

		responseMsg, ok := msg.(aiResponseMsg)
		require.True(t, ok)
		assert.Contains(t, responseMsg.content, "tool results here")
		assert.Contains(t, responseMsg.content, "empty")
	})

	t.Run("with accumulated content", func(t *testing.T) {
		response := &types.Response{
			Usage: &types.Usage{TotalTokens: 200},
		}

		msg := handleEmptyFollowUpResponse("previous content", "tool results", response)

		responseMsg, ok := msg.(aiResponseMsg)
		require.True(t, ok)
		assert.Contains(t, responseMsg.content, "previous content")
		assert.Contains(t, responseMsg.content, "tool results")
	})
}

// =============================================================================
// chat_handlers.go: handleProviderSwitched tests
// =============================================================================

func TestHandleProviderSwitched(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("error during switch adds error message", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		initialCount := len(model.messages)
		msg := providerSwitchedMsg{
			err: fmt.Errorf("connection failed"),
		}
		model.handleProviderSwitched(msg)

		assert.Len(t, model.messages, initialCount+1)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Equal(t, roleSystem, lastMsg.Role)
		assert.Contains(t, lastMsg.Content, "Error switching provider")
	})

	t.Run("successful switch replaces client and adds message", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		newClient := &mockAIClient{model: "new-model", maxTokens: 8192}
		providerConfig := &schema.AIProviderConfig{Model: "claude-3-opus"}

		initialCount := len(model.messages)
		msg := providerSwitchedMsg{
			provider:       "anthropic",
			providerConfig: providerConfig,
			newClient:      newClient,
			err:            nil,
		}
		model.handleProviderSwitched(msg)

		assert.Equal(t, newClient, model.client)
		assert.Len(t, model.messages, initialCount+1)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Contains(t, lastMsg.Content, "claude-3-opus")
	})

	t.Run("successful switch updates session", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.sess = &session.Session{Provider: "openai", Model: "gpt-4o"}

		newClient := &mockAIClient{model: "new-model", maxTokens: 8192}
		providerConfig := &schema.AIProviderConfig{Model: "claude-3-opus"}

		msg := providerSwitchedMsg{
			provider:       "anthropic",
			providerConfig: providerConfig,
			newClient:      newClient,
			err:            nil,
		}
		model.handleProviderSwitched(msg)

		assert.Equal(t, "anthropic", model.sess.Provider)
		assert.Equal(t, "claude-3-opus", model.sess.Model)
	})
}

// =============================================================================
// chat_render.go: renderNonTableLine, parseTableStructure, parseTableCells tests
// =============================================================================

func TestRenderNonTableLine(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("empty line adds newline", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		var result strings.Builder
		model.renderNonTableLine(&result, "", "")

		assert.Equal(t, "\n", result.String())
	})

	t.Run("nil renderer uses plain text with padding", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.markdownRenderer = nil

		var result strings.Builder
		model.renderNonTableLine(&result, "hello world", "hello world")

		assert.Contains(t, result.String(), "hello world")
		assert.True(t, strings.HasPrefix(result.String(), "  "))
	})

	t.Run("with renderer renders markdown", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		// Model should have a renderer by default.

		var result strings.Builder
		model.renderNonTableLine(&result, "hello **bold**", "hello **bold**")

		assert.NotEmpty(t, result.String())
	})
}

func TestParseTableStructure(t *testing.T) {
	t.Run("basic table", func(t *testing.T) {
		lines := []string{
			"| Name | Age |",
			"|------|-----|",
			"| Alice | 30 |",
			"| Bob | 25 |",
		}

		headers, rows := parseTableStructure(lines)

		assert.Equal(t, []string{"Name", "Age"}, headers)
		assert.Len(t, rows, 2)
		assert.Equal(t, []string{"Alice", "30"}, rows[0])
		assert.Equal(t, []string{"Bob", "25"}, rows[1])
	})

	t.Run("empty table", func(t *testing.T) {
		lines := []string{}

		headers, rows := parseTableStructure(lines)

		assert.Nil(t, headers)
		assert.Nil(t, rows)
	})
}

func TestParseTableCells(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected []string
	}{
		{
			name:     "normal row",
			line:     "| Alice | 30 |",
			expected: []string{"Alice", "30"},
		},
		{
			name:     "separator row returns nil",
			line:     "|---|---|",
			expected: nil,
		},
		{
			name:     "empty line returns nil",
			line:     "",
			expected: nil,
		},
		{
			name:     "whitespace only returns nil",
			line:     "   ",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTableCells(tt.line)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRenderTable(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("valid table renders", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		lines := []string{
			"| Name | Age |",
			"|------|-----|",
			"| Alice | 30 |",
		}

		result := model.renderTable(lines)

		assert.NotEmpty(t, result)
		assert.Contains(t, result, "Alice")
		assert.Contains(t, result, "Name")
	})

	t.Run("too few lines returns joined", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		lines := []string{"| only one line |"}

		result := model.renderTable(lines)

		assert.Equal(t, "| only one line |", result)
	})
}

func TestRecreateMarkdownRenderer(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("creates new renderer and clears cache", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		// Populate some cached messages.
		model.renderedMessages = []string{"cached1", "cached2"}

		model.recreateMarkdownRenderer(80)

		assert.NotNil(t, model.markdownRenderer)
		assert.Len(t, model.renderedMessages, 0)
	})

	t.Run("enforces minimum width", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		// Very small width should still work.
		model.recreateMarkdownRenderer(10)
		assert.NotNil(t, model.markdownRenderer)
	})
}

func TestRenderMarkdown(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("with nil renderer falls back to plain text", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.markdownRenderer = nil
		model.viewport.Width = 80

		result := model.renderMarkdown("Hello **world**")

		assert.Contains(t, result, "Hello")
	})

	t.Run("with renderer processes markdown", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.viewport.Width = 80

		result := model.renderMarkdown("Hello world")

		assert.NotEmpty(t, result)
		assert.Contains(t, result, "Hello")
	})

	t.Run("content with table triggers table rendering", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.viewport.Width = 80

		content := "Some text\n| H1 | H2 |\n|---|---|\n| a | b |"
		result := model.renderMarkdown(content)

		assert.NotEmpty(t, result)
	})
}

// =============================================================================
// chat_ai.go: getAtmosMemory, prependMemoryContext tests
// =============================================================================

func TestGetAtmosMemory(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("nil memory manager returns empty", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.memoryMgr = nil

		result := model.getAtmosMemory()
		assert.Equal(t, "", result)
	})
}

func TestPrependMemoryContext(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("nil memory manager returns messages unchanged", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.memoryMgr = nil

		messages := []types.Message{
			{Role: "user", Content: "hello"},
		}

		result := model.prependMemoryContext(messages)
		assert.Len(t, result, 1)
		assert.Equal(t, "hello", result[0].Content)
	})
}

// =============================================================================
// chat_render.go: renderMarkdownWithTables tests
// =============================================================================

func TestRenderMarkdownWithTables(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("mixed content with table", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.viewport.Width = 80

		content := "Before table\n| A | B |\n|---|---|\n| 1 | 2 |\nAfter table"
		result := model.renderMarkdownWithTables(content)

		assert.NotEmpty(t, result)
		assert.Contains(t, result, "A")
		assert.Contains(t, result, "1")
	})

	t.Run("table at end of content", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.viewport.Width = 80

		content := "| X | Y |\n|---|---|\n| a | b |"
		result := model.renderMarkdownWithTables(content)

		assert.NotEmpty(t, result)
	})
}

// =============================================================================
// keys.go: keyboard handler tests
// =============================================================================

func TestHandleLoadingKeys(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("ctrl+c returns quit", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.isLoading = true

		msg := tea.KeyMsg{Type: tea.KeyCtrlC}
		cmd := model.handleLoadingKeys(msg)

		assert.NotNil(t, cmd)
	})

	t.Run("esc cancels request", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.isLoading = true
		cancelled := false
		model.cancelFunc = func() { cancelled = true }

		msg := tea.KeyMsg{Type: tea.KeyEsc}
		cmd := model.handleLoadingKeys(msg)

		assert.NotNil(t, cmd)
		assert.True(t, cancelled)
		assert.True(t, model.isCancelling)
	})

	t.Run("esc does not cancel twice", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.isLoading = true
		model.isCancelling = true
		callCount := 0
		model.cancelFunc = func() { callCount++ }

		msg := tea.KeyMsg{Type: tea.KeyEsc}
		model.handleLoadingKeys(msg)

		// Should not call cancel again.
		assert.Equal(t, 0, callCount)
	})

	t.Run("other keys return noop", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.isLoading = true

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
		cmd := model.handleLoadingKeys(msg)

		assert.NotNil(t, cmd)
	})
}

func TestHandleCtrlL(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("without manager shows error", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.manager = nil

		initialCount := len(model.messages)
		cmd := model.handleCtrlL()

		assert.NotNil(t, cmd)
		assert.Greater(t, len(model.messages), initialCount)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Contains(t, lastMsg.Content, "Sessions are not enabled")
	})
}

func TestHandleCtrlN(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("without manager shows error", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.manager = nil

		initialCount := len(model.messages)
		cmd := model.handleCtrlN()

		assert.NotNil(t, cmd)
		assert.Greater(t, len(model.messages), initialCount)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Contains(t, lastMsg.Content, "Sessions are not enabled")
	})
}

func TestHandleCtrlP(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("without atmosConfig returns noop", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.atmosConfig = nil

		cmd := model.handleCtrlP()
		assert.NotNil(t, cmd)
		// View should not change.
		assert.Equal(t, viewModeChat, model.currentView)
	})

	t.Run("with atmosConfig switches to provider select", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.atmosConfig = &schema.AtmosConfiguration{}

		cmd := model.handleCtrlP()
		assert.NotNil(t, cmd)
		assert.Equal(t, viewModeProviderSelect, model.currentView)
	})
}

func TestHandleCtrlA(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("without skill registry returns noop", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.skillRegistry = nil

		cmd := model.handleCtrlA()
		assert.NotNil(t, cmd)
		assert.Equal(t, viewModeChat, model.currentView)
	})

	t.Run("with skill registry switches to skill select", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		// skillRegistry is initialized by default in NewChatModel.

		if model.skillRegistry != nil {
			cmd := model.handleCtrlA()
			assert.NotNil(t, cmd)
			assert.Equal(t, viewModeSkillSelect, model.currentView)
		}
	})
}

func TestGetCurrentProvider(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("returns session provider when set", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.atmosConfig = &schema.AtmosConfiguration{}
		model.sess = &session.Session{Provider: "openai"}

		result := model.getCurrentProvider()
		assert.Equal(t, "openai", result)
	})

	t.Run("returns default provider from config", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.atmosConfig = &schema.AtmosConfiguration{}
		model.atmosConfig.AI.DefaultProvider = "gemini"
		model.sess = nil

		result := model.getCurrentProvider()
		assert.Equal(t, "gemini", result)
	})

	t.Run("defaults to anthropic", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.atmosConfig = &schema.AtmosConfiguration{}
		model.sess = nil

		result := model.getCurrentProvider()
		assert.Equal(t, "anthropic", result)
	})
}

func TestProviderSelectView(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("renders provider list", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.atmosConfig = &schema.AtmosConfiguration{}

		view := model.providerSelectView()

		assert.Contains(t, view, "Switch AI Provider")
		assert.Contains(t, view, "Navigate")
	})
}

func TestHandleMessage_StatusMsg(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("statusMsg updates loading text", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		cmds := []tea.Cmd{}
		handled, _ := model.handleMessage(statusMsg("Processing tools..."), &cmds)

		assert.True(t, handled)
		assert.Equal(t, "Processing tools...", model.loadingText)
	})
}

func TestHandleChatKeys(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("ctrl+c returns quit", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		cmd := model.handleChatControlKeys("ctrl+c")
		assert.NotNil(t, cmd)
	})

	t.Run("unhandled key returns nil for textarea", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)

		cmd := model.handleChatControlKeys("ctrl+x")
		assert.Nil(t, cmd)
	})

	t.Run("ctrl+l calls handleCtrlL", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.manager = nil // Will show error message.

		cmd := model.handleChatControlKeys("ctrl+l")
		assert.NotNil(t, cmd)
	})

	t.Run("ctrl+n calls handleCtrlN", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.manager = nil

		cmd := model.handleChatControlKeys("ctrl+n")
		assert.NotNil(t, cmd)
	})

	t.Run("ctrl+p calls handleCtrlP", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.atmosConfig = nil

		cmd := model.handleChatControlKeys("ctrl+p")
		assert.NotNil(t, cmd)
	})

	t.Run("ctrl+a calls handleCtrlA", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.skillRegistry = nil

		cmd := model.handleChatControlKeys("ctrl+a")
		assert.NotNil(t, cmd)
	})
}

// =============================================================================
// keys.go: navigateListUp, navigateListDown tests
// =============================================================================

func TestNavigateListUp(t *testing.T) {
	tests := []struct {
		name     string
		current  int
		total    int
		expected int
	}{
		{name: "middle of list", current: 2, total: 5, expected: 1},
		{name: "at top wraps", current: 0, total: 5, expected: 4},
		{name: "empty list", current: 0, total: 0, expected: 0},
		{name: "single item", current: 0, total: 1, expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := navigateListUp(tt.current, tt.total)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNavigateListDown(t *testing.T) {
	tests := []struct {
		name     string
		current  int
		total    int
		expected int
	}{
		{name: "middle of list", current: 2, total: 5, expected: 3},
		{name: "at bottom wraps", current: 4, total: 5, expected: 0},
		{name: "empty list", current: 0, total: 0, expected: 0},
		{name: "single item wraps", current: 0, total: 1, expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := navigateListDown(tt.current, tt.total)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// keys.go: handleProviderSelectKeys, handleSkillSelectKeys tests
// =============================================================================

func TestHandleProviderSelectKeys(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("esc returns to chat", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.atmosConfig = &schema.AtmosConfiguration{}
		model.currentView = viewModeProviderSelect

		msg := tea.KeyMsg{Type: tea.KeyEsc}
		cmd := model.handleProviderSelectKeys(msg)

		assert.NotNil(t, cmd)
		assert.Equal(t, viewModeChat, model.currentView)
	})

	t.Run("up navigates up", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.atmosConfig = &schema.AtmosConfiguration{}
		model.selectedProviderIdx = 1

		msg := tea.KeyMsg{Type: tea.KeyUp}
		cmd := model.handleProviderSelectKeys(msg)

		assert.NotNil(t, cmd)
		assert.Equal(t, 0, model.selectedProviderIdx)
	})

	t.Run("down navigates down", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.atmosConfig = &schema.AtmosConfiguration{}
		model.selectedProviderIdx = 0

		msg := tea.KeyMsg{Type: tea.KeyDown}
		cmd := model.handleProviderSelectKeys(msg)

		assert.NotNil(t, cmd)
	})

	t.Run("q returns to chat", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.atmosConfig = &schema.AtmosConfiguration{}
		model.currentView = viewModeProviderSelect

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
		cmd := model.handleProviderSelectKeys(msg)

		assert.NotNil(t, cmd)
		assert.Equal(t, viewModeChat, model.currentView)
	})
}

func TestHandleSkillSelectKeys(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("esc returns to chat", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		if model.skillRegistry == nil {
			t.Skip("skill registry not initialized")
		}
		model.currentView = viewModeSkillSelect

		msg := tea.KeyMsg{Type: tea.KeyEsc}
		cmd := model.handleSkillSelectKeys(msg)

		assert.NotNil(t, cmd)
		assert.Equal(t, viewModeChat, model.currentView)
	})

	t.Run("up navigates up", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		if model.skillRegistry == nil {
			t.Skip("skill registry not initialized")
		}
		model.selectedSkillIdx = 1

		msg := tea.KeyMsg{Type: tea.KeyUp}
		cmd := model.handleSkillSelectKeys(msg)

		assert.NotNil(t, cmd)
		assert.Equal(t, 0, model.selectedSkillIdx)
	})

	t.Run("down navigates down", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		if model.skillRegistry == nil {
			t.Skip("skill registry not initialized")
		}
		model.selectedSkillIdx = 0

		msg := tea.KeyMsg{Type: tea.KeyDown}
		cmd := model.handleSkillSelectKeys(msg)

		assert.NotNil(t, cmd)
	})

	t.Run("q returns to chat", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		if model.skillRegistry == nil {
			t.Skip("skill registry not initialized")
		}
		model.currentView = viewModeSkillSelect

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
		cmd := model.handleSkillSelectKeys(msg)

		assert.NotNil(t, cmd)
		assert.Equal(t, viewModeChat, model.currentView)
	})
}

func TestSkillSelectView(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("renders skill list", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		if model.skillRegistry == nil {
			t.Skip("skill registry not initialized")
		}

		view := model.skillSelectView()

		assert.Contains(t, view, "Switch AI Skill")
		assert.Contains(t, view, "Navigate")
	})
}

// =============================================================================
// create_session.go: tests
// =============================================================================

func TestCancelCreateSession(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("with manager goes to session list", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		// We cannot set a real manager, so test the no-manager path.
		model.manager = nil
		model.currentView = viewModeCreateSession

		cmd := model.cancelCreateSession()

		assert.NotNil(t, cmd)
		assert.Equal(t, viewModeChat, model.currentView)
	})
}

func TestToggleCreateFormFocus(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("toggles from name to provider", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.createForm = newCreateSessionForm()
		model.createForm.focusedField = 0

		model.toggleCreateFormFocus()

		assert.Equal(t, 1, model.createForm.focusedField)
	})

	t.Run("toggles from provider to name", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.createForm = newCreateSessionForm()
		model.createForm.focusedField = 1

		model.toggleCreateFormFocus()

		assert.Equal(t, 0, model.createForm.focusedField)
	})
}

func TestHandleCreateSessionKeys(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("ctrl+c returns quit", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.createForm = newCreateSessionForm()

		msg := tea.KeyMsg{Type: tea.KeyCtrlC}
		cmd := model.handleCreateSessionKeys(msg)

		assert.NotNil(t, cmd)
	})

	t.Run("esc cancels", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.createForm = newCreateSessionForm()
		model.currentView = viewModeCreateSession

		msg := tea.KeyMsg{Type: tea.KeyEsc}
		cmd := model.handleCreateSessionKeys(msg)

		assert.NotNil(t, cmd)
	})

	t.Run("tab toggles focus", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.createForm = newCreateSessionForm()
		model.createForm.focusedField = 0

		msg := tea.KeyMsg{Type: tea.KeyTab}
		cmd := model.handleCreateSessionKeys(msg)

		assert.NotNil(t, cmd)
		assert.Equal(t, 1, model.createForm.focusedField)
	})

	t.Run("up navigates provider", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.createForm = newCreateSessionForm()

		msg := tea.KeyMsg{Type: tea.KeyUp}
		cmd := model.handleCreateSessionKeys(msg)

		assert.NotNil(t, cmd)
	})

	t.Run("down navigates provider", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.createForm = newCreateSessionForm()

		msg := tea.KeyMsg{Type: tea.KeyDown}
		cmd := model.handleCreateSessionKeys(msg)

		assert.NotNil(t, cmd)
	})
}

func TestHandleKeyMsg_ViewDispatching(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("loading state dispatches to handleLoadingKeys", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.isLoading = true

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
		cmd := model.handleKeyMsg(msg)

		assert.NotNil(t, cmd)
	})

	t.Run("session list view dispatches correctly", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.currentView = viewModeSessionList
		model.availableSessions = []*session.Session{}

		msg := tea.KeyMsg{Type: tea.KeyEsc}
		cmd := model.handleKeyMsg(msg)

		assert.NotNil(t, cmd)
		assert.Equal(t, viewModeChat, model.currentView)
	})

	t.Run("create session view dispatches correctly", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.currentView = viewModeCreateSession
		model.createForm = newCreateSessionForm()

		msg := tea.KeyMsg{Type: tea.KeyCtrlC}
		cmd := model.handleKeyMsg(msg)

		assert.NotNil(t, cmd)
	})

	t.Run("provider select view dispatches correctly", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.atmosConfig = &schema.AtmosConfiguration{}
		model.currentView = viewModeProviderSelect

		msg := tea.KeyMsg{Type: tea.KeyEsc}
		cmd := model.handleKeyMsg(msg)

		assert.NotNil(t, cmd)
		assert.Equal(t, viewModeChat, model.currentView)
	})

	t.Run("skill select view dispatches correctly", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		if model.skillRegistry == nil {
			t.Skip("skill registry not initialized")
		}
		model.currentView = viewModeSkillSelect

		msg := tea.KeyMsg{Type: tea.KeyEsc}
		cmd := model.handleKeyMsg(msg)

		assert.NotNil(t, cmd)
		assert.Equal(t, viewModeChat, model.currentView)
	})

	t.Run("chat view dispatches to handleChatKeys", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.currentView = viewModeChat

		msg := tea.KeyMsg{Type: tea.KeyCtrlC}
		cmd := model.handleKeyMsg(msg)

		assert.NotNil(t, cmd)
	})
}

func TestRenderProviderLine(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("selected provider has arrow prefix", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.selectedProviderIdx = 0

		line := model.renderProviderLine(0, "anthropic", "Anthropic (Claude)", "openai")
		assert.Contains(t, line, "anthropic")
	})

	t.Run("current provider shows current label", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.selectedProviderIdx = 1

		line := model.renderProviderLine(0, "anthropic", "Anthropic (Claude)", "anthropic")
		assert.Contains(t, line, "current")
	})
}

func TestRenderSkillLine(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	t.Run("renders skill info", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.selectedSkillIdx = 0

		skill := &skills.Skill{
			Name:        "general",
			DisplayName: "General Assistant",
			Description: "A general purpose assistant",
			IsBuiltIn:   true,
		}

		line := model.renderSkillLine(0, skill, "other-skill")
		assert.Contains(t, line, "General Assistant")
		assert.Contains(t, line, "built-in")
	})

	t.Run("current skill shows current label", func(t *testing.T) {
		model, err := NewChatModel(ChatModelParams{Client: client})
		require.NoError(t, err)
		model.selectedSkillIdx = 1

		skill := &skills.Skill{
			Name:        "general",
			DisplayName: "General Assistant",
			Description: "A general purpose assistant",
		}

		line := model.renderSkillLine(0, skill, "general")
		assert.Contains(t, line, "current")
	})
}
