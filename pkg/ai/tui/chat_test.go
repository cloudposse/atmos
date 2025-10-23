package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAIClient is a mock implementation of ai.Client for testing.
type mockAIClient struct {
	model     string
	maxTokens int
	response  string
	err       error
}

func (m *mockAIClient) SendMessage(ctx context.Context, message string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func (m *mockAIClient) GetModel() string {
	return m.model
}

func (m *mockAIClient) GetMaxTokens() int {
	return m.maxTokens
}

func TestNewChatModel(t *testing.T) {
	t.Run("successful initialization", func(t *testing.T) {
		client := &mockAIClient{
			model:     "test-model",
			maxTokens: 4096,
		}

		model, err := NewChatModel(client, nil, nil, nil)

		require.NoError(t, err)
		assert.NotNil(t, model)
		assert.Equal(t, client, model.client)
		assert.NotNil(t, model.messages)
		assert.Len(t, model.messages, 0)
		assert.NotNil(t, model.viewport)
		assert.NotNil(t, model.textarea)
		assert.NotNil(t, model.spinner)
		assert.False(t, model.isLoading)
		assert.False(t, model.ready)
	})

	t.Run("nil client returns error", func(t *testing.T) {
		model, err := NewChatModel(nil, nil, nil, nil)

		assert.Error(t, err)
		assert.Nil(t, model)
		assert.Contains(t, err.Error(), "AI client cannot be nil")
	})
}

func TestChatModel_Init(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil)
	require.NoError(t, err)

	cmd := model.Init()

	// Init should return a batch command with textarea blink and spinner tick.
	assert.NotNil(t, cmd)
}

func TestChatMessage(t *testing.T) {
	t.Run("message creation", func(t *testing.T) {
		now := time.Now()
		msg := ChatMessage{
			Role:    "user",
			Content: "Hello, AI!",
			Time:    now,
		}

		assert.Equal(t, "user", msg.Role)
		assert.Equal(t, "Hello, AI!", msg.Content)
		assert.Equal(t, now, msg.Time)
	})

	t.Run("assistant message", func(t *testing.T) {
		msg := ChatMessage{
			Role:    "assistant",
			Content: "Hello! How can I help?",
			Time:    time.Now(),
		}

		assert.Equal(t, "assistant", msg.Role)
		assert.Contains(t, msg.Content, "help")
	})
}

func TestChatModel_WindowResize(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil)
	require.NoError(t, err)

	// Simulate window resize.
	resizeMsg := tea.WindowSizeMsg{
		Width:  100,
		Height: 40,
	}

	updatedModel, _ := model.Update(resizeMsg)

	chatModel, ok := updatedModel.(*ChatModel)
	assert.True(t, ok)
	assert.Equal(t, 100, chatModel.width)
	assert.Equal(t, 40, chatModel.height)
	assert.True(t, chatModel.ready)
}

func TestChatModel_MessageStorage(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil)
	require.NoError(t, err)

	// Add a message to the model.
	msg := ChatMessage{
		Role:    "user",
		Content: "Test message",
		Time:    time.Now(),
	}

	model.messages = append(model.messages, msg)

	assert.Len(t, model.messages, 1)
	assert.Equal(t, "user", model.messages[0].Role)
	assert.Equal(t, "Test message", model.messages[0].Content)
}

func TestChatModel_MultipleMessages(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil)
	require.NoError(t, err)

	// Add multiple messages.
	messages := []ChatMessage{
		{Role: "user", Content: "First message", Time: time.Now()},
		{Role: "assistant", Content: "First response", Time: time.Now()},
		{Role: "user", Content: "Second message", Time: time.Now()},
		{Role: "assistant", Content: "Second response", Time: time.Now()},
	}

	model.messages = append(model.messages, messages...)

	assert.Len(t, model.messages, 4)
	assert.Equal(t, "user", model.messages[0].Role)
	assert.Equal(t, "assistant", model.messages[1].Role)
	assert.Equal(t, "user", model.messages[2].Role)
	assert.Equal(t, "assistant", model.messages[3].Role)
}

func TestChatModel_LoadingState(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil)
	require.NoError(t, err)

	// Initially not loading.
	assert.False(t, model.isLoading)

	// Simulate loading state.
	model.isLoading = true
	assert.True(t, model.isLoading)

	// Complete loading.
	model.isLoading = false
	assert.False(t, model.isLoading)
}
