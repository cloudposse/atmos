//nolint:dupl // Test files contain similar setup code by design for isolation and clarity.
package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestManager_ExportSession(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create a test session with messages.
	session, err := manager.CreateSession(ctx, "export-test", "gpt-4", "openai", "", map[string]interface{}{"test": "value"})
	require.NoError(t, err)
	require.NotNil(t, session)

	// Add some messages.
	msg1 := &Message{
		SessionID: session.ID,
		Role:      "user",
		Content:   "Hello, how are you?",
		CreatedAt: time.Now(),
	}
	err = manager.storage.AddMessage(ctx, msg1)
	require.NoError(t, err)

	msg2 := &Message{
		SessionID: session.ID,
		Role:      "assistant",
		Content:   "I'm doing well, thank you!",
		CreatedAt: time.Now(),
	}
	err = manager.storage.AddMessage(ctx, msg2)
	require.NoError(t, err)

	tests := []struct {
		name    string
		format  string
		opts    ExportOptions
		wantErr bool
		check   func(t *testing.T, path string)
	}{
		{
			name:   "exports to JSON",
			format: "json",
			opts: ExportOptions{
				Format:          "json",
				IncludeMetadata: true,
				IncludeContext:  false,
			},
			wantErr: false,
			check: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)

				var checkpoint Checkpoint
				err = json.Unmarshal(data, &checkpoint)
				require.NoError(t, err)

				assert.Equal(t, CheckpointVersion, checkpoint.Version)
				assert.Equal(t, "export-test", checkpoint.Session.Name)
				assert.Equal(t, "gpt-4", checkpoint.Session.Model)
				assert.Equal(t, "openai", checkpoint.Session.Provider)
				assert.Equal(t, 2, len(checkpoint.Messages))
				assert.Equal(t, 2, checkpoint.Statistics.MessageCount)
				assert.Equal(t, 1, checkpoint.Statistics.UserMessages)
				assert.Equal(t, 1, checkpoint.Statistics.AssistantMessages)
			},
		},
		{
			name:   "exports to YAML",
			format: "yaml",
			opts: ExportOptions{
				Format:          "yaml",
				IncludeMetadata: true,
				IncludeContext:  false,
			},
			wantErr: false,
			check: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)

				var checkpoint Checkpoint
				err = yaml.Unmarshal(data, &checkpoint)
				require.NoError(t, err)

				assert.Equal(t, CheckpointVersion, checkpoint.Version)
				assert.Equal(t, "export-test", checkpoint.Session.Name)
				assert.Equal(t, 2, len(checkpoint.Messages))
			},
		},
		{
			name:   "exports to Markdown",
			format: "md",
			opts: ExportOptions{
				Format:          "markdown",
				IncludeMetadata: true,
				IncludeContext:  false,
			},
			wantErr: false,
			check: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)

				content := string(data)
				assert.Contains(t, content, "# Atmos AI Session: export-test")
				assert.Contains(t, content, "**Provider:** openai")
				assert.Contains(t, content, "**Model:** gpt-4")
				assert.Contains(t, content, "## Statistics")
				assert.Contains(t, content, "## Conversation")
				assert.Contains(t, content, "Hello, how are you?")
				assert.Contains(t, content, "I'm doing well, thank you!")
			},
		},
		{
			name:   "auto-detects format from .json extension",
			format: "json",
			opts: ExportOptions{
				Format:          "", // Empty - should auto-detect
				IncludeMetadata: true,
			},
			wantErr: false,
			check: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)

				var checkpoint Checkpoint
				err = json.Unmarshal(data, &checkpoint)
				require.NoError(t, err)
				assert.Equal(t, "export-test", checkpoint.Session.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := filepath.Join(t.TempDir(), "checkpoint."+tt.format)

			err := manager.ExportSession(ctx, session.ID, tmpFile, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.FileExists(t, tmpFile)
				if tt.check != nil {
					tt.check(t, tmpFile)
				}
			}
		})
	}
}

func TestManager_ExportSessionByName(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create a test session.
	session, err := manager.CreateSession(ctx, "named-export-test", "gpt-4", "openai", "", nil)
	require.NoError(t, err)

	// Add a message.
	msg := &Message{
		SessionID: session.ID,
		Role:      "user",
		Content:   "Test message",
		CreatedAt: time.Now(),
	}
	err = manager.storage.AddMessage(ctx, msg)
	require.NoError(t, err)

	tests := []struct {
		name        string
		sessionName string
		wantErr     bool
	}{
		{
			name:        "exports existing session by name",
			sessionName: "named-export-test",
			wantErr:     false,
		},
		{
			name:        "returns error for non-existent session",
			sessionName: "non-existent",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := filepath.Join(t.TempDir(), "checkpoint.json")
			opts := ExportOptions{
				Format:          "json",
				IncludeMetadata: true,
			}

			err := manager.ExportSessionByName(ctx, tt.sessionName, tmpFile, opts)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.FileExists(t, tmpFile)
			}
		})
	}
}

func TestManager_ImportSession(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name       string
		checkpoint *Checkpoint
		opts       ImportOptions
		setup      func(t *testing.T, manager *Manager)
		wantErr    bool
		check      func(t *testing.T, imported *Session)
	}{
		{
			name: "imports valid JSON checkpoint",
			checkpoint: &Checkpoint{
				Version:    CheckpointVersion,
				ExportedAt: time.Now(),
				Session: CheckpointSession{
					Name:        "imported-session",
					Provider:    "openai",
					Model:       "gpt-4",
					ProjectPath: "/test/project",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata:    map[string]interface{}{"key": "value"},
				},
				Messages: []CheckpointMessage{
					{
						Role:      "user",
						Content:   "Hello",
						CreatedAt: time.Now(),
					},
					{
						Role:      "assistant",
						Content:   "Hi there!",
						CreatedAt: time.Now(),
					},
				},
				Statistics: CheckpointStatistics{
					MessageCount:      2,
					UserMessages:      1,
					AssistantMessages: 1,
				},
			},
			opts: ImportOptions{
				Name:              "",
				OverwriteExisting: false,
			},
			wantErr: false,
			check: func(t *testing.T, imported *Session) {
				assert.NotNil(t, imported)
				assert.Equal(t, "imported-session", imported.Name)
				assert.Equal(t, "gpt-4", imported.Model)
				assert.Equal(t, "openai", imported.Provider)
				assert.Equal(t, 2, imported.MessageCount)
			},
		},
		{
			name: "imports with custom name",
			checkpoint: &Checkpoint{
				Version:    CheckpointVersion,
				ExportedAt: time.Now(),
				Session: CheckpointSession{
					Name:        "original-name",
					Provider:    "openai",
					Model:       "gpt-4",
					ProjectPath: "/test/project",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
				Messages: []CheckpointMessage{
					{
						Role:      "user",
						Content:   "Test",
						CreatedAt: time.Now(),
					},
				},
				Statistics: CheckpointStatistics{
					MessageCount: 1,
					UserMessages: 1,
				},
			},
			opts: ImportOptions{
				Name:              "custom-name",
				OverwriteExisting: false,
			},
			wantErr: false,
			check: func(t *testing.T, imported *Session) {
				assert.Equal(t, "custom-name", imported.Name)
			},
		},
		{
			name: "fails when session already exists without overwrite",
			checkpoint: &Checkpoint{
				Version:    CheckpointVersion,
				ExportedAt: time.Now(),
				Session: CheckpointSession{
					Name:        "existing-session",
					Provider:    "openai",
					Model:       "gpt-4",
					ProjectPath: "/test/project",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
				Messages: []CheckpointMessage{
					{
						Role:      "user",
						Content:   "Test",
						CreatedAt: time.Now(),
					},
				},
				Statistics: CheckpointStatistics{
					MessageCount: 1,
					UserMessages: 1,
				},
			},
			opts: ImportOptions{
				Name:              "existing-session",
				OverwriteExisting: false,
			},
			setup: func(t *testing.T, manager *Manager) {
				// Create existing session.
				_, err := manager.CreateSession(ctx, "existing-session", "gpt-3.5", "openai", "", nil)
				require.NoError(t, err)
			},
			wantErr: true,
		},
		{
			name: "overwrites existing session when flag is set",
			checkpoint: &Checkpoint{
				Version:    CheckpointVersion,
				ExportedAt: time.Now(),
				Session: CheckpointSession{
					Name:        "overwrite-session",
					Provider:    "openai",
					Model:       "gpt-4",
					ProjectPath: "/test/project",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
				Messages: []CheckpointMessage{
					{
						Role:      "user",
						Content:   "New content",
						CreatedAt: time.Now(),
					},
				},
				Statistics: CheckpointStatistics{
					MessageCount: 1,
					UserMessages: 1,
				},
			},
			opts: ImportOptions{
				Name:              "overwrite-session",
				OverwriteExisting: true,
			},
			setup: func(t *testing.T, manager *Manager) {
				// Create existing session.
				_, err := manager.CreateSession(ctx, "overwrite-session", "gpt-3.5", "openai", "", nil)
				require.NoError(t, err)
			},
			wantErr: false,
			check: func(t *testing.T, imported *Session) {
				assert.Equal(t, "gpt-4", imported.Model) // Should have new model.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t, manager)
			}

			// Write checkpoint to temporary file.
			tmpFile := filepath.Join(t.TempDir(), "checkpoint.json")
			data, err := json.Marshal(tt.checkpoint)
			require.NoError(t, err)
			err = os.WriteFile(tmpFile, data, 0o600)
			require.NoError(t, err)

			// Import session.
			imported, err := manager.ImportSession(ctx, tmpFile, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, imported)
				if tt.check != nil {
					tt.check(t, imported)
				}
			}
		})
	}
}

func TestValidateCheckpoint(t *testing.T) {
	tests := []struct {
		name       string
		checkpoint *Checkpoint
		wantErr    bool
	}{
		{
			name: "valid checkpoint",
			checkpoint: &Checkpoint{
				Version:    CheckpointVersion,
				ExportedAt: time.Now(),
				Session: CheckpointSession{
					Name:        "test",
					Provider:    "openai",
					Model:       "gpt-4",
					ProjectPath: "/test",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
				Messages: []CheckpointMessage{
					{
						Role:      "user",
						Content:   "Hello",
						CreatedAt: time.Now(),
					},
				},
				Statistics: CheckpointStatistics{
					MessageCount: 1,
					UserMessages: 1,
				},
			},
			wantErr: false,
		},
		{
			name:       "nil checkpoint",
			checkpoint: nil,
			wantErr:    true,
		},
		{
			name: "missing version",
			checkpoint: &Checkpoint{
				Version: "",
				Session: CheckpointSession{
					Name:     "test",
					Provider: "openai",
					Model:    "gpt-4",
				},
				Messages: []CheckpointMessage{
					{Role: "user", Content: "Hello", CreatedAt: time.Now()},
				},
				Statistics: CheckpointStatistics{MessageCount: 1},
			},
			wantErr: true,
		},
		{
			name: "unsupported version",
			checkpoint: &Checkpoint{
				Version: "2.0",
				Session: CheckpointSession{
					Name:     "test",
					Provider: "openai",
					Model:    "gpt-4",
				},
				Messages: []CheckpointMessage{
					{Role: "user", Content: "Hello", CreatedAt: time.Now()},
				},
				Statistics: CheckpointStatistics{MessageCount: 1},
			},
			wantErr: true,
		},
		{
			name: "missing session name",
			checkpoint: &Checkpoint{
				Version: CheckpointVersion,
				Session: CheckpointSession{
					Name:     "",
					Provider: "openai",
					Model:    "gpt-4",
				},
				Messages: []CheckpointMessage{
					{Role: "user", Content: "Hello", CreatedAt: time.Now()},
				},
				Statistics: CheckpointStatistics{MessageCount: 1},
			},
			wantErr: true,
		},
		{
			name: "missing provider",
			checkpoint: &Checkpoint{
				Version: CheckpointVersion,
				Session: CheckpointSession{
					Name:     "test",
					Provider: "",
					Model:    "gpt-4",
				},
				Messages: []CheckpointMessage{
					{Role: "user", Content: "Hello", CreatedAt: time.Now()},
				},
				Statistics: CheckpointStatistics{MessageCount: 1},
			},
			wantErr: true,
		},
		{
			name: "missing model",
			checkpoint: &Checkpoint{
				Version: CheckpointVersion,
				Session: CheckpointSession{
					Name:     "test",
					Provider: "openai",
					Model:    "",
				},
				Messages: []CheckpointMessage{
					{Role: "user", Content: "Hello", CreatedAt: time.Now()},
				},
				Statistics: CheckpointStatistics{MessageCount: 1},
			},
			wantErr: true,
		},
		{
			name: "no messages",
			checkpoint: &Checkpoint{
				Version: CheckpointVersion,
				Session: CheckpointSession{
					Name:     "test",
					Provider: "openai",
					Model:    "gpt-4",
				},
				Messages:   []CheckpointMessage{},
				Statistics: CheckpointStatistics{MessageCount: 0},
			},
			wantErr: true,
		},
		{
			name: "invalid message role",
			checkpoint: &Checkpoint{
				Version: CheckpointVersion,
				Session: CheckpointSession{
					Name:     "test",
					Provider: "openai",
					Model:    "gpt-4",
				},
				Messages: []CheckpointMessage{
					{Role: "invalid", Content: "Hello", CreatedAt: time.Now()},
				},
				Statistics: CheckpointStatistics{MessageCount: 1},
			},
			wantErr: true,
		},
		{
			name: "statistics mismatch",
			checkpoint: &Checkpoint{
				Version: CheckpointVersion,
				Session: CheckpointSession{
					Name:     "test",
					Provider: "openai",
					Model:    "gpt-4",
				},
				Messages: []CheckpointMessage{
					{Role: "user", Content: "Hello", CreatedAt: time.Now()},
					{Role: "assistant", Content: "Hi", CreatedAt: time.Now()},
				},
				Statistics: CheckpointStatistics{
					MessageCount: 1, // Wrong count.
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCheckpoint(tt.checkpoint)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDetectFormatFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "detects json",
			path: "/path/to/file.json",
			want: "json",
		},
		{
			name: "detects yaml",
			path: "/path/to/file.yaml",
			want: "yaml",
		},
		{
			name: "detects yml",
			path: "/path/to/file.yml",
			want: "yaml",
		},
		{
			name: "detects markdown",
			path: "/path/to/file.md",
			want: "markdown",
		},
		{
			name: "detects markdown from .markdown",
			path: "/path/to/file.markdown",
			want: "markdown",
		},
		{
			name: "defaults to json for unknown extension",
			path: "/path/to/file.txt",
			want: "json",
		},
		{
			name: "defaults to json for no extension",
			path: "/path/to/file",
			want: "json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectFormatFromPath(tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateCheckpointFile(t *testing.T) {
	tests := []struct {
		name       string
		checkpoint *Checkpoint
		wantErr    bool
	}{
		{
			name: "validates valid checkpoint file",
			checkpoint: &Checkpoint{
				Version:    CheckpointVersion,
				ExportedAt: time.Now(),
				Session: CheckpointSession{
					Name:        "test",
					Provider:    "openai",
					Model:       "gpt-4",
					ProjectPath: "/test",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
				Messages: []CheckpointMessage{
					{Role: "user", Content: "Hello", CreatedAt: time.Now()},
				},
				Statistics: CheckpointStatistics{
					MessageCount: 1,
					UserMessages: 1,
				},
			},
			wantErr: false,
		},
		{
			name: "rejects invalid checkpoint file",
			checkpoint: &Checkpoint{
				Version: CheckpointVersion,
				Session: CheckpointSession{
					Name:     "test",
					Provider: "openai",
					Model:    "gpt-4",
				},
				Messages:   []CheckpointMessage{}, // No messages - invalid.
				Statistics: CheckpointStatistics{MessageCount: 0},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write checkpoint to file.
			tmpFile := filepath.Join(t.TempDir(), "checkpoint.json")
			data, err := json.Marshal(tt.checkpoint)
			require.NoError(t, err)
			err = os.WriteFile(tmpFile, data, 0o600)
			require.NoError(t, err)

			// Validate file.
			err = ValidateCheckpointFile(tmpFile)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestManager_ExportImportRoundTrip(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create a session with messages and metadata.
	session, err := manager.CreateSession(ctx, "roundtrip-test", "gpt-4", "openai", "", map[string]interface{}{
		"project": "test-project",
		"version": "1.0",
	})
	require.NoError(t, err)

	// Add messages.
	messages := []*Message{
		{
			SessionID: session.ID,
			Role:      "user",
			Content:   "What is the capital of France?",
			CreatedAt: time.Now(),
		},
		{
			SessionID: session.ID,
			Role:      "assistant",
			Content:   "The capital of France is Paris.",
			CreatedAt: time.Now(),
		},
		{
			SessionID: session.ID,
			Role:      "user",
			Content:   "Thanks!",
			CreatedAt: time.Now(),
		},
	}

	for _, msg := range messages {
		err = manager.storage.AddMessage(ctx, msg)
		require.NoError(t, err)
	}

	// Export session.
	tmpFile := filepath.Join(t.TempDir(), "roundtrip.json")
	opts := ExportOptions{
		Format:          "json",
		IncludeMetadata: true,
		IncludeContext:  false,
	}
	err = manager.ExportSession(ctx, session.ID, tmpFile, opts)
	require.NoError(t, err)

	// Import session with different name.
	importOpts := ImportOptions{
		Name:              "roundtrip-imported",
		OverwriteExisting: false,
	}
	imported, err := manager.ImportSession(ctx, tmpFile, importOpts)
	require.NoError(t, err)
	require.NotNil(t, imported)

	// Verify imported session.
	assert.Equal(t, "roundtrip-imported", imported.Name)
	assert.Equal(t, "gpt-4", imported.Model)
	assert.Equal(t, "openai", imported.Provider)
	assert.Equal(t, 3, imported.MessageCount)

	// Verify messages were imported.
	importedMessages, err := manager.storage.GetMessages(ctx, imported.ID, 0)
	require.NoError(t, err)
	require.Equal(t, 3, len(importedMessages))

	// Check message content.
	assert.Equal(t, "user", importedMessages[0].Role)
	assert.Equal(t, "What is the capital of France?", importedMessages[0].Content)
	assert.Equal(t, "assistant", importedMessages[1].Role)
	assert.Equal(t, "The capital of France is Paris.", importedMessages[1].Content)
}

func TestManager_ExportSession_ErrorHandling(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name       string
		sessionID  string
		outputPath string
		opts       ExportOptions
		setup      func() string
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "returns error for non-existent session",
			sessionID:  "non-existent-id",
			outputPath: filepath.Join(t.TempDir(), "test.json"),
			opts:       ExportOptions{Format: "json"},
			wantErr:    true,
		},
		{
			name:       "returns error for unsupported format",
			sessionID:  "test-id",
			outputPath: filepath.Join(t.TempDir(), "test.txt"),
			opts:       ExportOptions{Format: "unsupported"},
			setup: func() string {
				session, err := manager.CreateSession(ctx, "test", "gpt-4", "openai", "", nil)
				require.NoError(t, err)
				err = manager.AddMessage(ctx, session.ID, "user", "test")
				require.NoError(t, err)
				return session.ID
			},
			wantErr: true,
			errMsg:  "unsupported export format",
		},
		{
			name:       "returns error for invalid output path",
			sessionID:  "test-id",
			outputPath: filepath.Join(t.TempDir(), "nonexistent", "subdir", "test.json"),
			opts:       ExportOptions{Format: "json"},
			setup: func() string {
				session, err := manager.CreateSession(ctx, "test", "gpt-4", "openai", "", nil)
				require.NoError(t, err)
				err = manager.AddMessage(ctx, session.ID, "user", "test")
				require.NoError(t, err)
				return session.ID
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionID := tt.sessionID
			if tt.setup != nil {
				sessionID = tt.setup()
			}

			err := manager.ExportSession(ctx, sessionID, tt.outputPath, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestManager_BuildCheckpoint(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	// Create a session with metadata.
	session, err := manager.CreateSession(ctx, "test-checkpoint", "gpt-4", "openai", "test-skill", map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	})
	require.NoError(t, err)

	// Create messages with different roles.
	messages := []*Message{
		{SessionID: session.ID, Role: "user", Content: "Question 1", CreatedAt: time.Now()},
		{SessionID: session.ID, Role: "assistant", Content: "Answer 1", CreatedAt: time.Now()},
		{SessionID: session.ID, Role: "user", Content: "Question 2", CreatedAt: time.Now()},
		{SessionID: session.ID, Role: "assistant", Content: "Answer 2", CreatedAt: time.Now()},
		{SessionID: session.ID, Role: "system", Content: "System message", CreatedAt: time.Now()},
	}

	tests := []struct {
		name     string
		opts     ExportOptions
		messages []*Message
		check    func(t *testing.T, checkpoint *Checkpoint)
	}{
		{
			name: "builds checkpoint with metadata",
			opts: ExportOptions{
				IncludeMetadata: true,
				IncludeContext:  false,
			},
			messages: messages,
			check: func(t *testing.T, checkpoint *Checkpoint) {
				assert.NotNil(t, checkpoint)
				assert.Equal(t, CheckpointVersion, checkpoint.Version)
				assert.Equal(t, "test-checkpoint", checkpoint.Session.Name)
				assert.Equal(t, "gpt-4", checkpoint.Session.Model)
				assert.Equal(t, "openai", checkpoint.Session.Provider)
				assert.Equal(t, "test-skill", checkpoint.Session.Skill)
				assert.Equal(t, 5, checkpoint.Statistics.MessageCount)
				assert.Equal(t, 2, checkpoint.Statistics.UserMessages)
				assert.Equal(t, 2, checkpoint.Statistics.AssistantMessages)
				assert.NotNil(t, checkpoint.Session.Metadata)
				assert.Nil(t, checkpoint.Context)
			},
		},
		{
			name: "builds checkpoint without metadata",
			opts: ExportOptions{
				IncludeMetadata: false,
				IncludeContext:  false,
			},
			messages: messages,
			check: func(t *testing.T, checkpoint *Checkpoint) {
				assert.NotNil(t, checkpoint)
				assert.Nil(t, checkpoint.Session.Metadata)
				assert.Nil(t, checkpoint.Context)
			},
		},
		{
			name: "builds checkpoint with context",
			opts: ExportOptions{
				IncludeMetadata: true,
				IncludeContext:  true,
			},
			messages: messages,
			check: func(t *testing.T, checkpoint *Checkpoint) {
				assert.NotNil(t, checkpoint)
				assert.NotNil(t, checkpoint.Context)
				// Working directory should be set by extractContext.
				assert.NotEmpty(t, checkpoint.Context.WorkingDirectory)
			},
		},
		{
			name: "builds checkpoint with no messages",
			opts: ExportOptions{
				IncludeMetadata: false,
				IncludeContext:  false,
			},
			messages: []*Message{},
			check: func(t *testing.T, checkpoint *Checkpoint) {
				assert.NotNil(t, checkpoint)
				assert.Equal(t, 0, len(checkpoint.Messages))
				assert.Equal(t, 0, checkpoint.Statistics.MessageCount)
				assert.Equal(t, 0, checkpoint.Statistics.UserMessages)
				assert.Equal(t, 0, checkpoint.Statistics.AssistantMessages)
			},
		},
		{
			name: "counts message types correctly",
			opts: ExportOptions{
				IncludeMetadata: false,
				IncludeContext:  false,
			},
			messages: []*Message{
				{SessionID: session.ID, Role: "user", Content: "Q1", CreatedAt: time.Now()},
				{SessionID: session.ID, Role: "user", Content: "Q2", CreatedAt: time.Now()},
				{SessionID: session.ID, Role: "user", Content: "Q3", CreatedAt: time.Now()},
				{SessionID: session.ID, Role: "assistant", Content: "A1", CreatedAt: time.Now()},
			},
			check: func(t *testing.T, checkpoint *Checkpoint) {
				assert.Equal(t, 4, checkpoint.Statistics.MessageCount)
				assert.Equal(t, 3, checkpoint.Statistics.UserMessages)
				assert.Equal(t, 1, checkpoint.Statistics.AssistantMessages)
			},
		},
		{
			name: "includes archived messages",
			opts: ExportOptions{
				IncludeMetadata: false,
				IncludeContext:  false,
			},
			messages: []*Message{
				{SessionID: session.ID, Role: "user", Content: "Q1", CreatedAt: time.Now(), Archived: true},
				{SessionID: session.ID, Role: "assistant", Content: "A1", CreatedAt: time.Now(), Archived: false},
			},
			check: func(t *testing.T, checkpoint *Checkpoint) {
				assert.Equal(t, 2, len(checkpoint.Messages))
				assert.True(t, checkpoint.Messages[0].Archived)
				assert.False(t, checkpoint.Messages[1].Archived)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkpoint := manager.buildCheckpoint(session, tt.messages, tt.opts)
			require.NotNil(t, checkpoint)
			tt.check(t, checkpoint)
		})
	}
}

func TestExtractContext(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	context := manager.extractContext()

	assert.NotNil(t, context)
	// Working directory should be set.
	assert.NotEmpty(t, context.WorkingDirectory)
	// Other fields should be empty for now (placeholder implementation).
	assert.Empty(t, context.ProjectMemory)
	assert.Empty(t, context.FilesAccessed)
}

func TestExportJSON_ErrorHandling(t *testing.T) {
	checkpoint := &Checkpoint{
		Version:    CheckpointVersion,
		ExportedAt: time.Now(),
		Session: CheckpointSession{
			Name:     "test",
			Provider: "openai",
			Model:    "gpt-4",
		},
		Messages:   []CheckpointMessage{{Role: "user", Content: "test", CreatedAt: time.Now()}},
		Statistics: CheckpointStatistics{MessageCount: 1},
	}

	tests := []struct {
		name       string
		outputPath string
		wantErr    bool
	}{
		{
			name:       "fails with invalid directory",
			outputPath: filepath.Join(t.TempDir(), "nonexistent", "subdir", "test.json"),
			wantErr:    true,
		},
		{
			name:       "succeeds with valid path",
			outputPath: filepath.Join(t.TempDir(), "valid.json"),
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := exportJSON(checkpoint, tt.outputPath)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.FileExists(t, tt.outputPath)
			}
		})
	}
}

func TestExportYAML_ErrorHandling(t *testing.T) {
	checkpoint := &Checkpoint{
		Version:    CheckpointVersion,
		ExportedAt: time.Now(),
		Session: CheckpointSession{
			Name:     "test",
			Provider: "openai",
			Model:    "gpt-4",
		},
		Messages:   []CheckpointMessage{{Role: "user", Content: "test", CreatedAt: time.Now()}},
		Statistics: CheckpointStatistics{MessageCount: 1},
	}

	tests := []struct {
		name       string
		outputPath string
		wantErr    bool
	}{
		{
			name:       "fails with invalid directory",
			outputPath: filepath.Join(t.TempDir(), "nonexistent", "subdir", "test.yaml"),
			wantErr:    true,
		},
		{
			name:       "succeeds with valid path",
			outputPath: filepath.Join(t.TempDir(), "valid.yaml"),
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := exportYAML(checkpoint, tt.outputPath)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.FileExists(t, tt.outputPath)
			}
		})
	}
}

func TestExportMarkdown_ErrorHandling(t *testing.T) {
	checkpoint := &Checkpoint{
		Version:    CheckpointVersion,
		ExportedAt: time.Now(),
		Session: CheckpointSession{
			Name:     "test",
			Provider: "openai",
			Model:    "gpt-4",
		},
		Messages:   []CheckpointMessage{{Role: "user", Content: "test", CreatedAt: time.Now()}},
		Statistics: CheckpointStatistics{MessageCount: 1},
	}

	tests := []struct {
		name       string
		outputPath string
		wantErr    bool
	}{
		{
			name:       "fails with invalid directory",
			outputPath: filepath.Join(t.TempDir(), "nonexistent", "subdir", "test.md"),
			wantErr:    true,
		},
		{
			name:       "succeeds with valid path",
			outputPath: filepath.Join(t.TempDir(), "valid.md"),
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := exportMarkdown(checkpoint, tt.outputPath)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.FileExists(t, tt.outputPath)
			}
		})
	}
}

func TestExportMarkdown_Content(t *testing.T) {
	tests := []struct {
		name       string
		checkpoint *Checkpoint
		check      func(t *testing.T, content string)
	}{
		{
			name: "includes all session info",
			checkpoint: &Checkpoint{
				Version:    CheckpointVersion,
				ExportedAt: time.Now(),
				ExportedBy: "test-user",
				Session: CheckpointSession{
					Name:        "test-session",
					Provider:    "openai",
					Model:       "gpt-4",
					Skill:       "general",
					ProjectPath: "/test/project",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
				Messages: []CheckpointMessage{
					{Role: "user", Content: "Hello", CreatedAt: time.Now()},
					{Role: "assistant", Content: "Hi", CreatedAt: time.Now()},
				},
				Statistics: CheckpointStatistics{
					MessageCount:      2,
					UserMessages:      1,
					AssistantMessages: 1,
				},
			},
			check: func(t *testing.T, content string) {
				assert.Contains(t, content, "# Atmos AI Session: test-session")
				assert.Contains(t, content, "**Provider:** openai")
				assert.Contains(t, content, "**Model:** gpt-4")
				assert.Contains(t, content, "**Skill:** general")
				assert.Contains(t, content, "**Exported By:** test-user")
				assert.Contains(t, content, "## Statistics")
				assert.Contains(t, content, "- Total Messages: 2")
				assert.Contains(t, content, "- User Messages: 1")
				assert.Contains(t, content, "- Assistant Messages: 1")
			},
		},
		{
			name: "includes optional statistics",
			checkpoint: &Checkpoint{
				Version:    CheckpointVersion,
				ExportedAt: time.Now(),
				Session: CheckpointSession{
					Name:     "test",
					Provider: "openai",
					Model:    "gpt-4",
				},
				Messages: []CheckpointMessage{
					{Role: "user", Content: "Test", CreatedAt: time.Now()},
				},
				Statistics: CheckpointStatistics{
					MessageCount: 1,
					TotalTokens:  5000,
					ToolCalls:    10,
				},
			},
			check: func(t *testing.T, content string) {
				assert.Contains(t, content, "- Total Tokens: 5000")
				assert.Contains(t, content, "- Tool Calls: 10")
			},
		},
		{
			name: "includes context when present",
			checkpoint: &Checkpoint{
				Version:    CheckpointVersion,
				ExportedAt: time.Now(),
				Session: CheckpointSession{
					Name:     "test",
					Provider: "openai",
					Model:    "gpt-4",
				},
				Messages: []CheckpointMessage{
					{Role: "user", Content: "Test", CreatedAt: time.Now()},
				},
				Context: &CheckpointContext{
					WorkingDirectory: "/test/dir",
					ProjectMemory:    "This is project memory",
					FilesAccessed:    []string{"file1.go", "file2.go", "file3.go"},
				},
				Statistics: CheckpointStatistics{MessageCount: 1},
			},
			check: func(t *testing.T, content string) {
				assert.Contains(t, content, "## Context")
				assert.Contains(t, content, "**Working Directory:** `/test/dir`")
				assert.Contains(t, content, "### Project Memory")
				assert.Contains(t, content, "This is project memory")
				assert.Contains(t, content, "### Files Accessed")
				assert.Contains(t, content, "- `file1.go`")
				assert.Contains(t, content, "- `file2.go`")
				assert.Contains(t, content, "- `file3.go`")
			},
		},
		{
			name: "marks archived messages",
			checkpoint: &Checkpoint{
				Version:    CheckpointVersion,
				ExportedAt: time.Now(),
				Session: CheckpointSession{
					Name:     "test",
					Provider: "openai",
					Model:    "gpt-4",
				},
				Messages: []CheckpointMessage{
					{Role: "user", Content: "Old message", CreatedAt: time.Now(), Archived: true},
					{Role: "assistant", Content: "New message", CreatedAt: time.Now(), Archived: false},
				},
				Statistics: CheckpointStatistics{MessageCount: 2},
			},
			check: func(t *testing.T, content string) {
				assert.Contains(t, content, "USER (COMPACTED)")
				assert.Contains(t, content, "Old message")
				assert.Contains(t, content, "ASSISTANT")
				assert.Contains(t, content, "New message")
			},
		},
		{
			name: "handles empty optional fields gracefully",
			checkpoint: &Checkpoint{
				Version:    CheckpointVersion,
				ExportedAt: time.Now(),
				ExportedBy: "",
				Session: CheckpointSession{
					Name:     "test",
					Provider: "openai",
					Model:    "gpt-4",
					Skill:    "",
				},
				Messages: []CheckpointMessage{
					{Role: "user", Content: "Test", CreatedAt: time.Now()},
				},
				Context:    nil,
				Statistics: CheckpointStatistics{MessageCount: 1},
			},
			check: func(t *testing.T, content string) {
				// Should not contain empty optional fields.
				assert.NotContains(t, content, "**Exported By:**")
				assert.NotContains(t, content, "**Skill:**")
				assert.NotContains(t, content, "## Context")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := filepath.Join(t.TempDir(), "test.md")
			err := exportMarkdown(tt.checkpoint, tmpFile)
			require.NoError(t, err)

			data, err := os.ReadFile(tmpFile)
			require.NoError(t, err)

			content := string(data)
			tt.check(t, content)
		})
	}
}
