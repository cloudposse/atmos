package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
)

// executableTool is a mock tool that returns configurable results.
type executableTool struct {
	name         string
	result       *Result
	err          error
	requiresPerm bool
	restricted   bool
}

func (m *executableTool) Name() string            { return m.name }
func (m *executableTool) Description() string     { return "executable mock" }
func (m *executableTool) Parameters() []Parameter { return nil }
func (m *executableTool) Execute(_ context.Context, _ map[string]interface{}) (*Result, error) {
	return m.result, m.err
}
func (m *executableTool) RequiresPermission() bool { return m.requiresPerm }
func (m *executableTool) IsRestricted() bool       { return m.restricted }

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

func TestNewExecutor_DefaultTimeout(t *testing.T) {
	registry := NewRegistry()
	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeAllow}, nil)

	// Zero timeout should use default.
	exec := NewExecutor(registry, permChecker, 0)
	assert.NotNil(t, exec)
}

func TestExecute_Success(t *testing.T) {
	registry := NewRegistry()
	tool := &executableTool{
		name:   "test_tool",
		result: &Result{Success: true, Output: "done"},
	}
	require.NoError(t, registry.Register(tool))

	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeAllow}, nil)
	exec := NewExecutor(registry, permChecker, DefaultTimeout)

	result, err := exec.Execute(context.Background(), "test_tool", nil)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "done", result.Output)
}

func TestExecute_ToolNotFound(t *testing.T) {
	registry := NewRegistry()
	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeAllow}, nil)
	exec := NewExecutor(registry, permChecker, DefaultTimeout)

	result, err := exec.Execute(context.Background(), "nonexistent", nil)
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestExecute_ToolReturnsError(t *testing.T) {
	registry := NewRegistry()
	tool := &executableTool{
		name: "failing_tool",
		err:  errors.New("tool failed"),
	}
	require.NoError(t, registry.Register(tool))

	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeAllow}, nil)
	exec := NewExecutor(registry, permChecker, DefaultTimeout)

	result, err := exec.Execute(context.Background(), "failing_tool", nil)
	assert.Error(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestExecute_PermissionDenied(t *testing.T) {
	registry := NewRegistry()
	tool := &executableTool{
		name:         "restricted_tool",
		requiresPerm: true,
		result:       &Result{Success: true},
	}
	require.NoError(t, registry.Register(tool))

	// Deny mode returns an error for tools that require permission.
	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeDeny}, nil)
	exec := NewExecutor(registry, permChecker, DefaultTimeout)

	_, err := exec.Execute(context.Background(), "restricted_tool", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "permission check failed")
}

func TestExecute_NoPermissionRequired(t *testing.T) {
	registry := NewRegistry()
	tool := &executableTool{
		name:         "open_tool",
		requiresPerm: false,
		result:       &Result{Success: true, Output: "ok"},
	}
	require.NoError(t, registry.Register(tool))

	// Even deny mode allows tools that don't require permission.
	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeDeny}, nil)
	exec := NewExecutor(registry, permChecker, DefaultTimeout)

	result, err := exec.Execute(context.Background(), "open_tool", nil)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestExecuteBatch_Success(t *testing.T) {
	registry := NewRegistry()
	tool1 := &executableTool{name: "tool_a", result: &Result{Success: true, Output: "a"}}
	tool2 := &executableTool{name: "tool_b", result: &Result{Success: true, Output: "b"}}
	require.NoError(t, registry.Register(tool1))
	require.NoError(t, registry.Register(tool2))

	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeAllow}, nil)
	exec := NewExecutor(registry, permChecker, DefaultTimeout)

	calls := []ToolCall{
		{Tool: "tool_a", Params: nil},
		{Tool: "tool_b", Params: nil},
	}
	results, err := exec.ExecuteBatch(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.True(t, results[0].Success)
	assert.True(t, results[1].Success)
	assert.Equal(t, "a", results[0].Output)
	assert.Equal(t, "b", results[1].Output)
}

func TestExecuteBatch_PartialFailure(t *testing.T) {
	registry := NewRegistry()
	tool1 := &executableTool{name: "good", result: &Result{Success: true, Output: "ok"}}
	tool2 := &executableTool{name: "bad", err: errors.New("failed")}
	tool3 := &executableTool{name: "also_good", result: &Result{Success: true, Output: "ok2"}}
	require.NoError(t, registry.Register(tool1))
	require.NoError(t, registry.Register(tool2))
	require.NoError(t, registry.Register(tool3))

	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeAllow}, nil)
	exec := NewExecutor(registry, permChecker, DefaultTimeout)

	calls := []ToolCall{
		{Tool: "good"},
		{Tool: "bad"},
		{Tool: "also_good"},
	}
	results, err := exec.ExecuteBatch(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.True(t, results[0].Success)
	assert.False(t, results[1].Success)
	assert.True(t, results[2].Success)
}

func TestExecuteBatch_ToolNotFound(t *testing.T) {
	registry := NewRegistry()
	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeAllow}, nil)
	exec := NewExecutor(registry, permChecker, DefaultTimeout)

	calls := []ToolCall{{Tool: "nonexistent"}}
	results, err := exec.ExecuteBatch(context.Background(), calls)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].Success)
}

func TestListTools(t *testing.T) {
	registry := NewRegistry()
	tool1 := &executableTool{name: "tool_x", result: &Result{}}
	tool2 := &executableTool{name: "tool_y", result: &Result{}}
	require.NoError(t, registry.Register(tool1))
	require.NoError(t, registry.Register(tool2))

	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeAllow}, nil)
	exec := NewExecutor(registry, permChecker, DefaultTimeout)

	tools := exec.ListTools()
	assert.Len(t, tools, 2)
}

func TestListTools_NilRegistry(t *testing.T) {
	exec := &Executor{registry: nil}
	tools := exec.ListTools()
	assert.Nil(t, tools)
}

func TestListTools_Empty(t *testing.T) {
	registry := NewRegistry()
	permChecker := permission.NewChecker(&permission.Config{Mode: permission.ModeAllow}, nil)
	exec := NewExecutor(registry, permChecker, DefaultTimeout)

	tools := exec.ListTools()
	assert.Empty(t, tools)
}
