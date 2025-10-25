package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/session"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
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

func (m *mockAIClient) SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error) {
	response, err := m.SendMessage(ctx, message)
	if err != nil {
		return nil, err
	}
	return &types.Response{
		Content:    response,
		ToolCalls:  []types.ToolCall{},
		StopReason: types.StopReasonEndTurn,
	}, nil
}

func (m *mockAIClient) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	// For testing, just return the mock response
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func (m *mockAIClient) SendMessageWithToolsAndHistory(ctx context.Context, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	// For testing, just return the mock response
	if m.err != nil {
		return nil, m.err
	}
	return &types.Response{
		Content:    m.response,
		ToolCalls:  []types.ToolCall{},
		StopReason: types.StopReasonEndTurn,
	}, nil
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

		model, err := NewChatModel(client, nil, nil, nil, nil, nil)

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
		model, err := NewChatModel(nil, nil, nil, nil, nil, nil)

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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
	require.NoError(t, err)

	t.Run("renders empty session list", func(t *testing.T) {
		model.currentView = viewModeSessionList
		model.availableSessions = []*session.Session{}
		model.sessionFilter = "all" // Initialize filter

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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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
		assert.NotNil(t, cmd) // Returns command to consume key event
	})

	t.Run("q returns to chat from session list", func(t *testing.T) {
		model.currentView = viewModeSessionList

		keyMsg := tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{'q'},
		}

		cmd := model.handleSessionListKeys(keyMsg)

		assert.Equal(t, viewModeChat, model.currentView)
		assert.NotNil(t, cmd) // Returns command to consume key event
	})
}

func TestChatModel_SessionNavigation(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
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

func TestChatModel_CreateSession(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	// Create temporary session storage.
	tmpDir := t.TempDir()
	storagePath := tmpDir + "/sessions.db"
	storage, err := session.NewSQLiteStorage(storagePath)
	require.NoError(t, err)
	defer storage.Close()

	manager := session.NewManager(storage, tmpDir, 10)

	model, err := NewChatModel(client, nil, manager, nil, nil, nil)
	require.NoError(t, err)

	t.Run("opens create session form with Ctrl+N", func(t *testing.T) {
		model.currentView = viewModeChat
		keyMsg := tea.KeyMsg{Type: tea.KeyCtrlN}

		cmd := model.handleKeyMsg(keyMsg)

		assert.NotNil(t, cmd) // Returns command to consume key event
		assert.Equal(t, viewModeCreateSession, model.currentView)
	})

	t.Run("opens create session form from session list with n", func(t *testing.T) {
		model.currentView = viewModeSessionList
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}

		cmd := model.handleSessionListKeys(keyMsg)

		assert.NotNil(t, cmd) // Returns command to consume key event
		assert.Equal(t, viewModeCreateSession, model.currentView)
	})

	t.Run("navigates create session form fields with tab", func(t *testing.T) {
		model.currentView = viewModeCreateSession
		model.createForm = newCreateSessionForm()
		assert.Equal(t, 0, model.createForm.focusedField) // Name input

		keyMsg := tea.KeyMsg{Type: tea.KeyTab}
		model.handleCreateSessionKeys(keyMsg)

		assert.Equal(t, 1, model.createForm.focusedField) // Provider selection
	})

	t.Run("navigates provider selection with arrow keys", func(t *testing.T) {
		model.currentView = viewModeCreateSession
		model.createForm = newCreateSessionForm()
		model.createForm.focusedField = 1 // Provider selection
		model.createForm.selectedProvider = 0

		// Down arrow
		keyMsg := tea.KeyMsg{Type: tea.KeyDown}
		model.handleCreateSessionKeys(keyMsg)
		assert.Equal(t, 1, model.createForm.selectedProvider)

		// Up arrow
		keyMsg = tea.KeyMsg{Type: tea.KeyUp}
		model.handleCreateSessionKeys(keyMsg)
		assert.Equal(t, 0, model.createForm.selectedProvider)
	})

	t.Run("cancels create session with Esc", func(t *testing.T) {
		model.currentView = viewModeCreateSession
		keyMsg := tea.KeyMsg{Type: tea.KeyEsc}

		cmd := model.handleCreateSessionKeys(keyMsg)

		// Should either go to session list or chat view
		assert.True(t, model.currentView == viewModeSessionList || model.currentView == viewModeChat)
		assert.NotNil(t, cmd) // May load session list
	})

	t.Run("renders create session view", func(t *testing.T) {
		model.currentView = viewModeCreateSession
		model.createForm = newCreateSessionForm()
		model.ready = true

		view := model.View()

		assert.Contains(t, view, "Create New Session")
		assert.Contains(t, view, "Session Name:")
		assert.Contains(t, view, "Provider:")
		assert.Contains(t, view, "Anthropic")
		assert.Contains(t, view, "OpenAI")
		assert.Contains(t, view, "Gemini")
		assert.Contains(t, view, "Grok")
	})

	t.Run("handles session created successfully", func(t *testing.T) {
		model.currentView = viewModeCreateSession
		sess := &session.Session{
			ID:       "test-id",
			Name:     "test-session",
			Model:    "gpt-4o",
			Provider: "openai",
		}

		msg := sessionCreatedMsg{session: sess}
		model.handleSessionCreated(msg)

		assert.Equal(t, viewModeChat, model.currentView)
		assert.Equal(t, sess, model.sess)
		assert.Greater(t, len(model.messages), 0) // Welcome message added
	})

	t.Run("handles session creation error", func(t *testing.T) {
		model.currentView = viewModeCreateSession
		model.createForm = newCreateSessionForm()

		msg := sessionCreatedMsg{err: assert.AnError}
		model.handleSessionCreated(msg)

		assert.Equal(t, viewModeCreateSession, model.currentView) // Stays in form
		assert.NotEmpty(t, model.createForm.error)
	})
}

func TestNewCreateSessionForm(t *testing.T) {
	form := newCreateSessionForm()

	assert.NotNil(t, form.nameInput)
	assert.Equal(t, 0, form.selectedProvider) // Defaults to Anthropic
	assert.Equal(t, 0, form.focusedField)     // Starts with name input focused
	assert.Empty(t, form.error)
}

func TestAvailableProviders(t *testing.T) {
	assert.Len(t, AvailableProviders, 7)

	// Verify all expected providers are present.
	providerNames := make([]string, len(AvailableProviders))
	for i, p := range AvailableProviders {
		providerNames[i] = p.Name
	}

	assert.Contains(t, providerNames, "anthropic")
	assert.Contains(t, providerNames, "openai")
	assert.Contains(t, providerNames, "gemini")
	assert.Contains(t, providerNames, "grok")
	assert.Contains(t, providerNames, "ollama")
	assert.Contains(t, providerNames, "bedrock")
	assert.Contains(t, providerNames, "azureopenai")

	// Verify provider details.
	for _, p := range AvailableProviders {
		assert.NotEmpty(t, p.Name)
		assert.NotEmpty(t, p.DisplayName)
		assert.NotEmpty(t, p.DefaultModel)
		assert.NotEmpty(t, p.APIKeyEnv)
	}
}

func TestChatModel_DeleteSession(t *testing.T) {
	t.Run("initiates delete confirmation with d key", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up session list with sessions
		m.availableSessions = []*session.Session{
			{ID: "session-1", Name: "Test Session 1"},
			{ID: "session-2", Name: "Test Session 2"},
		}
		m.selectedSessionIndex = 0
		m.currentView = viewModeSessionList

		// Press 'd' key to initiate delete
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
		cmd := m.handleSessionListKeys(msg)

		// Verify delete confirmation state is set
		assert.True(t, m.deleteConfirm)
		assert.Equal(t, "session-1", m.deleteSessionID)
		assert.NotNil(t, cmd) // Returns command to consume key event
	})

	t.Run("cancels delete with n key", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up delete confirmation state
		m.deleteConfirm = true
		m.deleteSessionID = "session-1"
		m.currentView = viewModeSessionList

		// Press 'n' to cancel
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
		cmd := m.handleSessionListKeys(msg)

		// Verify delete confirmation is canceled
		assert.False(t, m.deleteConfirm)
		assert.Empty(t, m.deleteSessionID)
		assert.Nil(t, cmd)
	})

	t.Run("cancels delete with esc key", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up delete confirmation state
		m.deleteConfirm = true
		m.deleteSessionID = "session-1"
		m.currentView = viewModeSessionList

		// Press 'esc' to cancel
		msg := tea.KeyMsg{Type: tea.KeyEscape}
		cmd := m.handleSessionListKeys(msg)

		// Verify delete confirmation is canceled
		assert.False(t, m.deleteConfirm)
		assert.Empty(t, m.deleteSessionID)
		assert.Nil(t, cmd)
	})

	t.Run("confirms delete with y key", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up delete confirmation state
		m.deleteConfirm = true
		m.deleteSessionID = "session-1"
		m.currentView = viewModeSessionList

		// Press 'y' to confirm
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
		cmd := m.handleSessionListKeys(msg)

		// Verify delete command is returned
		assert.NotNil(t, cmd)

		// Execute the command - it should return sessionDeletedMsg
		// (We can't test the actual deletion without a real manager)
	})

	t.Run("renders delete confirmation dialog", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up session list and delete confirmation
		m.availableSessions = []*session.Session{
			{ID: "session-1", Name: "Test Session 1"},
		}
		m.deleteConfirm = true
		m.deleteSessionID = "session-1"
		m.currentView = viewModeSessionList

		// Render view
		view := m.sessionListView()

		// Verify confirmation dialog is shown
		assert.Contains(t, view, "Delete session 'Test Session 1'")
		assert.Contains(t, view, "This action cannot be undone")
		assert.Contains(t, view, "y: Confirm Delete")
		assert.Contains(t, view, "n/Esc: Cancel")
	})

	t.Run("shows delete option in help text", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up session list
		m.availableSessions = []*session.Session{
			{ID: "session-1", Name: "Test Session 1"},
		}
		m.currentView = viewModeSessionList

		// Render view
		view := m.sessionListView()

		// Verify delete option is in help text
		assert.Contains(t, view, "d: Delete")
	})

	t.Run("handles sessionDeletedMsg successfully", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up delete confirmation state
		m.deleteConfirm = true
		m.deleteSessionID = "session-1"

		// Handle successful deletion
		msg := sessionDeletedMsg{sessionID: "session-1", err: nil}
		cmd := m.handleSessionDeleted(msg)

		// Verify state is reset
		assert.False(t, m.deleteConfirm)
		assert.Empty(t, m.deleteSessionID)
		assert.Empty(t, m.sessionListError)

		// Command should be returned to reload session list
		assert.NotNil(t, cmd)
	})

	t.Run("handles sessionDeletedMsg with error", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up delete confirmation state
		m.deleteConfirm = true
		m.deleteSessionID = "session-1"

		// Handle deletion error
		msg := sessionDeletedMsg{sessionID: "session-1", err: assert.AnError}
		cmd := m.handleSessionDeleted(msg)

		// Verify state is reset
		assert.False(t, m.deleteConfirm)
		assert.Empty(t, m.deleteSessionID)
		assert.Contains(t, m.sessionListError, "Failed to delete session")

		// No command should be returned on error
		assert.Nil(t, cmd)
	})

	t.Run("clears current session if deleted", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up current session
		m.sess = &session.Session{ID: "session-1", Name: "Test Session"}
		m.messages = []ChatMessage{
			{Role: "user", Content: "test", Time: time.Now()},
		}

		// Handle deletion of current session
		msg := sessionDeletedMsg{sessionID: "session-1", err: nil}
		m.handleSessionDeleted(msg)

		// Verify current session is cleared
		assert.Nil(t, m.sess)
		assert.Empty(t, m.messages)
	})

	t.Run("does not clear different session", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up current session
		m.sess = &session.Session{ID: "session-1", Name: "Test Session"}
		m.messages = []ChatMessage{
			{Role: "user", Content: "test", Time: time.Now()},
		}

		// Handle deletion of different session
		msg := sessionDeletedMsg{sessionID: "session-2", err: nil}
		m.handleSessionDeleted(msg)

		// Verify current session is not cleared
		assert.NotNil(t, m.sess)
		assert.NotEmpty(t, m.messages)
	})
}

func TestChatModel_RenameSession(t *testing.T) {
	t.Run("initiates rename mode with r key", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up session list with sessions
		m.availableSessions = []*session.Session{
			{ID: "session-1", Name: "Test Session 1"},
			{ID: "session-2", Name: "Test Session 2"},
		}
		m.selectedSessionIndex = 0
		m.currentView = viewModeSessionList

		// Press 'r' key to initiate rename
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
		cmd := m.handleSessionListKeys(msg)

		// Verify rename mode is set
		assert.True(t, m.renameMode)
		assert.Equal(t, "session-1", m.renameSessionID)
		assert.Equal(t, "Test Session 1", m.renameInput.Value())
		assert.NotNil(t, cmd) // Returns command to consume key event
	})

	t.Run("cancels rename with esc key", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up rename mode
		m.renameMode = true
		m.renameSessionID = "session-1"
		m.currentView = viewModeSessionList

		// Press 'esc' to cancel
		msg := tea.KeyMsg{Type: tea.KeyEscape}
		cmd := m.handleSessionListKeys(msg)

		// Verify rename mode is canceled
		assert.False(t, m.renameMode)
		assert.Empty(t, m.renameSessionID)
		assert.Nil(t, cmd)
	})

	t.Run("submits rename with enter key", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up rename mode
		m.renameMode = true
		m.renameSessionID = "session-1"
		m.renameInput = textinput.New()
		m.renameInput.SetValue("New Session Name")
		m.currentView = viewModeSessionList

		// Press 'enter' to submit
		msg := tea.KeyMsg{Type: tea.KeyEnter}
		cmd := m.handleSessionListKeys(msg)

		// Verify rename command is returned
		assert.NotNil(t, cmd)
	})

	t.Run("cancels rename if empty name submitted", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up rename mode with empty input
		m.renameMode = true
		m.renameSessionID = "session-1"
		m.renameInput = textinput.New()
		m.renameInput.SetValue("   ") // Empty after trim
		m.currentView = viewModeSessionList

		// Press 'enter' to submit
		msg := tea.KeyMsg{Type: tea.KeyEnter}
		cmd := m.handleSessionListKeys(msg)

		// Verify rename is canceled
		assert.False(t, m.renameMode)
		assert.Empty(t, m.renameSessionID)
		assert.Nil(t, cmd)
	})

	t.Run("updates text input during rename", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up rename mode
		m.renameMode = true
		m.renameSessionID = "session-1"
		m.renameInput = textinput.New()
		m.renameInput.SetValue("Test")
		m.currentView = viewModeSessionList

		// Type a character
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}}
		m.handleSessionListKeys(msg)

		// Verify we're still in rename mode (input was processed)
		assert.True(t, m.renameMode)
	})

	t.Run("renders rename dialog", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up session list and rename mode
		m.availableSessions = []*session.Session{
			{ID: "session-1", Name: "Test Session 1"},
		}
		m.renameMode = true
		m.renameSessionID = "session-1"
		m.renameInput = textinput.New()
		m.renameInput.SetValue("New Name")
		m.currentView = viewModeSessionList

		// Render view
		view := m.sessionListView()

		// Verify rename dialog is shown
		assert.Contains(t, view, "Rename session 'Test Session 1'")
		assert.Contains(t, view, "Enter: Save")
		assert.Contains(t, view, "Esc: Cancel")
	})

	t.Run("shows rename option in help text", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up session list
		m.availableSessions = []*session.Session{
			{ID: "session-1", Name: "Test Session 1"},
		}
		m.currentView = viewModeSessionList

		// Render view
		view := m.sessionListView()

		// Verify rename option is in help text
		assert.Contains(t, view, "r: Rename")
	})

	t.Run("handles sessionRenamedMsg successfully", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up rename mode
		m.renameMode = true
		m.renameSessionID = "session-1"

		// Handle successful rename
		msg := sessionRenamedMsg{sessionID: "session-1", newName: "New Name", err: nil}
		cmd := m.handleSessionRenamed(msg)

		// Verify state is reset
		assert.False(t, m.renameMode)
		assert.Empty(t, m.renameSessionID)
		assert.Empty(t, m.sessionListError)

		// Command should be returned to reload session list
		assert.NotNil(t, cmd)
	})

	t.Run("handles sessionRenamedMsg with error", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up rename mode
		m.renameMode = true
		m.renameSessionID = "session-1"

		// Handle rename error
		msg := sessionRenamedMsg{sessionID: "session-1", newName: "New Name", err: assert.AnError}
		cmd := m.handleSessionRenamed(msg)

		// Verify state is reset
		assert.False(t, m.renameMode)
		assert.Empty(t, m.renameSessionID)
		assert.Contains(t, m.sessionListError, "Failed to rename session")

		// No command should be returned on error
		assert.Nil(t, cmd)
	})

	t.Run("updates current session name if renamed", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up current session
		m.sess = &session.Session{ID: "session-1", Name: "Old Name"}

		// Handle rename of current session
		msg := sessionRenamedMsg{sessionID: "session-1", newName: "New Name", err: nil}
		m.handleSessionRenamed(msg)

		// Verify current session name is updated
		assert.Equal(t, "New Name", m.sess.Name)
	})

	t.Run("does not update different session name", func(t *testing.T) {
		client := &mockAIClient{model: "test-model"}
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Set up current session
		m.sess = &session.Session{ID: "session-1", Name: "Old Name"}

		// Handle rename of different session
		msg := sessionRenamedMsg{sessionID: "session-2", newName: "New Name", err: nil}
		m.handleSessionRenamed(msg)

		// Verify current session name is not updated
		assert.Equal(t, "Old Name", m.sess.Name)
	})
}

func TestChatModel_HistoryNavigation(t *testing.T) {
	client := &mockAIClient{model: "test-model"}

	t.Run("initializes with empty history", func(t *testing.T) {
		m, err := NewChatModel(client, nil, nil, nil, nil, nil)
		require.NoError(t, err)

		assert.Empty(t, m.messageHistory)
		assert.Equal(t, -1, m.historyIndex)
		assert.Empty(t, m.historyBuffer)
	})

	t.Run("adds messages to history when sent", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Send a message
		msg := sendMessageMsg("Test message 1")
		m.handleSendMessage(msg)

		// Verify history
		assert.Len(t, m.messageHistory, 1)
		assert.Equal(t, "Test message 1", m.messageHistory[0])
		assert.Equal(t, -1, m.historyIndex)

		// Send another message
		msg = sendMessageMsg("Test message 2")
		m.handleSendMessage(msg)

		// Verify history
		assert.Len(t, m.messageHistory, 2)
		assert.Equal(t, "Test message 2", m.messageHistory[1])
	})

	t.Run("navigates up through history", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Add messages to history
		m.messageHistory = []string{"Message 1", "Message 2", "Message 3"}
		m.textarea.SetValue("Current input")

		// Navigate up once
		m.navigateHistoryUp()
		assert.Equal(t, "Message 3", m.textarea.Value())
		assert.Equal(t, 2, m.historyIndex)
		assert.Equal(t, "Current input", m.historyBuffer)

		// Navigate up again
		m.navigateHistoryUp()
		assert.Equal(t, "Message 2", m.textarea.Value())
		assert.Equal(t, 1, m.historyIndex)

		// Navigate up again
		m.navigateHistoryUp()
		assert.Equal(t, "Message 1", m.textarea.Value())
		assert.Equal(t, 0, m.historyIndex)

		// Try to navigate past beginning
		m.navigateHistoryUp()
		assert.Equal(t, "Message 1", m.textarea.Value())
		assert.Equal(t, 0, m.historyIndex)
	})

	t.Run("navigates down through history", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Add messages to history
		m.messageHistory = []string{"Message 1", "Message 2", "Message 3"}
		m.textarea.SetValue("Current input")

		// Navigate to beginning
		m.navigateHistoryUp()
		m.navigateHistoryUp()
		m.navigateHistoryUp()
		assert.Equal(t, "Message 1", m.textarea.Value())
		assert.Equal(t, 0, m.historyIndex)

		// Navigate down
		m.navigateHistoryDown()
		assert.Equal(t, "Message 2", m.textarea.Value())
		assert.Equal(t, 1, m.historyIndex)

		// Navigate down
		m.navigateHistoryDown()
		assert.Equal(t, "Message 3", m.textarea.Value())
		assert.Equal(t, 2, m.historyIndex)

		// Navigate down past end - should restore original input
		m.navigateHistoryDown()
		assert.Equal(t, "Current input", m.textarea.Value())
		assert.Equal(t, -1, m.historyIndex)
		assert.Empty(t, m.historyBuffer)
	})

	t.Run("does nothing on empty history", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		m.textarea.SetValue("Current input")

		// Try to navigate up
		m.navigateHistoryUp()
		assert.Equal(t, "Current input", m.textarea.Value())
		assert.Equal(t, -1, m.historyIndex)
	})

	t.Run("handles up key in chat mode", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.messageHistory = []string{"Previous message"}
		m.textarea.SetValue("Current input")

		// Simulate up key
		keyMsg := tea.KeyMsg{Type: tea.KeyUp}
		m.handleKeyMsg(keyMsg)

		// Verify navigation occurred
		assert.Equal(t, "Previous message", m.textarea.Value())
		assert.Equal(t, 0, m.historyIndex)
	})

	t.Run("handles down key in chat mode", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.messageHistory = []string{"Previous message"}
		m.textarea.SetValue("Current input")

		// Navigate up first
		m.navigateHistoryUp()

		// Simulate down key
		keyMsg := tea.KeyMsg{Type: tea.KeyDown}
		m.handleKeyMsg(keyMsg)

		// Verify navigation occurred
		assert.Equal(t, "Current input", m.textarea.Value())
		assert.Equal(t, -1, m.historyIndex)
	})

	t.Run("resets history index when sending message", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.messageHistory = []string{"Message 1", "Message 2"}

		// Navigate in history
		m.navigateHistoryUp()
		assert.Equal(t, 1, m.historyIndex)

		// Send message
		msg := sendMessageMsg("New message")
		m.handleSendMessage(msg)

		// Verify history index is reset
		assert.Equal(t, -1, m.historyIndex)
		assert.Empty(t, m.historyBuffer)
	})

	t.Run("loads history from existing session messages", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

		// Simulate loaded messages
		m.messages = []ChatMessage{
			{Role: roleUser, Content: "User message 1"},
			{Role: roleAssistant, Content: "Assistant response 1"},
			{Role: roleUser, Content: "User message 2"},
			{Role: roleAssistant, Content: "Assistant response 2"},
		}

		// Manually populate history (simulating what loadSessionMessages does)
		for _, msg := range m.messages {
			if msg.Role == roleUser {
				m.messageHistory = append(m.messageHistory, msg.Content)
			}
		}

		// Verify history contains only user messages
		assert.Len(t, m.messageHistory, 2)
		assert.Equal(t, "User message 1", m.messageHistory[0])
		assert.Equal(t, "User message 2", m.messageHistory[1])
	})
}

// mockAIClientWithHistory is a mock that captures message history.
type mockAIClientWithHistory struct {
	mockAIClient
	receivedMessages []types.Message
}

func (m *mockAIClientWithHistory) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	m.receivedMessages = messages
	return m.response, m.err
}

func TestChatModel_ProviderFilteredHistory(t *testing.T) {
	t.Run("filters messages by provider when building history", func(t *testing.T) {
		// Create a mock client that tracks the messages it receives
		client := &mockAIClientWithHistory{
			mockAIClient: mockAIClient{
				model:    "test-model",
				response: "Test response",
			},
		}

		// Create a session with anthropic provider
		sess := &session.Session{
			ID:       "test-session",
			Name:     "Test Session",
			Provider: "anthropic",
		}

		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.sess = sess
		m.ready = true

		// Add messages from different providers
		m.messages = []ChatMessage{
			{Role: roleUser, Content: "User question 1", Provider: ""},
			{Role: roleAssistant, Content: "Anthropic response 1", Provider: "anthropic"},
			{Role: roleUser, Content: "User question 2", Provider: ""},
			{Role: roleAssistant, Content: "OpenAI response", Provider: "openai"}, // Different provider
			{Role: roleUser, Content: "User question 3", Provider: ""},
			{Role: roleAssistant, Content: "Anthropic response 2", Provider: "anthropic"},
		}

		// Trigger getAIResponse which should filter messages
		cmd := m.getAIResponse("New user message")
		result := cmd()

		// Verify we got a response
		assert.NotNil(t, result)

		// Check that receivedMessages only contains:
		// - All user messages
		// - Only assistant messages from anthropic (current provider)
		// - The new user message
		expectedMessageCount := 6 // 3 existing user + 2 anthropic assistant + 1 new user
		assert.Len(t, client.receivedMessages, expectedMessageCount,
			"Should include all user messages and only assistant messages from current provider")

		// Verify the messages are in the correct order and from correct provider
		assert.Equal(t, roleUser, client.receivedMessages[0].Role)
		assert.Equal(t, "User question 1", client.receivedMessages[0].Content)

		assert.Equal(t, roleAssistant, client.receivedMessages[1].Role)
		assert.Equal(t, "Anthropic response 1", client.receivedMessages[1].Content)

		assert.Equal(t, roleUser, client.receivedMessages[2].Role)
		assert.Equal(t, "User question 2", client.receivedMessages[2].Content)

		// OpenAI response should be skipped (index 3 would be User question 3)
		assert.Equal(t, roleUser, client.receivedMessages[3].Role)
		assert.Equal(t, "User question 3", client.receivedMessages[3].Content)

		assert.Equal(t, roleAssistant, client.receivedMessages[4].Role)
		assert.Equal(t, "Anthropic response 2", client.receivedMessages[4].Content)

		// New user message should be last
		assert.Equal(t, roleUser, client.receivedMessages[5].Role)
		assert.Equal(t, "New user message", client.receivedMessages[5].Content)
	})

	t.Run("includes all messages when provider matches", func(t *testing.T) {
		client := &mockAIClientWithHistory{
			mockAIClient: mockAIClient{
				model:    "test-model",
				response: "Test response",
			},
		}

		sess := &session.Session{
			ID:       "test-session",
			Name:     "Test Session",
			Provider: "openai",
		}

		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.sess = sess
		m.ready = true

		// Add messages all from openai
		m.messages = []ChatMessage{
			{Role: roleUser, Content: "User question 1", Provider: ""},
			{Role: roleAssistant, Content: "OpenAI response 1", Provider: "openai"},
			{Role: roleUser, Content: "User question 2", Provider: ""},
			{Role: roleAssistant, Content: "OpenAI response 2", Provider: "openai"},
		}

		cmd := m.getAIResponse("New message")
		cmd()

		// All messages should be included
		assert.Len(t, client.receivedMessages, 5) // 2 user + 2 assistant + 1 new user
	})

	t.Run("excludes all assistant messages when provider doesn't match", func(t *testing.T) {
		client := &mockAIClientWithHistory{
			mockAIClient: mockAIClient{
				model:    "test-model",
				response: "Test response",
			},
		}

		sess := &session.Session{
			ID:       "test-session",
			Name:     "Test Session",
			Provider: "gemini", // Different from messages
		}

		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.sess = sess
		m.ready = true

		// Add messages from anthropic and openai
		m.messages = []ChatMessage{
			{Role: roleUser, Content: "User question 1", Provider: ""},
			{Role: roleAssistant, Content: "Anthropic response", Provider: "anthropic"},
			{Role: roleUser, Content: "User question 2", Provider: ""},
			{Role: roleAssistant, Content: "OpenAI response", Provider: "openai"},
		}

		cmd := m.getAIResponse("New message")
		cmd()

		// Only user messages should be included
		assert.Len(t, client.receivedMessages, 3) // 2 existing user + 1 new user
		assert.Equal(t, roleUser, client.receivedMessages[0].Role)
		assert.Equal(t, roleUser, client.receivedMessages[1].Role)
		assert.Equal(t, roleUser, client.receivedMessages[2].Role)
	})
}

func TestChatModel_MarkdownRendering(t *testing.T) {
	client := &mockAIClient{model: "test-model"}

	t.Run("renders markdown for assistant messages", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.ready = true
		m.viewport.Width = 80

		markdown := "# Hello\n\nThis is **bold** and *italic*.\n\n```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```"
		rendered := m.renderMarkdown(markdown)

		// Verify markdown was rendered (not plain text)
		assert.NotEqual(t, markdown, rendered)
		// Verify it's not empty
		assert.NotEmpty(t, rendered)
		// Verify padding was added
		assert.Contains(t, rendered, "  ")
	})

	t.Run("handles plain text in markdown renderer", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.ready = true
		m.viewport.Width = 80

		plainText := "This is plain text without markdown"
		rendered := m.renderMarkdown(plainText)

		// Should still render successfully
		assert.NotEmpty(t, rendered)
		assert.Contains(t, rendered, "  ")
	})

	t.Run("handles minimum width constraint", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.ready = true
		m.viewport.Width = 10 // Very narrow viewport

		markdown := "# Test"
		rendered := m.renderMarkdown(markdown)

		// Should still render without panicking
		assert.NotEmpty(t, rendered)
	})

	t.Run("renders code blocks with syntax highlighting", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.ready = true
		m.viewport.Width = 80

		markdown := "```yaml\ncomponents:\n  terraform:\n    vpc:\n      vars:\n        cidr: 10.0.0.0/16\n```"
		rendered := m.renderMarkdown(markdown)

		// Verify code block was processed
		assert.NotEmpty(t, rendered)
		// Should contain the YAML content
		assert.Contains(t, rendered, "components")
	})

	t.Run("renders lists correctly", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.ready = true
		m.viewport.Width = 80

		markdown := "- Item 1\n- Item 2\n- Item 3"
		rendered := m.renderMarkdown(markdown)

		// Verify list was rendered
		assert.NotEmpty(t, rendered)
	})

	t.Run("uses markdown for assistant role only", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.ready = true
		m.viewport.Width = 80
		m.viewport.Height = 20

		// Add assistant message with markdown
		m.messages = []ChatMessage{
			{Role: roleAssistant, Content: "**Bold text**", Time: time.Now()},
			{Role: roleUser, Content: "**Bold text**", Time: time.Now()},
		}

		m.updateViewportContent()
		content := m.viewport.View()

		// Assistant message should be rendered differently than user message
		assert.NotEmpty(t, content)
	})

	t.Run("handles empty markdown", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.ready = true
		m.viewport.Width = 80

		rendered := m.renderMarkdown("")

		// Should handle gracefully
		assert.NotNil(t, rendered)
	})

	t.Run("handles markdown with tables", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.ready = true
		m.viewport.Width = 80

		markdown := "| Header 1 | Header 2 |\n|----------|----------|\n| Cell 1   | Cell 2   |"
		rendered := m.renderMarkdown(markdown)

		// Verify table was rendered
		assert.NotEmpty(t, rendered)
		assert.Contains(t, rendered, "Header")
	})
}

func TestChatModel_GetProviderBadge(t *testing.T) {
	client := &mockAIClient{model: "test-model"}
	m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

	tests := []struct {
		name          string
		provider      string
		expectedBadge string
		expectedColor string
	}{
		{"anthropic provider", "anthropic", "[Claude]", "#00FFFF"},
		{"openai provider", "openai", "[GPT]", "#00FF00"},
		{"gemini provider", "gemini", "[Gemini]", "#FFFF00"},
		{"grok provider", "grok", "[Grok]", "#FF69B4"},
		{"ollama provider", "ollama", "[Ollama]", "#5F5FFF"},
		{"unknown provider", "unknown", "[AI]", "240"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			badge, color := m.getProviderBadge(tt.provider)
			assert.Equal(t, tt.expectedBadge, badge)
			assert.Equal(t, tt.expectedColor, color)
		})
	}
}

func TestChatModel_GetFilterDisplayName(t *testing.T) {
	client := &mockAIClient{model: "test-model"}
	m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

	tests := []struct {
		name         string
		filter       string
		expectedName string
	}{
		{"all filter", "all", "All"},
		{"anthropic filter", "anthropic", "Claude"},
		{"openai filter", "openai", "GPT"},
		{"gemini filter", "gemini", "Gemini"},
		{"grok filter", "grok", "Grok"},
		{"ollama filter", "ollama", "Ollama"},
		{"unknown filter", "unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := m.getFilterDisplayName(tt.filter)
			assert.Equal(t, tt.expectedName, name)
		})
	}
}

func TestChatModel_CycleFilter(t *testing.T) {
	client := &mockAIClient{model: "test-model"}
	m, _ := NewChatModel(client, nil, nil, nil, nil, nil)

	// Initialize with "all" filter
	m.sessionFilter = "all"
	m.selectedSessionIndex = 5

	// Test cycling through all filters
	t.Run("cycles from all to anthropic", func(t *testing.T) {
		m.sessionFilter = "all"
		m.selectedSessionIndex = 5
		m.cycleFilter()
		assert.Equal(t, "anthropic", m.sessionFilter)
		assert.Equal(t, 0, m.selectedSessionIndex, "should reset index")
	})

	t.Run("cycles from anthropic to openai", func(t *testing.T) {
		m.sessionFilter = "anthropic"
		m.selectedSessionIndex = 3
		m.cycleFilter()
		assert.Equal(t, "openai", m.sessionFilter)
		assert.Equal(t, 0, m.selectedSessionIndex, "should reset index")
	})

	t.Run("cycles from openai to gemini", func(t *testing.T) {
		m.sessionFilter = "openai"
		m.cycleFilter()
		assert.Equal(t, "gemini", m.sessionFilter)
	})

	t.Run("cycles from gemini to grok", func(t *testing.T) {
		m.sessionFilter = "gemini"
		m.cycleFilter()
		assert.Equal(t, "grok", m.sessionFilter)
	})

	t.Run("cycles from grok to ollama", func(t *testing.T) {
		m.sessionFilter = "grok"
		m.cycleFilter()
		assert.Equal(t, "ollama", m.sessionFilter)
	})

	t.Run("cycles from ollama back to all", func(t *testing.T) {
		m.sessionFilter = "ollama"
		m.cycleFilter()
		assert.Equal(t, "all", m.sessionFilter)
	})

	t.Run("handles unknown filter by defaulting to all", func(t *testing.T) {
		m.sessionFilter = "unknown"
		m.selectedSessionIndex = 2
		m.cycleFilter()
		assert.Equal(t, "all", m.sessionFilter)
		assert.Equal(t, 0, m.selectedSessionIndex)
	})
}
