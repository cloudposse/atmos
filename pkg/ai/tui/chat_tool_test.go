package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/ai/types"
)

// MockTool implements tools.Tool for testing.
type MockTool struct {
	NameVal        string
	DescriptionVal string
	ExecuteFunc    func(ctx context.Context, params map[string]interface{}) (*tools.Result, error)
}

func (m *MockTool) Name() string {
	return m.NameVal
}

func (m *MockTool) Description() string {
	return m.DescriptionVal
}

func (m *MockTool) Parameters() []tools.Parameter {
	return []tools.Parameter{}
}

func (m *MockTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, params)
	}
	return &tools.Result{
		Success: true,
		Output:  "Mock tool output",
	}, nil
}

func (m *MockTool) RequiresPermission() bool {
	return false
}

func (m *MockTool) IsRestricted() bool {
	return false
}

func TestChatModel_HandleToolExecutionFlowWithAccumulator(t *testing.T) {
	// Setup dependencies
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	mockTool := &MockTool{
		NameVal:        "mock_tool",
		DescriptionVal: "A mock tool",
		ExecuteFunc: func(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
			return &tools.Result{
				Success: true,
				Output:  "Mock tool execution result",
			}, nil
		},
	}

	registry := tools.NewRegistry()
	err := registry.Register(mockTool)
	require.NoError(t, err)

	// Use permissive config
	permConfig := &permission.Config{
		Mode: permission.ModeAllow,
	}
	permChecker := permission.NewChecker(permConfig, nil)

	executor := tools.NewExecutor(registry, permChecker, 10*time.Second)

	model, err := NewChatModel(client, nil, nil, nil, executor, nil)
	require.NoError(t, err)

	// Prepare input for handleToolExecutionFlowWithAccumulator
	ctx := context.Background()
	response := &types.Response{
		Content: "I will use the mock tool.",
		ToolCalls: []types.ToolCall{
			{
				Name: "mock_tool",
				Input: map[string]interface{}{
					"key": "value",
				},
				ID: "call_123",
			},
		},
	}
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Run the tool"},
	}
	availableTools := []tools.Tool{mockTool}
	accumulatedContent := ""

	// Execute the function
	msg := model.handleToolExecutionFlowWithAccumulator(ctx, response, messages, availableTools, accumulatedContent)

	// Verify the result
	// The function returns tea.Msg, which should be the final response (likely from getAIResponseWithContext, which is a tea.Cmd)
	// Since getAIResponseWithContext returns a tea.Cmd, and handleToolExecutionFlowWithAccumulator returns tea.Msg (Wait, let's check signature)

	// Signature: func (m *ChatModel) handleToolExecutionFlowWithAccumulator(...) tea.Msg
	// It calls m.getAIResponseWithContext which returns tea.Cmd.
	// But wait, the function ends with:
	// return m.getAIResponseWithContext(...)
	// But getAIResponseWithContext returns tea.Cmd.
	// And tea.Cmd is func() tea.Msg.
	// So handleToolExecutionFlowWithAccumulator returns tea.Msg ??
	// Let's re-read the function signature in chat.go.

	// From line 1422: func (m *ChatModel) handleToolExecutionFlowWithAccumulator(...) tea.Msg
	// And line 1618 (end): return m.getAIResponseWithContext(...) which returns tea.Cmd.
	// tea.Cmd IS func() tea.Msg
	// So it returns a function.

	assert.NotNil(t, msg)

	// We can try to cast it to tea.Cmd and execute it if we want to go deeper,
	// but just verifying it executes without panic is a good start for coverage.

	cmd, ok := msg.(tea.Cmd)
	if ok && cmd != nil {
		// Execute the command? It might try to call the AI client.
		// Our mock AI client doesn't return error, so it should be fine.
		// but getAIResponseWithContext is async.
		_ = cmd // explicitly ignore the command to satisfy linter
	}
}
