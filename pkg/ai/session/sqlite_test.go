package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// setupTestStorage creates a temporary database for testing.
func setupTestStorage(t *testing.T) (*SQLiteStorage, string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	require.NoError(t, err)
	require.NotNil(t, storage)

	cleanup := func() {
		storage.Close()
		os.RemoveAll(tmpDir)
	}

	return storage, dbPath, cleanup
}

func TestNewSQLiteStorage(t *testing.T) {
	tests := []struct {
		name      string
		setupPath func(t *testing.T) string
		wantErr   bool
	}{
		{
			name: "creates new database successfully",
			setupPath: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "test.db")
			},
			wantErr: false,
		},
		{
			name: "creates database with nested directory",
			setupPath: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "subdir", "nested", "test.db")
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := tt.setupPath(t)

			storage, err := NewSQLiteStorage(dbPath)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, storage)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, storage)
				assert.FileExists(t, dbPath)
				storage.Close()
			}
		})
	}
}

func TestSQLiteStorage_CreateSession(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	session := &Session{
		ID:          "test-id",
		Name:        "test-session",
		ProjectPath: "/test/path",
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Metadata: map[string]interface{}{
			"key": "value",
		},
	}

	err := storage.CreateSession(ctx, session)
	assert.NoError(t, err)

	// Verify session was created.
	retrieved, err := storage.GetSession(ctx, session.ID)
	assert.NoError(t, err)
	assert.Equal(t, session.ID, retrieved.ID)
	assert.Equal(t, session.Name, retrieved.Name)
	assert.Equal(t, session.ProjectPath, retrieved.ProjectPath)
	assert.Equal(t, session.Model, retrieved.Model)
	assert.Equal(t, session.Provider, retrieved.Provider)
	assert.Equal(t, "value", retrieved.Metadata["key"])
}

func TestSQLiteStorage_GetSession(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name    string
		setup   func() string
		wantErr bool
		errIs   error
	}{
		{
			name: "retrieves existing session",
			setup: func() string {
				session := &Session{
					ID:          "existing-id",
					Name:        "existing",
					ProjectPath: "/test",
					Model:       "gpt-4",
					Provider:    "openai",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				storage.CreateSession(ctx, session)
				return session.ID
			},
			wantErr: false,
		},
		{
			name: "returns error for non-existent session",
			setup: func() string {
				return "non-existent-id"
			},
			wantErr: true,
			errIs:   errUtils.ErrAISessionNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := tt.setup()

			session, err := storage.GetSession(ctx, id)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
				assert.Nil(t, session)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, session)
				assert.Equal(t, id, session.ID)
			}
		})
	}
}

func TestSQLiteStorage_GetSessionByName(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	projectPath := "/test/project"

	// Create multiple sessions with same name in same project.
	session1 := &Session{
		ID:          "id-1",
		Name:        "test-session",
		ProjectPath: projectPath,
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	session2 := &Session{
		ID:          "id-2",
		Name:        "test-session",
		ProjectPath: projectPath,
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now().Add(time.Minute),
	}

	err := storage.CreateSession(ctx, session1)
	require.NoError(t, err)
	err = storage.CreateSession(ctx, session2)
	require.NoError(t, err)

	// Should retrieve most recently updated session.
	retrieved, err := storage.GetSessionByName(ctx, projectPath, "test-session")
	assert.NoError(t, err)
	assert.Equal(t, "id-2", retrieved.ID)

	// Non-existent session should return error.
	_, err = storage.GetSessionByName(ctx, projectPath, "non-existent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAISessionNotFound)
}

func TestSQLiteStorage_ListSessions(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	projectPath := "/test/project"

	// Create multiple sessions.
	for i := 0; i < 5; i++ {
		session := &Session{
			ID:          string(rune('a' + i)),
			Name:        string(rune('a' + i)),
			ProjectPath: projectPath,
			Model:       "gpt-4",
			Provider:    "openai",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now().Add(time.Duration(i) * time.Minute),
		}
		err := storage.CreateSession(ctx, session)
		require.NoError(t, err)
	}

	// Create session in different project.
	otherSession := &Session{
		ID:          "other",
		Name:        "other",
		ProjectPath: "/other/project",
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, otherSession)
	require.NoError(t, err)

	tests := []struct {
		name        string
		projectPath string
		limit       int
		wantCount   int
	}{
		{
			name:        "lists all sessions for project",
			projectPath: projectPath,
			limit:       10,
			wantCount:   5,
		},
		{
			name:        "respects limit",
			projectPath: projectPath,
			limit:       3,
			wantCount:   3,
		},
		{
			name:        "filters by project path",
			projectPath: "/other/project",
			limit:       10,
			wantCount:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions, err := storage.ListSessions(ctx, tt.projectPath, tt.limit)
			assert.NoError(t, err)
			assert.Len(t, sessions, tt.wantCount)

			// Verify sessions are sorted by updated_at DESC.
			if len(sessions) > 1 {
				for i := 0; i < len(sessions)-1; i++ {
					assert.True(t, sessions[i].UpdatedAt.After(sessions[i+1].UpdatedAt) || sessions[i].UpdatedAt.Equal(sessions[i+1].UpdatedAt))
				}
			}
		})
	}
}

func TestSQLiteStorage_UpdateSession(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create initial session.
	session := &Session{
		ID:          "test-id",
		Name:        "original-name",
		ProjectPath: "/test",
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Update session.
	session.Name = "updated-name"
	session.Model = "gpt-4-turbo"
	session.UpdatedAt = time.Now().Add(time.Hour)

	err = storage.UpdateSession(ctx, session)
	assert.NoError(t, err)

	// Verify update.
	retrieved, err := storage.GetSession(ctx, session.ID)
	assert.NoError(t, err)
	assert.Equal(t, "updated-name", retrieved.Name)
	assert.Equal(t, "gpt-4-turbo", retrieved.Model)

	// Update non-existent session should error.
	nonExistent := &Session{
		ID:          "non-existent",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err = storage.UpdateSession(ctx, nonExistent)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAISessionNotFound)
}

func TestSQLiteStorage_DeleteSession(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create session.
	session := &Session{
		ID:          "test-id",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Delete session.
	err = storage.DeleteSession(ctx, session.ID)
	assert.NoError(t, err)

	// Verify deletion.
	_, err = storage.GetSession(ctx, session.ID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAISessionNotFound)

	// Delete non-existent session should error.
	err = storage.DeleteSession(ctx, "non-existent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAISessionNotFound)
}

func TestSQLiteStorage_DeleteOldSessions(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create sessions with different ages.
	sessions := []*Session{
		{
			ID:          "old-1",
			Name:        "old-1",
			ProjectPath: "/test",
			Model:       "gpt-4",
			Provider:    "openai",
			CreatedAt:   now.AddDate(0, 0, -40),
			UpdatedAt:   now.AddDate(0, 0, -40),
		},
		{
			ID:          "old-2",
			Name:        "old-2",
			ProjectPath: "/test",
			Model:       "gpt-4",
			Provider:    "openai",
			CreatedAt:   now.AddDate(0, 0, -35),
			UpdatedAt:   now.AddDate(0, 0, -35),
		},
		{
			ID:          "recent",
			Name:        "recent",
			ProjectPath: "/test",
			Model:       "gpt-4",
			Provider:    "openai",
			CreatedAt:   now.AddDate(0, 0, -10),
			UpdatedAt:   now.AddDate(0, 0, -10),
		},
	}

	for _, session := range sessions {
		err := storage.CreateSession(ctx, session)
		require.NoError(t, err)
	}

	// Delete sessions older than 30 days.
	cutoff := now.AddDate(0, 0, -30)
	count, err := storage.DeleteOldSessions(ctx, cutoff)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)

	// Verify old sessions were deleted.
	_, err = storage.GetSession(ctx, "old-1")
	assert.Error(t, err)
	_, err = storage.GetSession(ctx, "old-2")
	assert.Error(t, err)

	// Verify recent session still exists.
	_, err = storage.GetSession(ctx, "recent")
	assert.NoError(t, err)
}

func TestSQLiteStorage_AddMessage(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create session.
	session := &Session{
		ID:          "test-id",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Add message.
	message := &Message{
		SessionID: session.ID,
		Role:      "user",
		Content:   "Hello, AI!",
		CreatedAt: time.Now(),
	}

	err = storage.AddMessage(ctx, message)
	assert.NoError(t, err)
	assert.NotZero(t, message.ID)

	// Verify message was added.
	messages, err := storage.GetMessages(ctx, session.ID, 0)
	assert.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Equal(t, "user", messages[0].Role)
	assert.Equal(t, "Hello, AI!", messages[0].Content)
}

func TestSQLiteStorage_GetMessages(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create session.
	session := &Session{
		ID:          "test-id",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Add multiple messages.
	roles := []string{"user", "assistant", "user", "assistant"}
	for i, role := range roles {
		message := &Message{
			SessionID: session.ID,
			Role:      role,
			Content:   string(rune('a' + i)),
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		err := storage.AddMessage(ctx, message)
		require.NoError(t, err)
	}

	tests := []struct {
		name      string
		limit     int
		wantCount int
	}{
		{
			name:      "retrieves all messages",
			limit:     0,
			wantCount: 4,
		},
		{
			name:      "respects limit",
			limit:     2,
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := storage.GetMessages(ctx, session.ID, tt.limit)
			assert.NoError(t, err)
			assert.Len(t, messages, tt.wantCount)

			// Verify messages are sorted by created_at ASC.
			if len(messages) > 1 {
				for i := 0; i < len(messages)-1; i++ {
					assert.True(t, messages[i].CreatedAt.Before(messages[i+1].CreatedAt) || messages[i].CreatedAt.Equal(messages[i+1].CreatedAt))
				}
			}
		})
	}
}

func TestSQLiteStorage_GetMessageCount(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create session.
	session := &Session{
		ID:          "test-id",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Initially should be 0.
	count, err := storage.GetMessageCount(ctx, session.ID)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)

	// Add messages.
	for i := 0; i < 3; i++ {
		message := &Message{
			SessionID: session.ID,
			Role:      "user",
			Content:   "test",
			CreatedAt: time.Now(),
		}
		err := storage.AddMessage(ctx, message)
		require.NoError(t, err)
	}

	// Should now be 3.
	count, err = storage.GetMessageCount(ctx, session.ID)
	assert.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestSQLiteStorage_AddContext(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create session.
	session := &Session{
		ID:          "test-id",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Add context item.
	item := &ContextItem{
		SessionID:    session.ID,
		ContextType:  "stack",
		ContextKey:   "prod-us-east-1",
		ContextValue: "some-value",
	}

	err = storage.AddContext(ctx, item)
	assert.NoError(t, err)

	// Verify context was added.
	items, err := storage.GetContext(ctx, session.ID)
	assert.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "stack", items[0].ContextType)
	assert.Equal(t, "prod-us-east-1", items[0].ContextKey)
}

func TestSQLiteStorage_GetContext(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create session.
	session := &Session{
		ID:          "test-id",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Add multiple context items.
	items := []*ContextItem{
		{
			SessionID:    session.ID,
			ContextType:  "stack",
			ContextKey:   "stack-1",
			ContextValue: "value-1",
		},
		{
			SessionID:    session.ID,
			ContextType:  "component",
			ContextKey:   "component-1",
			ContextValue: "value-2",
		},
	}

	for _, item := range items {
		err := storage.AddContext(ctx, item)
		require.NoError(t, err)
	}

	// Retrieve all context items.
	retrieved, err := storage.GetContext(ctx, session.ID)
	assert.NoError(t, err)
	assert.Len(t, retrieved, 2)
}

func TestSQLiteStorage_DeleteContext(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create session.
	session := &Session{
		ID:          "test-id",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Add context item.
	item := &ContextItem{
		SessionID:    session.ID,
		ContextType:  "stack",
		ContextKey:   "test-key",
		ContextValue: "test-value",
	}
	err = storage.AddContext(ctx, item)
	require.NoError(t, err)

	// Verify context exists.
	items, err := storage.GetContext(ctx, session.ID)
	assert.NoError(t, err)
	assert.Len(t, items, 1)

	// Delete context.
	err = storage.DeleteContext(ctx, session.ID)
	assert.NoError(t, err)

	// Verify context was deleted.
	items, err = storage.GetContext(ctx, session.ID)
	assert.NoError(t, err)
	assert.Len(t, items, 0)
}

func TestSQLiteStorage_CascadeDelete(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create session.
	session := &Session{
		ID:          "test-id",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "gpt-4",
		Provider:    "openai",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Add message.
	message := &Message{
		SessionID: session.ID,
		Role:      "user",
		Content:   "test",
		CreatedAt: time.Now(),
	}
	err = storage.AddMessage(ctx, message)
	require.NoError(t, err)

	// Add context.
	item := &ContextItem{
		SessionID:    session.ID,
		ContextType:  "stack",
		ContextKey:   "test",
		ContextValue: "value",
	}
	err = storage.AddContext(ctx, item)
	require.NoError(t, err)

	// Delete session.
	err = storage.DeleteSession(ctx, session.ID)
	assert.NoError(t, err)

	// Verify messages were cascade deleted.
	messages, err := storage.GetMessages(ctx, session.ID, 0)
	assert.NoError(t, err)
	assert.Len(t, messages, 0)

	// Verify context was cascade deleted.
	items, err := storage.GetContext(ctx, session.ID)
	assert.NoError(t, err)
	assert.Len(t, items, 0)
}

func TestSQLiteStorage_Close(t *testing.T) {
	storage, dbPath, _ := setupTestStorage(t)

	err := storage.Close()
	assert.NoError(t, err)

	// Verify database file still exists after close.
	assert.FileExists(t, dbPath)
}

// TestSQLiteStorage_StoreSummary tests storing message summaries.
func TestSQLiteStorage_StoreSummary(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create a test session.
	session := &Session{
		ID:          "test-session-1",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "test-model",
		Provider:    "test-provider",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Create and store summary.
	summary := &Summary{
		ID:                 "summary-1",
		SessionID:          session.ID,
		Provider:           "anthropic",
		OriginalMessageIDs: []int64{1, 2, 3, 4, 5},
		MessageRange:       "Messages 1-5",
		SummaryContent:     "Test summary content about VPC configuration",
		TokenCount:         25,
		CompactedAt:        time.Now(),
	}

	err = storage.StoreSummary(ctx, summary)
	assert.NoError(t, err, "Should store summary successfully")

	// Verify summary was stored.
	summaries, err := storage.GetSummaries(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, summaries, 1, "Should have one summary")

	stored := summaries[0]
	assert.Equal(t, summary.ID, stored.ID)
	assert.Equal(t, summary.SessionID, stored.SessionID)
	assert.Equal(t, summary.Provider, stored.Provider)
	assert.Equal(t, summary.MessageRange, stored.MessageRange)
	assert.Equal(t, summary.SummaryContent, stored.SummaryContent)
	assert.Equal(t, summary.TokenCount, stored.TokenCount)
	assert.Equal(t, len(summary.OriginalMessageIDs), len(stored.OriginalMessageIDs))
}

// TestSQLiteStorage_GetSummaries tests retrieving summaries for a session.
func TestSQLiteStorage_GetSummaries(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create test session.
	session := &Session{
		ID:          "test-session-2",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "test-model",
		Provider:    "test-provider",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Store multiple summaries.
	for i := 1; i <= 3; i++ {
		summary := &Summary{
			ID:                 "summary-" + string(rune(i)),
			SessionID:          session.ID,
			Provider:           "anthropic",
			OriginalMessageIDs: []int64{int64(i)},
			MessageRange:       "Messages " + string(rune(i)),
			SummaryContent:     "Summary " + string(rune(i)),
			TokenCount:         10,
			CompactedAt:        time.Now().Add(time.Minute * time.Duration(i)),
		}
		err := storage.StoreSummary(ctx, summary)
		require.NoError(t, err)
	}

	// Retrieve summaries.
	summaries, err := storage.GetSummaries(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, summaries, 3, "Should retrieve all 3 summaries")

	// Verify summaries are sorted by compacted_at.
	for i := 0; i < len(summaries)-1; i++ {
		assert.True(t, summaries[i].CompactedAt.Before(summaries[i+1].CompactedAt),
			"Summaries should be sorted chronologically")
	}
}

// TestSQLiteStorage_GetSummaries_NoSummaries tests retrieving from session with no summaries.
func TestSQLiteStorage_GetSummaries_NoSummaries(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	summaries, err := storage.GetSummaries(ctx, "nonexistent-session")
	require.NoError(t, err)
	assert.Empty(t, summaries, "Should return empty slice for session with no summaries")
}

// TestSQLiteStorage_ArchiveMessages tests marking messages as archived.
func TestSQLiteStorage_ArchiveMessages(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create test session.
	session := &Session{
		ID:          "test-session-3",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "test-model",
		Provider:    "test-provider",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Add messages.
	var messageIDs []int64
	for i := 1; i <= 5; i++ {
		msg := &Message{
			SessionID: session.ID,
			Role:      RoleUser,
			Content:   "Message " + string(rune(i)),
			CreatedAt: time.Now(),
		}
		err := storage.AddMessage(ctx, msg)
		require.NoError(t, err)
		messageIDs = append(messageIDs, msg.ID)
	}

	// Archive first 3 messages.
	err = storage.ArchiveMessages(ctx, messageIDs[:3])
	require.NoError(t, err)

	// Verify active messages.
	activeMessages, err := storage.GetActiveMessages(ctx, session.ID, 0)
	require.NoError(t, err)
	assert.Len(t, activeMessages, 2, "Should have 2 active messages")

	// Verify archived messages are excluded.
	for _, msg := range activeMessages {
		assert.False(t, msg.Archived, "Active messages should not be archived")
		assert.Contains(t, messageIDs[3:], msg.ID, "Only unarchived messages should be returned")
	}
}

// TestSQLiteStorage_ArchiveMessages_Empty tests archiving with empty list.
func TestSQLiteStorage_ArchiveMessages_Empty(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	err := storage.ArchiveMessages(ctx, []int64{})
	assert.NoError(t, err, "Should handle empty message ID list")
}

// TestSQLiteStorage_GetActiveMessages tests retrieving non-archived messages.
func TestSQLiteStorage_GetActiveMessages(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create test session.
	session := &Session{
		ID:          "test-session-4",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "test-model",
		Provider:    "test-provider",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Add messages.
	var messageIDs []int64
	for i := 1; i <= 10; i++ {
		msg := &Message{
			SessionID: session.ID,
			Role:      RoleUser,
			Content:   "Message " + string(rune(i)),
			CreatedAt: time.Now().Add(time.Second * time.Duration(i)),
		}
		err := storage.AddMessage(ctx, msg)
		require.NoError(t, err)
		messageIDs = append(messageIDs, msg.ID)
	}

	// Archive first 5 messages.
	err = storage.ArchiveMessages(ctx, messageIDs[:5])
	require.NoError(t, err)

	// Get active messages.
	activeMessages, err := storage.GetActiveMessages(ctx, session.ID, 0)
	require.NoError(t, err)
	assert.Len(t, activeMessages, 5, "Should have 5 active messages")

	// Verify messages are sorted chronologically.
	for i := 0; i < len(activeMessages)-1; i++ {
		assert.True(t, activeMessages[i].CreatedAt.Before(activeMessages[i+1].CreatedAt),
			"Messages should be sorted by creation time")
	}

	// Verify archived flag.
	for _, msg := range activeMessages {
		assert.False(t, msg.Archived, "Active messages should have archived=false")
	}
}

// TestSQLiteStorage_GetActiveMessages_WithLimit tests limit parameter.
func TestSQLiteStorage_GetActiveMessages_WithLimit(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create test session.
	session := &Session{
		ID:          "test-session-5",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "test-model",
		Provider:    "test-provider",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Add 20 messages.
	for i := 1; i <= 20; i++ {
		msg := &Message{
			SessionID: session.ID,
			Role:      RoleUser,
			Content:   "Message " + string(rune(i)),
			CreatedAt: time.Now(),
		}
		err := storage.AddMessage(ctx, msg)
		require.NoError(t, err)
	}

	// Get active messages with limit.
	activeMessages, err := storage.GetActiveMessages(ctx, session.ID, 5)
	require.NoError(t, err)
	assert.Len(t, activeMessages, 5, "Should respect limit parameter")
}

// TestSQLiteStorage_GetActiveMessages_NoMessages tests empty result.
func TestSQLiteStorage_GetActiveMessages_NoMessages(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	activeMessages, err := storage.GetActiveMessages(ctx, "nonexistent-session", 0)
	require.NoError(t, err)
	assert.Empty(t, activeMessages, "Should return empty slice for session with no messages")
}

// TestSQLiteStorage_Summary_CascadeDelete tests that summaries are deleted with session.
func TestSQLiteStorage_Summary_CascadeDelete(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create test session.
	session := &Session{
		ID:          "test-session-6",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "test-model",
		Provider:    "test-provider",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Store summary.
	summary := &Summary{
		ID:                 "summary-cascade",
		SessionID:          session.ID,
		Provider:           "anthropic",
		OriginalMessageIDs: []int64{1, 2, 3},
		MessageRange:       "Messages 1-3",
		SummaryContent:     "Test summary",
		TokenCount:         10,
		CompactedAt:        time.Now(),
	}
	err = storage.StoreSummary(ctx, summary)
	require.NoError(t, err)

	// Verify summary exists.
	summaries, err := storage.GetSummaries(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, summaries, 1)

	// Delete session.
	err = storage.DeleteSession(ctx, session.ID)
	require.NoError(t, err)

	// Verify summary was cascade deleted.
	summaries, err = storage.GetSummaries(ctx, session.ID)
	require.NoError(t, err)
	assert.Empty(t, summaries, "Summaries should be cascade deleted with session")
}

// TestSQLiteStorage_Migration_ArchivedColumn tests that archived column exists.
func TestSQLiteStorage_Migration_ArchivedColumn(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create test session.
	session := &Session{
		ID:          "test-session-7",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "test-model",
		Provider:    "test-provider",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Add message.
	msg := &Message{
		SessionID: session.ID,
		Role:      RoleUser,
		Content:   "Test message",
		CreatedAt: time.Now(),
	}
	err = storage.AddMessage(ctx, msg)
	require.NoError(t, err)

	// Verify message has archived field.
	messages, err := storage.GetActiveMessages(ctx, session.ID, 0)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.False(t, messages[0].Archived, "New messages should have archived=false by default")
}

// TestSQLiteStorage_ArchiveMessages_MultipleIDs tests archiving multiple messages.
func TestSQLiteStorage_ArchiveMessages_MultipleIDs(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create test session.
	session := &Session{
		ID:          "test-session-8",
		Name:        "test",
		ProjectPath: "/test",
		Model:       "test-model",
		Provider:    "test-provider",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := storage.CreateSession(ctx, session)
	require.NoError(t, err)

	// Add 100 messages.
	var messageIDs []int64
	for i := 1; i <= 100; i++ {
		msg := &Message{
			SessionID: session.ID,
			Role:      RoleUser,
			Content:   "Message " + string(rune(i)),
			CreatedAt: time.Now(),
		}
		err := storage.AddMessage(ctx, msg)
		require.NoError(t, err)
		messageIDs = append(messageIDs, msg.ID)
	}

	// Archive all messages.
	err = storage.ArchiveMessages(ctx, messageIDs)
	require.NoError(t, err)

	// Verify all messages archived.
	activeMessages, err := storage.GetActiveMessages(ctx, session.ID, 0)
	require.NoError(t, err)
	assert.Empty(t, activeMessages, "Should have no active messages after archiving all")
}
