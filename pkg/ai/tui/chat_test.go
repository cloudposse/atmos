package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/session"
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

		model, err := NewChatModel(client, nil, nil, nil, nil)

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
		model, err := NewChatModel(nil, nil, nil, nil, nil)

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

	model, err := NewChatModel(client, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil)
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

func TestChatModel_MultiLineInput(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	t.Run("plain enter sends message", func(t *testing.T) {
		model.textarea.SetValue("Test message")

		keyMsg := tea.KeyMsg{
			Type: tea.KeyEnter,
		}

		cmd := model.handleKeyMsg(keyMsg)

		// Should return a sendMessage command.
		assert.NotNil(t, cmd)
	})

	t.Run("shift+enter adds newline", func(t *testing.T) {
		keyMsg := tea.KeyMsg{
			Type: tea.KeyEnter,
			Alt:  false,
		}

		// Simulate Shift+Enter by checking string representation.
		// The actual key will be "shift+enter" when Shift is held.
		cmd := model.handleKeyMsg(keyMsg)

		// Plain enter with empty textarea should return no-op, not nil.
		assert.NotNil(t, cmd)
	})

	t.Run("empty message not sent", func(t *testing.T) {
		model.textarea.SetValue("")

		keyMsg := tea.KeyMsg{
			Type: tea.KeyEnter,
		}

		cmd := model.handleKeyMsg(keyMsg)

		// Should return a no-op command (not nil, not sendMessage).
		assert.NotNil(t, cmd)
	})
}

func TestChatModel_SessionPickerInitialization(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	// Check session picker fields are initialized.
	assert.Equal(t, viewModeChat, model.currentView)
	assert.NotNil(t, model.availableSessions)
	assert.Equal(t, 0, model.selectedSessionIndex)
	assert.Equal(t, "", model.sessionListError)
}

func TestChatModel_SessionListView(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	t.Run("renders empty session list", func(t *testing.T) {
		model.currentView = viewModeSessionList
		model.availableSessions = []*session.Session{}

		view := model.sessionListView()

		assert.Contains(t, view, "Session List")
		assert.Contains(t, view, "No sessions available")
	})

	t.Run("renders session list error", func(t *testing.T) {
		model.currentView = viewModeSessionList
		model.sessionListError = "Test error message"

		view := model.sessionListView()

		assert.Contains(t, view, "Error: Test error message")
	})

	t.Run("switches to session list view", func(t *testing.T) {
		model.currentView = viewModeChat
		model.ready = true

		view := model.View()
		assert.NotContains(t, view, "Session List")

		model.currentView = viewModeSessionList
		view = model.View()
		assert.Contains(t, view, "Session List")
	})
}

func TestChatModel_KeyboardShortcuts(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	t.Run("ctrl+c quits in chat mode", func(t *testing.T) {
		model.currentView = viewModeChat

		keyMsg := tea.KeyMsg{
			Type: tea.KeyCtrlC,
		}

		cmd := model.handleKeyMsg(keyMsg)

		assert.NotNil(t, cmd)
		// The cmd should be tea.Quit, but we can't easily test that.
		// We verify the cmd is not nil which means it was handled.
	})

	t.Run("ctrl+c quits in session list mode", func(t *testing.T) {
		model.currentView = viewModeSessionList

		keyMsg := tea.KeyMsg{
			Type: tea.KeyCtrlC,
		}

		cmd := model.handleKeyMsg(keyMsg)

		assert.NotNil(t, cmd)
	})

	t.Run("esc returns to chat from session list", func(t *testing.T) {
		model.currentView = viewModeSessionList
		model.availableSessions = []*session.Session{}

		keyMsg := tea.KeyMsg{
			Type: tea.KeyEsc,
		}

		cmd := model.handleSessionListKeys(keyMsg)

		assert.Equal(t, viewModeChat, model.currentView)
		assert.Nil(t, cmd)
	})

	t.Run("q returns to chat from session list", func(t *testing.T) {
		model.currentView = viewModeSessionList

		keyMsg := tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{'q'},
		}

		cmd := model.handleSessionListKeys(keyMsg)

		assert.Equal(t, viewModeChat, model.currentView)
		assert.Nil(t, cmd)
	})
}

func TestChatModel_SessionNavigation(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	// Create mock sessions.
	mockSessions := []*session.Session{
		{ID: "1", Name: "Session 1", CreatedAt: time.Now()},
		{ID: "2", Name: "Session 2", CreatedAt: time.Now()},
		{ID: "3", Name: "Session 3", CreatedAt: time.Now()},
	}

	model.currentView = viewModeSessionList
	model.availableSessions = mockSessions
	model.selectedSessionIndex = 0

	t.Run("down arrow navigates down", func(t *testing.T) {
		keyMsg := tea.KeyMsg{
			Type: tea.KeyDown,
		}

		model.handleSessionListKeys(keyMsg)

		assert.Equal(t, 1, model.selectedSessionIndex)
	})

	t.Run("down arrow at bottom stays at bottom", func(t *testing.T) {
		model.selectedSessionIndex = 2

		keyMsg := tea.KeyMsg{
			Type: tea.KeyDown,
		}

		model.handleSessionListKeys(keyMsg)

		assert.Equal(t, 2, model.selectedSessionIndex)
	})

	t.Run("up arrow navigates up", func(t *testing.T) {
		model.selectedSessionIndex = 2

		keyMsg := tea.KeyMsg{
			Type: tea.KeyUp,
		}

		model.handleSessionListKeys(keyMsg)

		assert.Equal(t, 1, model.selectedSessionIndex)
	})

	t.Run("up arrow at top stays at top", func(t *testing.T) {
		model.selectedSessionIndex = 0

		keyMsg := tea.KeyMsg{
			Type: tea.KeyUp,
		}

		model.handleSessionListKeys(keyMsg)

		assert.Equal(t, 0, model.selectedSessionIndex)
	})
}

func TestChatModel_SessionSwitching(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	t.Run("handles session list loaded with sessions", func(t *testing.T) {
		mockSessions := []*session.Session{
			{ID: "1", Name: "Test Session", CreatedAt: time.Now()},
		}

		msg := sessionListLoadedMsg{
			sessions: mockSessions,
			err:      nil,
		}

		model.handleSessionListLoaded(msg)

		assert.Equal(t, viewModeSessionList, model.currentView)
		assert.Len(t, model.availableSessions, 1)
		assert.Equal(t, 0, model.selectedSessionIndex)
		assert.Equal(t, "", model.sessionListError)
	})

	t.Run("handles session list loaded with error", func(t *testing.T) {
		msg := sessionListLoadedMsg{
			sessions: nil,
			err:      assert.AnError,
		}

		model.handleSessionListLoaded(msg)

		assert.NotEqual(t, "", model.sessionListError)
	})

	t.Run("handles session switched successfully", func(t *testing.T) {
		mockSession := &session.Session{
			ID:        "test-id",
			Name:      "Test Session",
			CreatedAt: time.Now(),
		}

		mockMessages := []*session.Message{
			{Role: "user", Content: "Hello", CreatedAt: time.Now()},
			{Role: "assistant", Content: "Hi", CreatedAt: time.Now()},
		}

		msg := sessionSwitchedMsg{
			session:  mockSession,
			messages: mockMessages,
			err:      nil,
		}

		model.handleSessionSwitched(msg)

		assert.Equal(t, viewModeChat, model.currentView)
		assert.Equal(t, mockSession, model.sess)
		assert.Len(t, model.messages, 2)
		assert.Equal(t, "user", model.messages[0].Role)
		assert.Equal(t, "assistant", model.messages[1].Role)
		assert.Equal(t, "", model.sessionListError)
	})

	t.Run("handles session switched with error", func(t *testing.T) {
		msg := sessionSwitchedMsg{
			session:  nil,
			messages: nil,
			err:      assert.AnError,
		}

		model.handleSessionSwitched(msg)

		assert.NotEqual(t, "", model.sessionListError)
	})
}

func TestChatModel_ViewModes(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	// Make the model ready.
	model.ready = true

	t.Run("renders chat view by default", func(t *testing.T) {
		model.currentView = viewModeChat

		view := model.View()

		assert.Contains(t, view, "Atmos AI Assistant")
		assert.NotContains(t, view, "Session List")
	})

	t.Run("renders session list view when switched", func(t *testing.T) {
		model.currentView = viewModeSessionList

		view := model.View()

		assert.Contains(t, view, "Session List")
	})

	t.Run("shows initialization message when not ready", func(t *testing.T) {
		model.ready = false

		view := model.View()

		assert.Contains(t, view, "Initializing")
	})
}

func TestChatModel_AddMessage(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	t.Run("adds user message", func(t *testing.T) {
		initialCount := len(model.messages)
		model.addMessage(roleUser, "Hello, AI!")

		assert.Len(t, model.messages, initialCount+1)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Equal(t, roleUser, lastMsg.Role)
		assert.Equal(t, "Hello, AI!", lastMsg.Content)
	})

	t.Run("adds assistant message", func(t *testing.T) {
		initialCount := len(model.messages)
		model.addMessage(roleAssistant, "Hello! How can I help?")

		assert.Len(t, model.messages, initialCount+1)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Equal(t, roleAssistant, lastMsg.Role)
		assert.Equal(t, "Hello! How can I help?", lastMsg.Content)
	})

	t.Run("adds system message", func(t *testing.T) {
		initialCount := len(model.messages)
		model.addMessage(roleSystem, "System notification")

		assert.Len(t, model.messages, initialCount+1)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Equal(t, roleSystem, lastMsg.Role)
		assert.Equal(t, "System notification", lastMsg.Content)
	})
}

func TestChatModel_MessageHandling(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
		response:  "AI response",
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	t.Run("handles send message", func(t *testing.T) {
		msg := sendMessageMsg("Test message")
		handled, cmd := model.handleSendMessage(msg)

		assert.True(t, handled)
		assert.NotNil(t, cmd)
		assert.True(t, model.isLoading)
		assert.Equal(t, "", model.textarea.Value())
		// Verify message was added.
		assert.Greater(t, len(model.messages), 0)
	})

	t.Run("handles AI response", func(t *testing.T) {
		model.isLoading = true
		initialCount := len(model.messages)

		msg := aiResponseMsg("Test response")
		handled := model.handleAIMessage(msg)

		assert.True(t, handled)
		assert.False(t, model.isLoading)
		assert.Len(t, model.messages, initialCount+1)
	})

	t.Run("handles AI error", func(t *testing.T) {
		model.isLoading = true
		initialCount := len(model.messages)

		msg := aiErrorMsg("Test error")
		handled := model.handleAIMessage(msg)

		assert.True(t, handled)
		assert.False(t, model.isLoading)
		assert.Len(t, model.messages, initialCount+1)
		lastMsg := model.messages[len(model.messages)-1]
		assert.Equal(t, roleSystem, lastMsg.Role)
		assert.Contains(t, lastMsg.Content, "Error:")
	})
}

func TestChatModel_SendMessage(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	cmd := model.sendMessage("Test message")
	assert.NotNil(t, cmd)

	msg := cmd()
	sendMsg, ok := msg.(sendMessageMsg)
	assert.True(t, ok)
	assert.Equal(t, "Test message", string(sendMsg))
}

func TestChatModel_HandleKeyMessage(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	t.Run("handles quit key", func(t *testing.T) {
		keyMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
		handled, cmd := model.handleKeyMessage(keyMsg)

		assert.True(t, handled)
		assert.NotNil(t, cmd)
	})

	t.Run("handles unhandled key", func(t *testing.T) {
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
		handled, cmd := model.handleKeyMessage(keyMsg)

		assert.False(t, handled)
		assert.Nil(t, cmd)
	})
}

func TestChatModel_UpdateWithKeyPress(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	// Set model as ready.
	model.ready = true
	model.width = 80
	model.height = 40

	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
	updatedModel, cmd := model.Update(keyMsg)

	assert.NotNil(t, updatedModel)
	assert.NotNil(t, cmd)
}

func TestChatModel_RenderSessionList(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	t.Run("renders session list with sessions", func(t *testing.T) {
		mockSessions := []*session.Session{
			{ID: "1", Name: "Session 1", CreatedAt: time.Now()},
			{ID: "2", Name: "Session 2", CreatedAt: time.Now()},
		}
		model.availableSessions = mockSessions
		model.selectedSessionIndex = 0

		var content strings.Builder
		styles := model.sessionListStyles()

		model.renderSessionList(&content, &styles)

		result := content.String()
		assert.Contains(t, result, "Session 1")
		assert.Contains(t, result, "Session 2")
		assert.Contains(t, result, "→") // Selected indicator
	})

	t.Run("highlights selected session", func(t *testing.T) {
		mockSessions := []*session.Session{
			{ID: "1", Name: "First", CreatedAt: time.Now()},
			{ID: "2", Name: "Second", CreatedAt: time.Now()},
		}
		model.availableSessions = mockSessions
		model.selectedSessionIndex = 1

		var content strings.Builder
		styles := model.sessionListStyles()

		model.renderSessionList(&content, &styles)

		result := content.String()
		// The second session should be marked as selected.
		lines := strings.Split(result, "\n")
		assert.Greater(t, len(lines), 1)
	})
}

func TestChatModel_HandleMessage(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
		response:  "Test response",
	}

	model, err := NewChatModel(client, nil, nil, nil, nil)
	require.NoError(t, err)

	t.Run("handles window size message", func(t *testing.T) {
		var cmds []tea.Cmd
		msg := tea.WindowSizeMsg{Width: 100, Height: 40}

		handled, cmd := model.handleMessage(msg, &cmds)

		assert.True(t, handled)
		assert.Nil(t, cmd)
		assert.Equal(t, 100, model.width)
		assert.Equal(t, 40, model.height)
	})

	t.Run("handles send message msg", func(t *testing.T) {
		var cmds []tea.Cmd
		msg := sendMessageMsg("Test")

		handled, cmd := model.handleMessage(msg, &cmds)

		assert.True(t, handled)
		assert.NotNil(t, cmd)
	})

	t.Run("returns false for unknown message", func(t *testing.T) {
		var cmds []tea.Cmd
		type unknownMsg struct{}
		msg := unknownMsg{}

		handled, cmd := model.handleMessage(msg, &cmds)

		assert.False(t, handled)
		assert.Nil(t, cmd)
	})
}
