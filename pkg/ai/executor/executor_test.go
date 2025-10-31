package executor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockClient is a mock AI client for testing.
type mockClient struct {
	responses []*types.Response
	callIndex int
}

func (m *mockClient) SendMessage(ctx context.Context, message string) (string, error) {
	return "Mock response", nil
}

func (m *mockClient) SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error) {
	if m.callIndex >= len(m.responses) {
		return &types.Response{
			Content:    "Final response",
			StopReason: types.StopReasonEndTurn,
		}, nil
	}

	response := m.responses[m.callIndex]
	m.callIndex++
	return response, nil
}

func (m *mockClient) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	return "Mock response with history", nil
}

func (m *mockClient) SendMessageWithToolsAndHistory(ctx context.Context, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	if m.callIndex >= len(m.responses) {
		return &types.Response{
			Content:    "Final response",
			StopReason: types.StopReasonEndTurn,
		}, nil
	}

	response := m.responses[m.callIndex]
	m.callIndex++
	return response, nil
}

func (m *mockClient) GetModel() string {
	return "mock-model"
}

func (m *mockClient) GetMaxTokens() int {
	return 4096
}

// Note: Tool executor tests are complex and require full integration testing.
// Basic executor functionality is tested below with simple (non-tool) execution.

func TestExecutor_ExecuteSimple(t *testing.T) {
	client := &mockClient{}
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultProvider: "mock",
			},
		},
	}

	exec := NewExecutor(client, nil, atmosConfig)

	result := exec.Execute(context.Background(), Options{
		Prompt:       "What is Atmos?",
		ToolsEnabled: false,
	})

	assert.True(t, result.Success)
	assert.Equal(t, "Mock response", result.Response)
	assert.Equal(t, "mock-model", result.Metadata.Model)
	assert.Equal(t, "mock", result.Metadata.Provider)
	assert.False(t, result.Metadata.ToolsEnabled)
}

// TODO: Add integration tests for tool execution.
// Tool execution tests require a real tools.Executor which is complex to mock.
// These should be tested via integration tests in tests/ directory.
