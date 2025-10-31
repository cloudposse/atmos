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
			err = os.WriteFile(tmpFile, data, 0600)
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
			err = os.WriteFile(tmpFile, data, 0600)
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
