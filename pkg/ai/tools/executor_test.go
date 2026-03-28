package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
)

func TestCleanToolName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "triple underscore becomes dot",
			input:    "aws___search_documentation",
			expected: "aws.search_documentation",
		},
		{
			name:     "double underscore becomes dot",
			input:    "aws__list_clusters",
			expected: "aws.list_clusters",
		},
		{
			name:     "single underscore preserved",
			input:    "search_documentation",
			expected: "search_documentation",
		},
		{
			name:     "no underscores",
			input:    "search",
			expected: "search",
		},
		{
			name:     "multiple namespace separators",
			input:    "aws___ec2___describe_instances",
			expected: "aws.ec2.describe_instances",
		},
		{
			name:     "trailing double underscore",
			input:    "tool__",
			expected: "tool.",
		},
		{
			name:     "trailing single underscore",
			input:    "tool_",
			expected: "tool_",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only underscores",
			input:    "___",
			expected: ".",
		},
		{
			name:     "single underscore only",
			input:    "_",
			expected: "_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, cleanToolName(tt.input))
		})
	}
}

// mockBridgedTool implements both Tool and BridgedToolInfo for testing.
type mockBridgedTool struct {
	name         string
	serverName   string
	originalName string
}

func (m *mockBridgedTool) Name() string            { return m.name }
func (m *mockBridgedTool) Description() string     { return "mock" }
func (m *mockBridgedTool) Parameters() []Parameter { return nil }
func (m *mockBridgedTool) Execute(_ context.Context, _ map[string]interface{}) (*Result, error) {
	return nil, nil
}
func (m *mockBridgedTool) RequiresPermission() bool { return false }
func (m *mockBridgedTool) IsRestricted() bool       { return false }
func (m *mockBridgedTool) ServerName() string       { return m.serverName }
func (m *mockBridgedTool) OriginalName() string     { return m.originalName }

// Verify mockBridgedTool implements both interfaces at compile time.
var (
	_ Tool            = (*mockBridgedTool)(nil)
	_ BridgedToolInfo = (*mockBridgedTool)(nil)
)

// mockSimpleTool implements only Tool (no BridgedToolInfo).
type mockSimpleTool struct {
	name string
}

func (m *mockSimpleTool) Name() string            { return m.name }
func (m *mockSimpleTool) Description() string     { return "simple" }
func (m *mockSimpleTool) Parameters() []Parameter { return nil }
func (m *mockSimpleTool) Execute(_ context.Context, _ map[string]interface{}) (*Result, error) {
	return nil, nil
}
func (m *mockSimpleTool) RequiresPermission() bool { return false }
func (m *mockSimpleTool) IsRestricted() bool       { return false }

func TestDisplayName_BridgedTool(t *testing.T) {
	registry := NewRegistry()
	bt := &mockBridgedTool{
		name:         "aws-knowledge__aws___search_documentation",
		serverName:   "aws-knowledge",
		originalName: "aws___search_documentation",
	}
	require.NoError(t, registry.Register(bt))

	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeAllow}, nil)
	exec := NewExecutor(registry, permChecker, DefaultTimeout)

	result := exec.DisplayName("aws-knowledge__aws___search_documentation")
	assert.Equal(t, "aws-knowledge → aws.search_documentation", result)
}

func TestDisplayName_SimpleTool(t *testing.T) {
	registry := NewRegistry()
	st := &mockSimpleTool{name: "atmos_list_stacks"}
	require.NoError(t, registry.Register(st))

	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeAllow}, nil)
	exec := NewExecutor(registry, permChecker, DefaultTimeout)

	result := exec.DisplayName("atmos_list_stacks")
	assert.Equal(t, "atmos_list_stacks", result)
}

func TestDisplayName_NotFound(t *testing.T) {
	registry := NewRegistry()
	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeAllow}, nil)
	exec := NewExecutor(registry, permChecker, DefaultTimeout)

	result := exec.DisplayName("nonexistent_tool")
	assert.Equal(t, "nonexistent_tool", result)
}
