package session

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestManager creates a manager with test storage.
func setupTestManager(t *testing.T) (*Manager, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	require.NoError(t, err)

	manager := NewManager(storage, "/test/project", DefaultMaxSessions)

	cleanup := func() {
		storage.Close()
	}

	return manager, cleanup
}

func TestNewManager(t *testing.T) {
	tests := []struct {
		name        string
		maxSessions int
		want        int
	}{
		{
			name:        "uses provided max sessions",
			maxSessions: 5,
			want:        5,
		},
		{
			name:        "uses default for zero",
			maxSessions: 0,
			want:        DefaultMaxSessions,
		},
		{
			name:        "uses default for negative",
			maxSessions: -1,
			want:        DefaultMaxSessions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "test.db")
			storage, err := NewSQLiteStorage(dbPath)
			require.NoError(t, err)
			defer storage.Close()

			manager := NewManager(storage, "/test", tt.maxSessions)

			assert.NotNil(t, manager)
			assert.Equal(t, tt.want, manager.maxSessions)
		})
	}
}

func TestManager_CreateSession(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name     string
		sessName string
		model    string
		provider string
		metadata map[string]interface{}
		wantErr  bool
	}{
		{
			name:     "creates session with name",
			sessName: "my-session",
			model:    "gpt-4",
			provider: "openai",
			metadata: map[string]interface{}{"key": "value"},
			wantErr:  false,
		},
		{
			name:     "generates name for empty name",
			sessName: "",
			model:    "gpt-4",
			provider: "openai",
			wantErr:  false,
		},
		{
			name:     "handles nil metadata",
			sessName: "test",
			model:    "gpt-4",
			provider: "openai",
			metadata: nil,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := manager.CreateSession(ctx, tt.sessName, tt.model, tt.provider, tt.metadata)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, session)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, session)
				assert.NotEmpty(t, session.ID)
				assert.NotEmpty(t, session.Name)
				assert.Equal(t, tt.model, session.Model)
				assert.Equal(t, tt.provider, session.Provider)
				assert.Equal(t, "/test/project", session.ProjectPath)

				if tt.sessName != "" {
					assert.Equal(t, tt.sessName, session.Name)
				} else {
					assert.Contains(t, session.Name, "session-")
				}
			}
		})
	}
}

func TestManager_GetSession(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create a session.
	created, err := manager.CreateSession(ctx, "test", "gpt-4", "openai", nil)
	require.NoError(t, err)

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "retrieves existing session",
			id:      created.ID,
			wantErr: false,
		},
		{
			name:    "errors on non-existent session",
			id:      "non-existent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := manager.GetSession(ctx, tt.id)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, session)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, session)
				assert.Equal(t, tt.id, session.ID)
			}
		})
	}
}

func TestManager_GetSessionByName(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create sessions.
	_, err := manager.CreateSession(ctx, "test-session", "gpt-4", "openai", nil)
	require.NoError(t, err)

	tests := []struct {
		name     string
		sessName string
		wantErr  bool
	}{
		{
			name:     "retrieves existing session by name",
			sessName: "test-session",
			wantErr:  false,
		},
		{
			name:     "errors on non-existent session",
			sessName: "non-existent",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := manager.GetSessionByName(ctx, tt.sessName)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, session)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, session)
				assert.Equal(t, tt.sessName, session.Name)
			}
		})
	}
}

func TestManager_ListSessions(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple sessions.
	for i := 0; i < 5; i++ {
		_, err := manager.CreateSession(ctx, string(rune('a'+i)), "gpt-4", "openai", nil)
		require.NoError(t, err)
	}

	sessions, err := manager.ListSessions(ctx)
	assert.NoError(t, err)
	assert.Len(t, sessions, 5)

	// Verify sessions are sorted by updated_at DESC.
	if len(sessions) > 1 {
		for i := 0; i < len(sessions)-1; i++ {
			assert.True(t, sessions[i].UpdatedAt.After(sessions[i+1].UpdatedAt) || sessions[i].UpdatedAt.Equal(sessions[i+1].UpdatedAt))
		}
	}
}

func TestManager_AddMessage(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create session.
	session, err := manager.CreateSession(ctx, "test", "gpt-4", "openai", nil)
	require.NoError(t, err)

	tests := []struct {
		name      string
		sessionID string
		role      string
		content   string
		wantErr   bool
	}{
		{
			name:      "adds message successfully",
			sessionID: session.ID,
			role:      "user",
			content:   "Hello!",
			wantErr:   false,
		},
		{
			name:      "adds assistant message",
			sessionID: session.ID,
			role:      "assistant",
			content:   "Hi there!",
			wantErr:   false,
		},
		{
			name:      "errors on non-existent session",
			sessionID: "non-existent",
			role:      "user",
			content:   "test",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.AddMessage(ctx, tt.sessionID, tt.role, tt.content)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify message was added and session was updated.
				messages, err := manager.GetMessages(ctx, tt.sessionID, 0)
				assert.NoError(t, err)
				assert.NotEmpty(t, messages)

				// Find the added message.
				found := false
				for _, msg := range messages {
					if msg.Role == tt.role && msg.Content == tt.content {
						found = true
						break
					}
				}
				assert.True(t, found, "message should be in the list")
			}
		})
	}
}

func TestManager_GetMessages(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create session.
	session, err := manager.CreateSession(ctx, "test", "gpt-4", "openai", nil)
	require.NoError(t, err)

	// Add messages.
	for i := 0; i < 3; i++ {
		err := manager.AddMessage(ctx, session.ID, "user", string(rune('a'+i)))
		require.NoError(t, err)
	}

	tests := []struct {
		name      string
		sessionID string
		limit     int
		wantCount int
		wantErr   bool
	}{
		{
			name:      "retrieves all messages",
			sessionID: session.ID,
			limit:     0,
			wantCount: 3,
			wantErr:   false,
		},
		{
			name:      "respects limit",
			sessionID: session.ID,
			limit:     2,
			wantCount: 2,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := manager.GetMessages(ctx, tt.sessionID, tt.limit)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, messages, tt.wantCount)
			}
		})
	}
}

func TestManager_GetMessageCount(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create session.
	session, err := manager.CreateSession(ctx, "test", "gpt-4", "openai", nil)
	require.NoError(t, err)

	// Initially should be 0.
	count, err := manager.GetMessageCount(ctx, session.ID)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)

	// Add messages.
	for i := 0; i < 5; i++ {
		err := manager.AddMessage(ctx, session.ID, "user", "test")
		require.NoError(t, err)
	}

	// Should now be 5.
	count, err = manager.GetMessageCount(ctx, session.ID)
	assert.NoError(t, err)
	assert.Equal(t, 5, count)
}

func TestManager_AddContext(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create session.
	session, err := manager.CreateSession(ctx, "test", "gpt-4", "openai", nil)
	require.NoError(t, err)

	err = manager.AddContext(ctx, session.ID, "stack", "prod-us-east-1", "config-data")
	assert.NoError(t, err)

	// Verify context was added.
	items, err := manager.GetContext(ctx, session.ID)
	assert.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "stack", items[0].ContextType)
	assert.Equal(t, "prod-us-east-1", items[0].ContextKey)
	assert.Equal(t, "config-data", items[0].ContextValue)
}

func TestManager_GetContext(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create session.
	session, err := manager.CreateSession(ctx, "test", "gpt-4", "openai", nil)
	require.NoError(t, err)

	// Add context items.
	err = manager.AddContext(ctx, session.ID, "stack", "stack-1", "value-1")
	require.NoError(t, err)
	err = manager.AddContext(ctx, session.ID, "component", "comp-1", "value-2")
	require.NoError(t, err)

	// Retrieve context.
	items, err := manager.GetContext(ctx, session.ID)
	assert.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestManager_DeleteSession(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create session with messages and context.
	session, err := manager.CreateSession(ctx, "test", "gpt-4", "openai", nil)
	require.NoError(t, err)

	err = manager.AddMessage(ctx, session.ID, "user", "test")
	require.NoError(t, err)

	err = manager.AddContext(ctx, session.ID, "stack", "test", "value")
	require.NoError(t, err)

	// Delete session.
	err = manager.DeleteSession(ctx, session.ID)
	assert.NoError(t, err)

	// Verify session was deleted.
	_, err = manager.GetSession(ctx, session.ID)
	assert.Error(t, err)

	// Verify messages were cascade deleted.
	messages, err := manager.GetMessages(ctx, session.ID, 0)
	assert.NoError(t, err)
	assert.Len(t, messages, 0)
}

func TestManager_CleanOldSessions(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create sessions with different ages using direct storage access.
	storage := manager.storage
	oldSession := &Session{
		ID:          "old",
		Name:        "old",
		ProjectPath: manager.projectPath,
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   now.AddDate(0, 0, -40),
		UpdatedAt:   now.AddDate(0, 0, -40),
	}
	recentSession := &Session{
		ID:          "recent",
		Name:        "recent",
		ProjectPath: manager.projectPath,
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   now.AddDate(0, 0, -10),
		UpdatedAt:   now.AddDate(0, 0, -10),
	}

	err := storage.CreateSession(ctx, oldSession)
	require.NoError(t, err)
	err = storage.CreateSession(ctx, recentSession)
	require.NoError(t, err)

	tests := []struct {
		name          string
		retentionDays int
		wantCount     int
		wantErr       bool
	}{
		{
			name:          "cleans old sessions",
			retentionDays: 30,
			wantCount:     1,
			wantErr:       false,
		},
		{
			name:          "uses default for zero",
			retentionDays: 0,
			wantCount:     1, // Old session is 40 days old, default is 30, so it gets deleted
			wantErr:       false,
		},
		{
			name:          "cleans nothing with large retention",
			retentionDays: 100,
			wantCount:     0, // Both sessions are younger than 100 days
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset sessions for each test by recreating them.
			// First clean any existing sessions.
			storage.DeleteSession(ctx, oldSession.ID)
			storage.DeleteSession(ctx, recentSession.ID)

			// Recreate sessions.
			storage.CreateSession(ctx, oldSession)
			storage.CreateSession(ctx, recentSession)

			count, err := manager.CleanOldSessions(ctx, tt.retentionDays)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCount, count)
			}
		})
	}
}

func TestManager_Close(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	err := manager.Close()
	assert.NoError(t, err)
}
