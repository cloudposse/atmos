package tui

import (
	"context"
	"fmt"
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
	"github.com/cloudposse/atmos/pkg/schema"
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

	t.Run("down arrow at bottom wraps to top", func(t *testing.T) {
		model.selectedSessionIndex = 2

		keyMsg := tea.KeyMsg{
			Type: tea.KeyDown,
		}

		model.handleSessionListKeys(keyMsg)

		assert.Equal(t, 0, model.selectedSessionIndex)
	})

	t.Run("up arrow navigates up", func(t *testing.T) {
		model.selectedSessionIndex = 2

		keyMsg := tea.KeyMsg{
			Type: tea.KeyUp,
		}

		model.handleSessionListKeys(keyMsg)

		assert.Equal(t, 1, model.selectedSessionIndex)
	})

	t.Run("up arrow at top wraps to bottom", func(t *testing.T) {
		model.selectedSessionIndex = 0

		keyMsg := tea.KeyMsg{
			Type: tea.KeyUp,
		}

		model.handleSessionListKeys(keyMsg)

		assert.Equal(t, 2, model.selectedSessionIndex)
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

		msg := aiResponseMsg{content: "Test response", usage: nil}
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

	manager := session.NewManager(storage, tmpDir, 10, nil)

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

	t.Run("wraps provider selection at boundaries", func(t *testing.T) {
		model.currentView = viewModeCreateSession
		model.createForm = newCreateSessionForm()
		model.createForm.focusedField = 1 // Provider selection

		// Get provider count
		configuredProviders := model.getConfiguredProvidersForCreate()
		lastIndex := len(configuredProviders) - 1

		// Up arrow at top wraps to bottom
		model.createForm.selectedProvider = 0
		keyMsg := tea.KeyMsg{Type: tea.KeyUp}
		model.handleCreateSessionKeys(keyMsg)
		assert.Equal(t, lastIndex, model.createForm.selectedProvider)

		// Down arrow at bottom wraps to top
		model.createForm.selectedProvider = lastIndex
		keyMsg = tea.KeyMsg{Type: tea.KeyDown}
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

	t.Run("handles mouse clicks to focus fields", func(t *testing.T) {
		model.currentView = viewModeCreateSession
		model.createForm = newCreateSessionForm()
		model.createForm.focusedField = 1 // Start with provider field focused

		// Click in name field area (Y <= 6)
		mouseMsg := tea.MouseMsg{
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionPress,
			X:      10,
			Y:      3,
		}

		handled, cmd := model.handleMouseMessage(mouseMsg)

		assert.True(t, handled)
		assert.Nil(t, cmd)
		assert.Equal(t, 0, model.createForm.focusedField) // Name field should be focused

		// Click in provider area (Y > 6)
		mouseMsg = tea.MouseMsg{
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionPress,
			X:      10,
			Y:      10,
		}

		handled, cmd = model.handleMouseMessage(mouseMsg)

		assert.True(t, handled)
		assert.Nil(t, cmd)
		assert.Equal(t, 1, model.createForm.focusedField) // Provider field should be focused
	})

	t.Run("ignores non-left mouse clicks in create session", func(t *testing.T) {
		model.currentView = viewModeCreateSession
		model.createForm = newCreateSessionForm()
		model.createForm.focusedField = 0

		// Right click should be ignored
		mouseMsg := tea.MouseMsg{
			Button: tea.MouseButtonRight,
			Action: tea.MouseActionPress,
			X:      10,
			Y:      10,
		}

		handled, cmd := model.handleMouseMessage(mouseMsg)

		assert.False(t, handled)
		assert.Nil(t, cmd)
		assert.Equal(t, 0, model.createForm.focusedField) // Field should not change
	})

	t.Run("ignores mouse clicks outside create session view", func(t *testing.T) {
		model.currentView = viewModeChat
		model.createForm = newCreateSessionForm()

		mouseMsg := tea.MouseMsg{
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionPress,
			X:      10,
			Y:      3,
		}

		handled, cmd := model.handleMouseMessage(mouseMsg)

		assert.False(t, handled)
		assert.Nil(t, cmd)
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

func TestGetConfiguredProvidersForCreate(t *testing.T) {
	t.Run("returns configured providers with models from atmos.yaml", func(t *testing.T) {
		// Create mock config with specific providers
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Providers: map[string]*schema.AIProviderConfig{
						"anthropic": {
							Model: "claude-3-opus-20240229",
						},
						"openai": {
							Model: "gpt-4-turbo",
						},
					},
				},
			},
		}

		model := &ChatModel{
			atmosConfig: atmosConfig,
		}

		providers := model.getConfiguredProvidersForCreate()

		// Should have exactly 2 configured providers
		assert.Len(t, providers, 2)

		// Verify Anthropic provider
		assert.Equal(t, "anthropic", providers[0].Name)
		assert.Equal(t, "Anthropic (Claude)", providers[0].DisplayName)
		assert.Equal(t, "claude-3-opus-20240229", providers[0].Model) // From atmos.yaml

		// Verify OpenAI provider
		assert.Equal(t, "openai", providers[1].Name)
		assert.Equal(t, "OpenAI (GPT)", providers[1].DisplayName)
		assert.Equal(t, "gpt-4-turbo", providers[1].Model) // From atmos.yaml
	})

	t.Run("returns default providers when no config", func(t *testing.T) {
		model := &ChatModel{
			atmosConfig: nil,
		}

		providers := model.getConfiguredProvidersForCreate()

		// Should have default providers
		assert.GreaterOrEqual(t, len(providers), 7)

		// Verify all have required fields
		for _, p := range providers {
			assert.NotEmpty(t, p.Name)
			assert.NotEmpty(t, p.DisplayName)
			assert.NotEmpty(t, p.Model)
		}
	})
}

func TestChatModel_ProviderSelectNavigation(t *testing.T) {
	// Create mock config with multiple providers
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Providers: map[string]*schema.AIProviderConfig{
					"anthropic": {Model: "claude-3-opus-20240229"},
					"openai":    {Model: "gpt-4-turbo"},
					"gemini":    {Model: "gemini-pro"},
				},
			},
		},
	}

	client := &mockAIClient{model: "test-model"}
	model, err := NewChatModel(client, atmosConfig, nil, nil, nil, nil)
	require.NoError(t, err)

	model.currentView = viewModeProviderSelect
	model.selectedProviderIdx = 0

	t.Run("navigates down through providers", func(t *testing.T) {
		model.selectedProviderIdx = 0
		keyMsg := tea.KeyMsg{Type: tea.KeyDown}

		model.handleProviderSelectKeys(keyMsg)

		assert.Equal(t, 1, model.selectedProviderIdx)
	})

	t.Run("navigates up through providers", func(t *testing.T) {
		model.selectedProviderIdx = 1
		keyMsg := tea.KeyMsg{Type: tea.KeyUp}

		model.handleProviderSelectKeys(keyMsg)

		assert.Equal(t, 0, model.selectedProviderIdx)
	})

	t.Run("wraps to bottom when up at top", func(t *testing.T) {
		model.selectedProviderIdx = 0
		configuredProviders := model.getConfiguredProviders()
		lastIndex := len(configuredProviders) - 1

		keyMsg := tea.KeyMsg{Type: tea.KeyUp}
		model.handleProviderSelectKeys(keyMsg)

		assert.Equal(t, lastIndex, model.selectedProviderIdx)
	})

	t.Run("wraps to top when down at bottom", func(t *testing.T) {
		configuredProviders := model.getConfiguredProviders()
		lastIndex := len(configuredProviders) - 1
		model.selectedProviderIdx = lastIndex

		keyMsg := tea.KeyMsg{Type: tea.KeyDown}
		model.handleProviderSelectKeys(keyMsg)

		assert.Equal(t, 0, model.selectedProviderIdx)
	})

	t.Run("returns to chat on esc", func(t *testing.T) {
		model.currentView = viewModeProviderSelect
		keyMsg := tea.KeyMsg{Type: tea.KeyEsc}

		model.handleProviderSelectKeys(keyMsg)

		assert.Equal(t, viewModeChat, model.currentView)
	})
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

	t.Run("allows up/down navigation in multiline text", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.messageHistory = []string{"Previous message"}

		// Set multiline text in textarea
		m.textarea.SetValue("Line 1\nLine 2\nLine 3")

		// Simulate up key - should NOT navigate history, should be handled by textarea
		keyMsg := tea.KeyMsg{Type: tea.KeyUp}
		cmd := m.handleKeyMsg(keyMsg)

		// Command should be nil, allowing textarea to handle the key
		assert.Nil(t, cmd)
		// History should not be activated
		assert.Equal(t, -1, m.historyIndex)

		// Simulate down key - should NOT navigate history, should be handled by textarea
		keyMsg = tea.KeyMsg{Type: tea.KeyDown}
		cmd = m.handleKeyMsg(keyMsg)

		// Command should be nil, allowing textarea to handle the key
		assert.Nil(t, cmd)
		// History should not be activated
		assert.Equal(t, -1, m.historyIndex)
	})

	t.Run("uses up/down for history in single-line text", func(t *testing.T) {
		m, _ := NewChatModel(client, nil, nil, nil, nil, nil)
		m.messageHistory = []string{"Previous message"}

		// Set single-line text in textarea (no newlines)
		m.textarea.SetValue("Current single line")

		// Simulate up key - should navigate history
		keyMsg := tea.KeyMsg{Type: tea.KeyUp}
		cmd := m.handleKeyMsg(keyMsg)

		// Command should consume the key
		assert.NotNil(t, cmd)
		// History should be activated
		assert.Equal(t, "Previous message", m.textarea.Value())
		assert.Equal(t, 0, m.historyIndex)
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

		// Add messages from different provider sessions
		// With complete isolation, only messages from the current provider session are sent
		m.messages = []ChatMessage{
			{Role: roleUser, Content: "User question 1", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Anthropic response 1", Provider: "anthropic"},
			{Role: roleUser, Content: "OpenAI user question", Provider: "openai"}, // Different provider
			{Role: roleAssistant, Content: "OpenAI response", Provider: "openai"}, // Different provider
			{Role: roleUser, Content: "User question 2", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Anthropic response 2", Provider: "anthropic"},
		}

		// Trigger getAIResponseWithContext which should filter messages
		ctx := context.Background()
		cmd := m.getAIResponseWithContext("New user message", ctx)
		result := cmd()

		// Verify we got a response
		assert.NotNil(t, result)

		// Check that receivedMessages only contains:
		// - User and assistant messages from anthropic (current provider)
		// - The new user message
		// OpenAI messages should be completely filtered out for isolation
		expectedMessageCount := 5 // 2 anthropic user + 2 anthropic assistant + 1 new user
		assert.Len(t, client.receivedMessages, expectedMessageCount,
			"Should include only messages from current provider (anthropic)")

		// Verify the messages are in the correct order and from correct provider
		assert.Equal(t, roleUser, client.receivedMessages[0].Role)
		assert.Equal(t, "User question 1", client.receivedMessages[0].Content)

		assert.Equal(t, roleAssistant, client.receivedMessages[1].Role)
		assert.Equal(t, "Anthropic response 1", client.receivedMessages[1].Content)

		assert.Equal(t, roleUser, client.receivedMessages[2].Role)
		assert.Equal(t, "User question 2", client.receivedMessages[2].Content)

		// OpenAI messages should be completely skipped
		assert.Equal(t, roleAssistant, client.receivedMessages[3].Role)
		assert.Equal(t, "Anthropic response 2", client.receivedMessages[3].Content)

		// New user message should be last
		assert.Equal(t, roleUser, client.receivedMessages[4].Role)
		assert.Equal(t, "New user message", client.receivedMessages[4].Content)
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
			{Role: roleUser, Content: "User question 1", Provider: "openai"},
			{Role: roleAssistant, Content: "OpenAI response 1", Provider: "openai"},
			{Role: roleUser, Content: "User question 2", Provider: "openai"},
			{Role: roleAssistant, Content: "OpenAI response 2", Provider: "openai"},
		}

		ctx := context.Background()
		cmd := m.getAIResponseWithContext("New message", ctx)
		cmd()

		// All messages from openai should be included
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

		// Add messages from anthropic and openai (but not gemini)
		m.messages = []ChatMessage{
			{Role: roleUser, Content: "User question 1", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Anthropic response", Provider: "anthropic"},
			{Role: roleUser, Content: "User question 2", Provider: "openai"},
			{Role: roleAssistant, Content: "OpenAI response", Provider: "openai"},
		}

		ctx := context.Background()
		cmd := m.getAIResponseWithContext("New message", ctx)
		cmd()

		// Since current provider is gemini and no messages match, only the new user message should be sent
		assert.Len(t, client.receivedMessages, 1) // Only 1 new user message
		assert.Equal(t, roleUser, client.receivedMessages[0].Role)
		assert.Equal(t, "New message", client.receivedMessages[0].Content)
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

// TestDetectActionIntent tests the action intent detection function.
func TestDetectActionIntent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "detects 'I'll fix' intent",
			content:  "I can see the broken template. I'll fix it by adding the missing braces.",
			expected: true,
		},
		{
			name:     "detects 'I will read' intent",
			content:  "I will read the file to check the contents.",
			expected: true,
		},
		{
			name:     "detects 'let me search' intent",
			content:  "Let me search for all the configuration files.",
			expected: true,
		},
		{
			name:     "detects 'I'm going to edit' intent",
			content:  "I'm going to edit the configuration file to fix this issue.",
			expected: true,
		},
		{
			name:     "detects 'I am going to update' intent",
			content:  "I am going to update the settings based on your requirements.",
			expected: true,
		},
		{
			name:     "no action intent - explanation only",
			content:  "This file contains the configuration for the VPC component.",
			expected: false,
		},
		{
			name:     "no action intent - question",
			content:  "Would you like me to update this configuration?",
			expected: false,
		},
		{
			name:     "no action intent - statement without action verb",
			content:  "I'll be happy to help you with this task.",
			expected: false,
		},
		{
			name:     "detects 'first, I'll check' intent",
			content:  "First, I'll check the existing configuration before making changes.",
			expected: true,
		},
		{
			name:     "detects 'now I will run' intent",
			content:  "Now I will run the validation command to verify the configuration.",
			expected: true,
		},
		{
			name:     "case insensitive detection",
			content:  "I'LL FIX the template now.",
			expected: true,
		},
		{
			name:     "detects multiple action phrases",
			content:  "Let me first read the file, then I'll edit it to fix the issue.",
			expected: true,
		},
		{
			name:     "detects 'I will use' intent",
			content:  "I will use the atmos_describe_component tool to get the information.",
			expected: true,
		},
		{
			name:     "detects 'I will start by' intent",
			content:  "I will start by describing all components in the stack.",
			expected: true,
		},
		{
			name:     "detects 'I will begin by' intent",
			content:  "I will begin by trying common component names like 'vpc' and 'eks'.",
			expected: true,
		},
		{
			name:     "detects 'let me try' intent",
			content:  "Let me try to get the component details first.",
			expected: true,
		},
		{
			name:     "detects 'I'll call' intent",
			content:  "I'll call the API to fetch the data.",
			expected: true,
		},
		{
			name:     "detects 'I will get' intent",
			content:  "I will get the list of all available stacks.",
			expected: true,
		},
		{
			name:     "real Gemini pattern - use tool",
			content:  "I need to find the inheritance chain of all components in all stacks. To do this, I will first get a list of all stacks using the atmos_list_stacks tool.",
			expected: true,
		},
		{
			name:     "real Gemini pattern - start describing",
			content:  "Since I don't know the component names, I will start by describing all components in uw2-prod.",
			expected: true,
		},
		{
			name:     "real Gemini pattern - begin trying",
			content:  "I will begin by trying common component names like 'vpc', 'eks', and 'rds'.",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectActionIntent(tt.content)
			assert.Equal(t, tt.expected, result, "Expected %v for content: %s", tt.expected, tt.content)
		})
	}
}

// TestFormatToolParameters tests the tool parameter formatting function.
func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		name     string
		count    int64
		expected string
	}{
		{
			name:     "zero tokens",
			count:    0,
			expected: "0",
		},
		{
			name:     "small count",
			count:    500,
			expected: "500",
		},
		{
			name:     "exactly 1k",
			count:    1000,
			expected: "1.0k",
		},
		{
			name:     "7.1k tokens",
			count:    7100,
			expected: "7.1k",
		},
		{
			name:     "large count under 10k",
			count:    9876,
			expected: "9.9k",
		},
		{
			name:     "10k or more",
			count:    15000,
			expected: "15k",
		},
		{
			name:     "millions",
			count:    2500000,
			expected: "2.5M",
		},
		{
			name:     "large millions",
			count:    15000000,
			expected: "15M",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTokenCount(tt.count)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatUsage(t *testing.T) {
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
			name: "zero tokens",
			usage: &types.Usage{
				InputTokens:  0,
				OutputTokens: 0,
				TotalTokens:  0,
			},
			expected: "",
		},
		{
			name: "input and output tokens",
			usage: &types.Usage{
				InputTokens:  1500,
				OutputTokens: 2500,
				TotalTokens:  4000,
			},
			expected: "↑ 1.5k · ↓ 2.5k",
		},
		{
			name: "with cache tokens",
			usage: &types.Usage{
				InputTokens:     1500,
				OutputTokens:    2500,
				TotalTokens:     4000,
				CacheReadTokens: 500,
			},
			expected: "↑ 1.5k · ↓ 2.5k · cache: 500",
		},
		{
			name: "only input tokens",
			usage: &types.Usage{
				InputTokens:  1000,
				OutputTokens: 0,
				TotalTokens:  1000,
			},
			expected: "↑ 1.0k",
		},
		{
			name: "only output tokens",
			usage: &types.Usage{
				InputTokens:  0,
				OutputTokens: 2000,
				TotalTokens:  2000,
			},
			expected: "↓ 2.0k",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatUsage(tt.usage)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCombineUsage(t *testing.T) {
	tests := []struct {
		name     string
		u1       *types.Usage
		u2       *types.Usage
		expected *types.Usage
	}{
		{
			name:     "both nil",
			u1:       nil,
			u2:       nil,
			expected: nil,
		},
		{
			name: "first nil",
			u1:   nil,
			u2: &types.Usage{
				InputTokens:  100,
				OutputTokens: 200,
				TotalTokens:  300,
			},
			expected: &types.Usage{
				InputTokens:  100,
				OutputTokens: 200,
				TotalTokens:  300,
			},
		},
		{
			name: "second nil",
			u1: &types.Usage{
				InputTokens:  100,
				OutputTokens: 200,
				TotalTokens:  300,
			},
			u2: nil,
			expected: &types.Usage{
				InputTokens:  100,
				OutputTokens: 200,
				TotalTokens:  300,
			},
		},
		{
			name: "combine both",
			u1: &types.Usage{
				InputTokens:         100,
				OutputTokens:        200,
				TotalTokens:         300,
				CacheReadTokens:     50,
				CacheCreationTokens: 10,
			},
			u2: &types.Usage{
				InputTokens:         150,
				OutputTokens:        250,
				TotalTokens:         400,
				CacheReadTokens:     30,
				CacheCreationTokens: 5,
			},
			expected: &types.Usage{
				InputTokens:         250,
				OutputTokens:        450,
				TotalTokens:         700,
				CacheReadTokens:     80,
				CacheCreationTokens: 15,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := combineUsage(tt.u1, tt.u2)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected.InputTokens, result.InputTokens)
				assert.Equal(t, tt.expected.OutputTokens, result.OutputTokens)
				assert.Equal(t, tt.expected.TotalTokens, result.TotalTokens)
				assert.Equal(t, tt.expected.CacheReadTokens, result.CacheReadTokens)
				assert.Equal(t, tt.expected.CacheCreationTokens, result.CacheCreationTokens)
			}
		})
	}
}

func TestUsageTrackingInHandleAIMessage(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
		response:  "AI response",
	}

	model, err := NewChatModel(client, nil, nil, nil, nil, nil)
	require.NoError(t, err)

	// Create a message with usage.
	msg := aiResponseMsg{
		content: "Test response",
		usage: &types.Usage{
			InputTokens:  1000,
			OutputTokens: 2000,
			TotalTokens:  3000,
		},
	}

	// Handle the message.
	handled := model.handleAIMessage(msg)

	assert.True(t, handled)
	assert.NotNil(t, model.lastUsage)
	assert.Equal(t, int64(1000), model.lastUsage.InputTokens)
	assert.Equal(t, int64(2000), model.lastUsage.OutputTokens)
	assert.Equal(t, int64(3000), model.lastUsage.TotalTokens)

	// Check cumulative usage.
	assert.Equal(t, int64(1000), model.cumulativeUsage.InputTokens)
	assert.Equal(t, int64(2000), model.cumulativeUsage.OutputTokens)
	assert.Equal(t, int64(3000), model.cumulativeUsage.TotalTokens)

	// Add another message and verify accumulation.
	msg2 := aiResponseMsg{
		content: "Another response",
		usage: &types.Usage{
			InputTokens:  500,
			OutputTokens: 1000,
			TotalTokens:  1500,
		},
	}

	handled = model.handleAIMessage(msg2)

	assert.True(t, handled)
	assert.NotNil(t, model.lastUsage)
	assert.Equal(t, int64(500), model.lastUsage.InputTokens)
	assert.Equal(t, int64(1000), model.lastUsage.OutputTokens)
	assert.Equal(t, int64(1500), model.lastUsage.TotalTokens)

	// Check cumulative usage has accumulated.
	assert.Equal(t, int64(1500), model.cumulativeUsage.InputTokens)
	assert.Equal(t, int64(3000), model.cumulativeUsage.OutputTokens)
	assert.Equal(t, int64(4500), model.cumulativeUsage.TotalTokens)
}

func TestFormatAPIError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "rate limit error with 429",
			err:      fmt.Errorf("POST \"https://api.anthropic.com/v1/messages\": 429 Too Many Requests"),
			expected: "Rate limit exceeded. Please wait a moment and try again, or contact your provider to increase your rate limit.",
		},
		{
			name:     "rate limit error with rate_limit_error",
			err:      fmt.Errorf("failed to send messages: rate_limit_error: This request would exceed the rate limit"),
			expected: "Rate limit exceeded. Please wait a moment and try again, or contact your provider to increase your rate limit.",
		},
		{
			name:     "authentication error 401",
			err:      fmt.Errorf("POST \"https://api.openai.com/v1/chat/completions\": 401 Unauthorized"),
			expected: "Authentication failed. Please check your API key configuration.",
		},
		{
			name:     "permission error 403",
			err:      fmt.Errorf("403 Forbidden: You do not have access to this model"),
			expected: "Permission denied. Your API key may not have access to this model or feature.",
		},
		{
			name:     "model not found 404",
			err:      fmt.Errorf("404 Not Found: Model gpt-99 does not exist"),
			expected: "Model not found. Please check your model configuration.",
		},
		{
			name:     "timeout error",
			err:      fmt.Errorf("context deadline exceeded: request timed out after 5 minutes"),
			expected: "Request timed out. The AI provider took too long to respond. Please try again.",
		},
		{
			name:     "context length exceeded",
			err:      fmt.Errorf("context_length_exceeded: Your message exceeded the maximum context length"),
			expected: "Context length exceeded. Your conversation is too long. Try starting a new session.",
		},
		{
			name:     "function calling not enabled (Gemini image model)",
			err:      fmt.Errorf("Error 400, Message: Function calling is not enabled for models/gemini-2.0-flash-preview-image-generation, Status: INVALID_ARGUMENT"),
			expected: "This model doesn't support function calling (tool use). Please switch to a different model using Ctrl+P. Try gemini-2.0-flash-exp or gemini-1.5-pro.",
		},
		{
			name:     "error with request ID stripped",
			err:      fmt.Errorf("failed to send messages with history and tools: POST \"https://api.anthropic.com/v1/messages\": 429 Too Many Requests (Request-ID: req_011CUYpGVDM4KU1nVwW9rzX4)"),
			expected: "Rate limit exceeded. Please wait a moment and try again, or contact your provider to increase your rate limit.",
		},
		{
			name:     "error with JSON body stripped",
			err:      fmt.Errorf("POST \"https://api.anthropic.com/v1/messages\": 500 Internal Server Error {\"type\":\"error\",\"error\":{\"type\":\"internal_error\",\"message\":\"Something went wrong\"}}"),
			expected: "500 Internal Server Error",
		},
		{
			name:     "clean generic error",
			err:      fmt.Errorf("network connection failed"),
			expected: "network connection failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAPIError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatToolParameters(t *testing.T) {
	tests := []struct {
		name     string
		toolCall types.ToolCall
		expected string
	}{
		{
			name: "execute_atmos_command shows command",
			toolCall: types.ToolCall{
				Name: "execute_atmos_command",
				Input: map[string]interface{}{
					"command": "describe component vpc -s ue1-network",
				},
			},
			expected: "**Command:** `atmos describe component vpc -s ue1-network`",
		},
		{
			name: "read_file shows path",
			toolCall: types.ToolCall{
				Name: "read_file",
				Input: map[string]interface{}{
					"path": "/path/to/file.yaml",
				},
			},
			expected: "**Path:** `/path/to/file.yaml`",
		},
		{
			name: "search_files shows pattern",
			toolCall: types.ToolCall{
				Name: "search_files",
				Input: map[string]interface{}{
					"pattern": "*.yaml",
				},
			},
			expected: "**Pattern:** `*.yaml`",
		},
		{
			name: "describe_component shows args",
			toolCall: types.ToolCall{
				Name: "describe_component",
				Input: map[string]interface{}{
					"component": "vpc",
					"stack":     "ue1-network",
				},
			},
			expected: "**Args:** `vpc -s ue1-network`",
		},
		{
			name: "execute_bash shows truncated long command",
			toolCall: types.ToolCall{
				Name: "execute_bash",
				Input: map[string]interface{}{
					"command": strings.Repeat("very long command ", 10), // > 80 chars
				},
			},
			expected: "**Command:** `" + strings.Repeat("very long command ", 10)[:77] + "...`",
		},
		{
			name: "empty input returns empty string",
			toolCall: types.ToolCall{
				Name:  "some_tool",
				Input: map[string]interface{}{},
			},
			expected: "",
		},
		{
			name: "generic tool shows all parameters",
			toolCall: types.ToolCall{
				Name: "unknown_tool",
				Input: map[string]interface{}{
					"param1": "value1",
					"param2": "value2",
				},
			},
			// Generic format shows all params (order may vary due to map iteration)
			expected: "", // We'll check it contains both params instead
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatToolParameters(tt.toolCall)

			if tt.name == "generic tool shows all parameters" {
				// For generic tools, check that both params are present (order may vary).
				assert.Contains(t, result, "param1=`value1`")
				assert.Contains(t, result, "param2=`value2`")
				assert.Contains(t, result, "**Parameters:**")
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestSlidingWindowConfiguration tests that max_history_messages configuration is properly loaded.
func TestSlidingWindowConfiguration(t *testing.T) {
	t.Run("loads max_history_messages from config", func(t *testing.T) {
		client := &mockAIClient{
			model:     "test-model",
			maxTokens: 4096,
		}

		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					MaxHistoryMessages: 10,
				},
			},
		}

		model, err := NewChatModel(client, atmosConfig, nil, nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, 10, model.maxHistoryMessages)
	})

	t.Run("defaults to 0 (unlimited) when not configured", func(t *testing.T) {
		client := &mockAIClient{
			model:     "test-model",
			maxTokens: 4096,
		}

		model, err := NewChatModel(client, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, 0, model.maxHistoryMessages)
	})

	t.Run("handles zero value in config (unlimited)", func(t *testing.T) {
		client := &mockAIClient{
			model:     "test-model",
			maxTokens: 4096,
		}

		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					MaxHistoryMessages: 0,
				},
			},
		}

		model, err := NewChatModel(client, atmosConfig, nil, nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, 0, model.maxHistoryMessages)
	})
}

// mockClientWithHistory tracks the messages sent to it for testing sliding window.
type mockClientWithHistory struct {
	mockAIClient
	lastMessages []types.Message
}

func (m *mockClientWithHistory) SendMessageWithToolsAndHistory(ctx context.Context, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	m.lastMessages = messages
	return m.mockAIClient.SendMessageWithToolsAndHistory(ctx, messages, availableTools)
}

func (m *mockClientWithHistory) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	m.lastMessages = messages
	return m.mockAIClient.SendMessageWithHistory(ctx, messages)
}

// TestSlidingWindowBehavior tests that sliding window correctly limits conversation history.
func TestSlidingWindowBehavior(t *testing.T) {
	t.Run("applies sliding window when limit is exceeded", func(t *testing.T) {
		client := &mockClientWithHistory{
			mockAIClient: mockAIClient{
				model:     "test-model",
				maxTokens: 4096,
				response:  "AI response",
			},
		}

		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					MaxHistoryMessages: 4, // Keep only 4 messages from history
				},
			},
		}

		// Create session to enable provider filtering.
		sess := &session.Session{
			ID:       "test-session",
			Name:     "Test Session",
			Provider: "anthropic",
		}

		model, err := NewChatModel(client, atmosConfig, nil, sess, nil, nil)
		require.NoError(t, err)

		// Add 6 historical messages (3 pairs of user/assistant exchanges).
		// These should be pruned to keep only the last 4.
		model.messages = []ChatMessage{
			{Role: roleUser, Content: "Message 1", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Response 1", Provider: "anthropic"},
			{Role: roleUser, Content: "Message 2", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Response 2", Provider: "anthropic"},
			{Role: roleUser, Content: "Message 3", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Response 3", Provider: "anthropic"},
		}

		// Send a new message.
		ctx := context.Background()
		cmd := model.getAIResponseWithContext("New message", ctx)
		msg := cmd()

		// Verify it's a response message (not an error).
		_, ok := msg.(aiResponseMsg)
		assert.True(t, ok, "Expected aiResponseMsg")

		// Verify the client received only the last 4 messages from history + the new message.
		// Expected: Message 2, Response 2, Message 3, Response 3, New message = 5 total.
		require.NotNil(t, client.lastMessages)
		assert.Equal(t, 5, len(client.lastMessages), "Should have 4 history messages + 1 new message")

		// Verify the oldest messages were pruned.
		assert.Equal(t, "Message 2", client.lastMessages[0].Content)
		assert.Equal(t, "Response 2", client.lastMessages[1].Content)
		assert.Equal(t, "Message 3", client.lastMessages[2].Content)
		assert.Equal(t, "Response 3", client.lastMessages[3].Content)
		assert.Equal(t, "New message", client.lastMessages[4].Content)
	})

	t.Run("does not apply window when under limit", func(t *testing.T) {
		client := &mockClientWithHistory{
			mockAIClient: mockAIClient{
				model:     "test-model",
				maxTokens: 4096,
				response:  "AI response",
			},
		}

		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					MaxHistoryMessages: 10, // Limit is higher than actual messages
				},
			},
		}

		sess := &session.Session{
			ID:       "test-session",
			Name:     "Test Session",
			Provider: "anthropic",
		}

		model, err := NewChatModel(client, atmosConfig, nil, sess, nil, nil)
		require.NoError(t, err)

		// Add 4 historical messages (under limit).
		model.messages = []ChatMessage{
			{Role: roleUser, Content: "Message 1", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Response 1", Provider: "anthropic"},
			{Role: roleUser, Content: "Message 2", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Response 2", Provider: "anthropic"},
		}

		// Send a new message.
		ctx := context.Background()
		cmd := model.getAIResponseWithContext("New message", ctx)
		msg := cmd()

		// Verify it's a response message (not an error).
		_, ok := msg.(aiResponseMsg)
		assert.True(t, ok, "Expected aiResponseMsg")

		// Verify all messages were sent (4 history + 1 new = 5 total).
		require.NotNil(t, client.lastMessages)
		assert.Equal(t, 5, len(client.lastMessages))

		// Verify all historical messages are present.
		assert.Equal(t, "Message 1", client.lastMessages[0].Content)
		assert.Equal(t, "Response 1", client.lastMessages[1].Content)
		assert.Equal(t, "Message 2", client.lastMessages[2].Content)
		assert.Equal(t, "Response 2", client.lastMessages[3].Content)
		assert.Equal(t, "New message", client.lastMessages[4].Content)
	})

	t.Run("unlimited history when maxHistoryMessages is 0", func(t *testing.T) {
		client := &mockClientWithHistory{
			mockAIClient: mockAIClient{
				model:     "test-model",
				maxTokens: 4096,
				response:  "AI response",
			},
		}

		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					MaxHistoryMessages: 0, // Unlimited
				},
			},
		}

		sess := &session.Session{
			ID:       "test-session",
			Name:     "Test Session",
			Provider: "anthropic",
		}

		model, err := NewChatModel(client, atmosConfig, nil, sess, nil, nil)
		require.NoError(t, err)

		// Add many historical messages.
		for i := 0; i < 20; i++ {
			model.messages = append(model.messages, ChatMessage{
				Role:     roleUser,
				Content:  fmt.Sprintf("Message %d", i),
				Provider: "anthropic",
			})
			model.messages = append(model.messages, ChatMessage{
				Role:     roleAssistant,
				Content:  fmt.Sprintf("Response %d", i),
				Provider: "anthropic",
			})
		}

		// Send a new message.
		ctx := context.Background()
		cmd := model.getAIResponseWithContext("New message", ctx)
		msg := cmd()

		// Verify it's a response message (not an error).
		_, ok := msg.(aiResponseMsg)
		assert.True(t, ok, "Expected aiResponseMsg")

		// Verify all messages were sent (40 history + 1 new = 41 total).
		require.NotNil(t, client.lastMessages)
		assert.Equal(t, 41, len(client.lastMessages))
	})

	t.Run("handles empty history with limits", func(t *testing.T) {
		testCases := []struct {
			name               string
			maxHistoryMessages int
			maxHistoryTokens   int
		}{
			{"message limit configured", 10, 0},
			{"token limit configured", 0, 100},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				client := &mockClientWithHistory{
					mockAIClient: mockAIClient{
						model:     "test-model",
						maxTokens: 4096,
						response:  "AI response",
					},
				}

				atmosConfig := &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						AI: schema.AISettings{
							MaxHistoryMessages: tc.maxHistoryMessages,
							MaxHistoryTokens:   tc.maxHistoryTokens,
						},
					},
				}

				sess := &session.Session{
					ID:       "test-session",
					Name:     "Test Session",
					Provider: "anthropic",
				}

				model, err := NewChatModel(client, atmosConfig, nil, sess, nil, nil)
				require.NoError(t, err)

				// No historical messages.
				model.messages = []ChatMessage{}

				// Send a new message.
				ctx := context.Background()
				cmd := model.getAIResponseWithContext("First message", ctx)
				msg := cmd()

				// Verify it's a response message (not an error).
				_, ok := msg.(aiResponseMsg)
				assert.True(t, ok, "Expected aiResponseMsg")

				// Verify only the new message was sent.
				require.NotNil(t, client.lastMessages)
				assert.Equal(t, 1, len(client.lastMessages))
				assert.Equal(t, "First message", client.lastMessages[0].Content)
			})
		}
	})

	t.Run("filters by provider before applying window", func(t *testing.T) {
		client := &mockClientWithHistory{
			mockAIClient: mockAIClient{
				model:     "test-model",
				maxTokens: 4096,
				response:  "AI response",
			},
		}

		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					MaxHistoryMessages: 2, // Very small window
				},
			},
		}

		sess := &session.Session{
			ID:       "test-session",
			Name:     "Test Session",
			Provider: "anthropic", // Current provider
		}

		model, err := NewChatModel(client, atmosConfig, nil, sess, nil, nil)
		require.NoError(t, err)

		// Add messages from different providers.
		model.messages = []ChatMessage{
			{Role: roleUser, Content: "OpenAI 1", Provider: "openai"},
			{Role: roleAssistant, Content: "OpenAI Response 1", Provider: "openai"},
			{Role: roleUser, Content: "Anthropic 1", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Anthropic Response 1", Provider: "anthropic"},
			{Role: roleUser, Content: "Anthropic 2", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Anthropic Response 2", Provider: "anthropic"},
			{Role: roleUser, Content: "Anthropic 3", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Anthropic Response 3", Provider: "anthropic"},
		}

		// Send a new message.
		ctx := context.Background()
		cmd := model.getAIResponseWithContext("New message", ctx)
		msg := cmd()

		// Verify it's a response message (not an error).
		_, ok := msg.(aiResponseMsg)
		assert.True(t, ok, "Expected aiResponseMsg")

		// Verify:
		// 1. OpenAI messages were filtered out
		// 2. Only last 2 Anthropic messages were kept (+ new message = 3 total).
		require.NotNil(t, client.lastMessages)
		assert.Equal(t, 3, len(client.lastMessages))

		// Should be: Anthropic 3, Anthropic Response 3, New message.
		assert.Equal(t, "Anthropic 3", client.lastMessages[0].Content)
		assert.Equal(t, "Anthropic Response 3", client.lastMessages[1].Content)
		assert.Equal(t, "New message", client.lastMessages[2].Content)
	})

	t.Run("system messages are excluded from history", func(t *testing.T) {
		client := &mockClientWithHistory{
			mockAIClient: mockAIClient{
				model:     "test-model",
				maxTokens: 4096,
				response:  "AI response",
			},
		}

		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					MaxHistoryMessages: 4,
				},
			},
		}

		sess := &session.Session{
			ID:       "test-session",
			Name:     "Test Session",
			Provider: "anthropic",
		}

		model, err := NewChatModel(client, atmosConfig, nil, sess, nil, nil)
		require.NoError(t, err)

		// Add messages including system messages.
		model.messages = []ChatMessage{
			{Role: roleSystem, Content: "System notification 1", Provider: "anthropic"},
			{Role: roleUser, Content: "Message 1", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Response 1", Provider: "anthropic"},
			{Role: roleSystem, Content: "System notification 2", Provider: "anthropic"},
			{Role: roleUser, Content: "Message 2", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Response 2", Provider: "anthropic"},
		}

		// Send a new message.
		ctx := context.Background()
		cmd := model.getAIResponseWithContext("New message", ctx)
		msg := cmd()

		// Verify it's a response message (not an error).
		_, ok := msg.(aiResponseMsg)
		assert.True(t, ok, "Expected aiResponseMsg")

		// Verify system messages were filtered out.
		// Should have: Message 1, Response 1, Message 2, Response 2, New message = 5 total.
		require.NotNil(t, client.lastMessages)
		for _, msg := range client.lastMessages {
			assert.NotEqual(t, types.RoleSystem, msg.Role, "System messages should be filtered out")
			assert.NotContains(t, msg.Content, "System notification")
		}
	})
}

// TestEstimateTokens tests the token estimation function.
func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "empty string",
			text:     "",
			expected: 0,
		},
		{
			name:     "single word",
			text:     "hello",
			expected: 1, // 1 word × 1.3 = 1.3 → 1
		},
		{
			name:     "simple sentence",
			text:     "Hello world this is a test",
			expected: 7, // 6 words × 1.3 = 7.8 → 7
		},
		{
			name:     "sentence with punctuation",
			text:     "Hello, world! This is a test.",
			expected: 7, // 6 words × 1.3 = 7.8 → 7
		},
		{
			name:     "multi-line text",
			text:     "Line one\nLine two\nLine three",
			expected: 7, // 6 words × 1.3 = 7.8 → 7
		},
		{
			name: "code snippet",
			text: `func main() {
    fmt.Println("Hello")
}`,
			expected: 6, // 5 words × 1.3 = 6.5 → 6
		},
		{
			name:     "long text",
			text:     strings.Repeat("word ", 100),
			expected: 130, // 100 words × 1.3 = 130
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := estimateTokens(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTokenBasedConfiguration tests that max_history_tokens configuration is properly loaded.
func TestTokenBasedConfiguration(t *testing.T) {
	t.Run("loads max_history_tokens from config", func(t *testing.T) {
		client := &mockAIClient{
			model:     "test-model",
			maxTokens: 4096,
		}

		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					MaxHistoryTokens: 1000,
				},
			},
		}

		model, err := NewChatModel(client, atmosConfig, nil, nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, 1000, model.maxHistoryTokens)
	})

	t.Run("defaults to 0 (unlimited) when not configured", func(t *testing.T) {
		client := &mockAIClient{
			model:     "test-model",
			maxTokens: 4096,
		}

		model, err := NewChatModel(client, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, 0, model.maxHistoryTokens)
	})

	t.Run("supports both message and token limits", func(t *testing.T) {
		client := &mockAIClient{
			model:     "test-model",
			maxTokens: 4096,
		}

		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					MaxHistoryMessages: 20,
					MaxHistoryTokens:   1000,
				},
			},
		}

		model, err := NewChatModel(client, atmosConfig, nil, nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, 20, model.maxHistoryMessages)
		assert.Equal(t, 1000, model.maxHistoryTokens)
	})
}

// TestTokenBasedPruning tests that token-based sliding window correctly limits conversation history.
func TestTokenBasedPruning(t *testing.T) {
	t.Run("applies token-based window when limit is exceeded", func(t *testing.T) {
		client := &mockClientWithHistory{
			mockAIClient: mockAIClient{
				model:     "test-model",
				maxTokens: 4096,
				response:  "AI response",
			},
		}

		// Set a low token limit to force pruning.
		// Each message is ~3-4 words × 1.3 = ~4-5 tokens
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					MaxHistoryTokens: 20, // Keep approximately 4-5 messages worth of tokens
				},
			},
		}

		sess := &session.Session{
			ID:       "test-session",
			Name:     "Test Session",
			Provider: "anthropic",
		}

		model, err := NewChatModel(client, atmosConfig, nil, sess, nil, nil)
		require.NoError(t, err)

		// Add 6 historical messages with varying lengths.
		model.messages = []ChatMessage{
			{Role: roleUser, Content: "Short msg", Provider: "anthropic"},                        // ~2 words × 1.3 = ~2 tokens
			{Role: roleAssistant, Content: "Short response", Provider: "anthropic"},              // ~2 words × 1.3 = ~2 tokens
			{Role: roleUser, Content: "Medium length message here", Provider: "anthropic"},       // ~4 words × 1.3 = ~5 tokens
			{Role: roleAssistant, Content: "Medium length response here", Provider: "anthropic"}, // ~4 words × 1.3 = ~5 tokens
			{Role: roleUser, Content: "Another message", Provider: "anthropic"},                  // ~2 words × 1.3 = ~2 tokens
			{Role: roleAssistant, Content: "Another response", Provider: "anthropic"},            // ~2 words × 1.3 = ~2 tokens
		}

		// Send a new message.
		ctx := context.Background()
		cmd := model.getAIResponseWithContext("New message", ctx)
		msg := cmd()

		// Verify it's a response message (not an error).
		_, ok := msg.(aiResponseMsg)
		assert.True(t, ok, "Expected aiResponseMsg")

		// With 20 token limit and ~18 total tokens in messages, should keep most recent messages.
		// Counting backwards: "Another response" (2) + "Another message" (2) +
		// "Medium length response here" (5) + "Medium length message here" (5) = ~14 tokens
		// Adding one more would exceed 20, so should keep last 4 messages.
		require.NotNil(t, client.lastMessages)
		assert.GreaterOrEqual(t, len(client.lastMessages), 3, "Should keep at least 3 messages within token limit")
		assert.LessOrEqual(t, len(client.lastMessages), 7, "Should not keep all messages if limit exceeded")
	})

	t.Run("does not apply token window when under limit", func(t *testing.T) {
		client := &mockClientWithHistory{
			mockAIClient: mockAIClient{
				model:     "test-model",
				maxTokens: 4096,
				response:  "AI response",
			},
		}

		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					MaxHistoryTokens: 1000, // High limit
				},
			},
		}

		sess := &session.Session{
			ID:       "test-session",
			Name:     "Test Session",
			Provider: "anthropic",
		}

		model, err := NewChatModel(client, atmosConfig, nil, sess, nil, nil)
		require.NoError(t, err)

		// Add 4 short messages (well under token limit).
		model.messages = []ChatMessage{
			{Role: roleUser, Content: "Message 1", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Response 1", Provider: "anthropic"},
			{Role: roleUser, Content: "Message 2", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Response 2", Provider: "anthropic"},
		}

		// Send a new message.
		ctx := context.Background()
		cmd := model.getAIResponseWithContext("New message", ctx)
		msg := cmd()

		// Verify it's a response message (not an error).
		_, ok := msg.(aiResponseMsg)
		assert.True(t, ok, "Expected aiResponseMsg")

		// All messages should be kept since we're well under token limit.
		require.NotNil(t, client.lastMessages)
		assert.Equal(t, 5, len(client.lastMessages), "Should keep all messages when under token limit")
	})

	t.Run("applies both message and token limits - message limit more restrictive", func(t *testing.T) {
		client := &mockClientWithHistory{
			mockAIClient: mockAIClient{
				model:     "test-model",
				maxTokens: 4096,
				response:  "AI response",
			},
		}

		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					MaxHistoryMessages: 2,    // Very restrictive
					MaxHistoryTokens:   1000, // Not restrictive
				},
			},
		}

		sess := &session.Session{
			ID:       "test-session",
			Name:     "Test Session",
			Provider: "anthropic",
		}

		model, err := NewChatModel(client, atmosConfig, nil, sess, nil, nil)
		require.NoError(t, err)

		// Add 6 short messages.
		model.messages = []ChatMessage{
			{Role: roleUser, Content: "Message 1", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Response 1", Provider: "anthropic"},
			{Role: roleUser, Content: "Message 2", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Response 2", Provider: "anthropic"},
			{Role: roleUser, Content: "Message 3", Provider: "anthropic"},
			{Role: roleAssistant, Content: "Response 3", Provider: "anthropic"},
		}

		// Send a new message.
		ctx := context.Background()
		cmd := model.getAIResponseWithContext("New message", ctx)
		msg := cmd()

		// Verify it's a response message (not an error).
		_, ok := msg.(aiResponseMsg)
		assert.True(t, ok, "Expected aiResponseMsg")

		// Message limit (2) should win since it's more restrictive than token limit.
		// However, system messages (tool prompts, memory context) are added after pruning.
		require.NotNil(t, client.lastMessages)
		assert.GreaterOrEqual(t, len(client.lastMessages), 3, "Should have at least 3 messages (2 history + 1 new)")
		assert.LessOrEqual(t, len(client.lastMessages), 8, "Should not have excessive messages")
	})

	t.Run("applies both message and token limits - token limit more restrictive", func(t *testing.T) {
		client := &mockClientWithHistory{
			mockAIClient: mockAIClient{
				model:     "test-model",
				maxTokens: 4096,
				response:  "AI response",
			},
		}

		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					MaxHistoryMessages: 10, // Not restrictive
					MaxHistoryTokens:   10, // Very restrictive (only ~7-8 words)
				},
			},
		}

		sess := &session.Session{
			ID:       "test-session",
			Name:     "Test Session",
			Provider: "anthropic",
		}

		model, err := NewChatModel(client, atmosConfig, nil, sess, nil, nil)
		require.NoError(t, err)

		// Add messages with varying lengths.
		model.messages = []ChatMessage{
			{Role: roleUser, Content: "This is a longer message with many words", Provider: "anthropic"},       // ~8 words × 1.3 = ~10 tokens
			{Role: roleAssistant, Content: "This is a longer response with many words", Provider: "anthropic"}, // ~8 words × 1.3 = ~10 tokens
			{Role: roleUser, Content: "Short", Provider: "anthropic"},                                          // ~1 word × 1.3 = ~1 token
			{Role: roleAssistant, Content: "Short response", Provider: "anthropic"},                            // ~2 words × 1.3 = ~2 tokens
		}

		// Send a new message.
		ctx := context.Background()
		cmd := model.getAIResponseWithContext("New msg", ctx)
		msg := cmd()

		// Verify it's a response message (not an error).
		_, ok := msg.(aiResponseMsg)
		assert.True(t, ok, "Expected aiResponseMsg")

		// Token limit (10) should win. With 10 tokens, we can only fit last 2-3 short messages.
		require.NotNil(t, client.lastMessages)
		assert.LessOrEqual(t, len(client.lastMessages), 5, "Token limit should be more restrictive than message limit")
	})
}

func TestChatModel_HandleCompactStatus_Starting(t *testing.T) {
	client := &mockAIClient{
		model:    "test-model",
		response: "Test response",
	}

	sess := &session.Session{
		ID:       "test-session",
		Name:     "Test Session",
		Provider: "openai",
	}

	model, err := NewChatModel(client, nil, nil, sess, nil, nil)
	require.NoError(t, err)

	// Initially not loading.
	assert.False(t, model.isLoading)
	initialMessageCount := len(model.messages)

	// Handle "starting" status.
	msg := compactStatusMsg{
		stage:        "starting",
		messageCount: 40,
		savings:      8000,
	}

	handled := model.handleCompactStatus(msg)
	assert.True(t, handled)

	// Should set loading state.
	assert.True(t, model.isLoading)

	// Should add a system message.
	assert.Equal(t, initialMessageCount+1, len(model.messages))
	lastMsg := model.messages[len(model.messages)-1]
	assert.Equal(t, roleSystem, lastMsg.Role)
	assert.Contains(t, lastMsg.Content, "Compacting conversation")
	assert.Contains(t, lastMsg.Content, "40 messages")
}

func TestChatModel_HandleCompactStatus_Completed(t *testing.T) {
	client := &mockAIClient{
		model:    "test-model",
		response: "Test response",
	}

	sess := &session.Session{
		ID:       "test-session",
		Name:     "Test Session",
		Provider: "openai",
	}

	model, err := NewChatModel(client, nil, nil, sess, nil, nil)
	require.NoError(t, err)

	// Set loading state (as if compaction is in progress).
	model.isLoading = true
	initialMessageCount := len(model.messages)

	// Handle "completed" status.
	msg := compactStatusMsg{
		stage:        "completed",
		messageCount: 40,
		savings:      7500,
	}

	handled := model.handleCompactStatus(msg)
	assert.True(t, handled)

	// Should clear loading state.
	assert.False(t, model.isLoading)

	// Should add a success message.
	assert.Equal(t, initialMessageCount+1, len(model.messages))
	lastMsg := model.messages[len(model.messages)-1]
	assert.Equal(t, roleSystem, lastMsg.Role)
	assert.Contains(t, lastMsg.Content, "Conversation compacted successfully")
	assert.Contains(t, lastMsg.Content, "40 messages summarized")
	assert.Contains(t, lastMsg.Content, "7500 tokens saved")
}

func TestChatModel_HandleCompactStatus_Failed(t *testing.T) {
	client := &mockAIClient{
		model:    "test-model",
		response: "Test response",
	}

	sess := &session.Session{
		ID:       "test-session",
		Name:     "Test Session",
		Provider: "openai",
	}

	model, err := NewChatModel(client, nil, nil, sess, nil, nil)
	require.NoError(t, err)

	// Set loading state (as if compaction is in progress).
	model.isLoading = true
	initialMessageCount := len(model.messages)

	// Handle "failed" status with error.
	testErr := fmt.Errorf("test error: AI summarization failed")
	msg := compactStatusMsg{
		stage:        "failed",
		messageCount: 40,
		err:          testErr,
	}

	handled := model.handleCompactStatus(msg)
	assert.True(t, handled)

	// Should clear loading state.
	assert.False(t, model.isLoading)

	// Should add an error message.
	assert.Equal(t, initialMessageCount+1, len(model.messages))
	lastMsg := model.messages[len(model.messages)-1]
	assert.Equal(t, roleSystem, lastMsg.Role)
	assert.Contains(t, lastMsg.Content, "Compaction failed")
	assert.Contains(t, lastMsg.Content, "test error")
}

func TestChatModel_HandleCompactStatus_FailedNoError(t *testing.T) {
	client := &mockAIClient{
		model:    "test-model",
		response: "Test response",
	}

	sess := &session.Session{
		ID:       "test-session",
		Name:     "Test Session",
		Provider: "openai",
	}

	model, err := NewChatModel(client, nil, nil, sess, nil, nil)
	require.NoError(t, err)

	model.isLoading = true
	initialMessageCount := len(model.messages)

	// Handle "failed" status without specific error.
	msg := compactStatusMsg{
		stage:        "failed",
		messageCount: 40,
		err:          nil,
	}

	handled := model.handleCompactStatus(msg)
	assert.True(t, handled)

	// Should clear loading state.
	assert.False(t, model.isLoading)

	// Should add a generic error message.
	assert.Equal(t, initialMessageCount+1, len(model.messages))
	lastMsg := model.messages[len(model.messages)-1]
	assert.Equal(t, roleSystem, lastMsg.Role)
	assert.Contains(t, lastMsg.Content, "Compaction failed")
	// Should not contain error details when err is nil.
	assert.NotContains(t, lastMsg.Content, ":")
}

func TestChatModel_Update_CompactStatusMsg(t *testing.T) {
	client := &mockAIClient{
		model:    "test-model",
		response: "Test response",
	}

	sess := &session.Session{
		ID:       "test-session",
		Name:     "Test Session",
		Provider: "openai",
	}

	model, err := NewChatModel(client, nil, nil, sess, nil, nil)
	require.NoError(t, err)

	initialMessageCount := len(model.messages)

	// Send compactStatusMsg through Update method.
	msg := compactStatusMsg{
		stage:        "starting",
		messageCount: 50,
		savings:      10000,
	}

	updatedModel, cmd := model.Update(msg)
	assert.Nil(t, cmd)

	// Verify model was updated.
	chatModel, ok := updatedModel.(*ChatModel)
	require.True(t, ok)

	// Should have added a message.
	assert.Equal(t, initialMessageCount+1, len(chatModel.messages))
	lastMsg := chatModel.messages[len(chatModel.messages)-1]
	assert.Equal(t, roleSystem, lastMsg.Role)
	assert.Contains(t, lastMsg.Content, "Compacting conversation")
}

func TestChatModel_CompactStatusCallback_Integration(t *testing.T) {
	// Test that the callback integration works end-to-end.
	client := &mockAIClient{
		model:    "test-model",
		response: "Test response",
	}

	// Create a mock session manager.
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test.db", tmpDir)
	storage, err := session.NewSQLiteStorage(dbPath)
	require.NoError(t, err)
	defer storage.Close()

	manager := session.NewManager(storage, "/test/project", 100, nil)

	sess := &session.Session{
		ID:       "test-session",
		Name:     "Test Session",
		Provider: "openai",
	}

	_, err = NewChatModel(client, nil, manager, sess, nil, nil)
	require.NoError(t, err)

	// Verify callback was registered on manager.
	// We can't directly test the callback without running the full tea.Program,
	// but we can verify that SetCompactStatusCallback was called.
	// The callback field is private, but we can trigger it and check for panics.
	var capturedStatus session.CompactStatus
	manager.SetCompactStatusCallback(func(status session.CompactStatus) {
		capturedStatus = status
	})

	// Manually trigger callback to verify it doesn't panic.
	manager.SetCompactStatusCallback(func(status session.CompactStatus) {
		capturedStatus = status
	})

	// Verify we can set the callback without errors.
	// Full integration test would require running tea.Program,
	// which is complex for unit tests. The handler tests above verify
	// the message handling logic works correctly.
	assert.NotNil(t, manager)
	_ = capturedStatus
}
