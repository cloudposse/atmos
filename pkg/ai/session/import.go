package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

// ImportSession imports a session from a checkpoint file.
func (m *Manager) ImportSession(ctx context.Context, checkpointPath string, opts ImportOptions) (*Session, error) {
	// Load checkpoint.
	checkpoint, err := loadCheckpoint(checkpointPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	// Validate checkpoint.
	if err := validateCheckpoint(checkpoint); err != nil {
		return nil, fmt.Errorf("invalid checkpoint: %w", err)
	}

	// Determine session name.
	sessionName := opts.Name
	if sessionName == "" {
		sessionName = checkpoint.Session.Name
	}

	// Determine project path.
	projectPath := opts.ProjectPath
	if projectPath == "" {
		projectPath = m.projectPath
	}

	// Check if session with this name already exists.
	existing, err := m.storage.GetSessionByName(ctx, projectPath, sessionName)
	if err == nil && existing != nil {
		if !opts.OverwriteExisting {
			return nil, fmt.Errorf("%w: session '%s' already exists in project '%s'", errUtils.ErrAISessionAlreadyExists, sessionName, projectPath)
		}
		// Delete existing session.
		if err := m.storage.DeleteSession(ctx, existing.ID); err != nil {
			return nil, fmt.Errorf("failed to delete existing session: %w", err)
		}
	}

	// Create new session from checkpoint.
	session := &Session{
		ID:          uuid.New().String(),
		Name:        sessionName,
		ProjectPath: projectPath,
		Model:       checkpoint.Session.Model,
		Provider:    checkpoint.Session.Provider,
		Agent:       checkpoint.Session.Agent,
		CreatedAt:   time.Now(), // Use current time for import
		UpdatedAt:   time.Now(),
		Metadata:    checkpoint.Session.Metadata,
	}

	// Create session in storage.
	if err := m.storage.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Import messages.
	for _, checkpointMsg := range checkpoint.Messages {
		msg := &Message{
			SessionID: session.ID,
			Role:      checkpointMsg.Role,
			Content:   checkpointMsg.Content,
			CreatedAt: checkpointMsg.CreatedAt,
			Archived:  checkpointMsg.Archived,
		}

		if err := m.storage.AddMessage(ctx, msg); err != nil {
			// Rollback: delete session if message import fails.
			_ = m.storage.DeleteSession(ctx, session.ID)
			return nil, fmt.Errorf("failed to import message: %w", err)
		}
	}

	// Update message count.
	session.MessageCount = len(checkpoint.Messages)

	return session, nil
}

// loadCheckpoint loads a checkpoint from a file.
// Automatically detects format based on file extension.
func loadCheckpoint(path string) (*Checkpoint, error) {
	// Read file.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	// Detect format.
	format := detectFormatFromPath(path)

	var checkpoint Checkpoint

	// Parse based on format.
	switch format {
	case "json":
		if err := json.Unmarshal(data, &checkpoint); err != nil {
			return nil, fmt.Errorf("failed to parse JSON checkpoint: %w", err)
		}
	case "yaml", "yml":
		if err := yaml.Unmarshal(data, &checkpoint); err != nil {
			return nil, fmt.Errorf("failed to parse YAML checkpoint: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported checkpoint format: %s (supported: json, yaml)", format)
	}

	return &checkpoint, nil
}

// validateCheckpoint validates a checkpoint before import.
func validateCheckpoint(checkpoint *Checkpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("checkpoint is nil")
	}

	// Check version compatibility.
	if checkpoint.Version == "" {
		return fmt.Errorf("checkpoint version is missing")
	}

	// Currently we only support version 1.0.
	// In the future, we can add version migration logic here.
	if checkpoint.Version != CheckpointVersion {
		return fmt.Errorf("unsupported checkpoint version: %s (expected: %s)", checkpoint.Version, CheckpointVersion)
	}

	// Validate session data.
	if checkpoint.Session.Name == "" {
		return fmt.Errorf("session name is required")
	}

	if checkpoint.Session.Provider == "" {
		return fmt.Errorf("session provider is required")
	}

	if checkpoint.Session.Model == "" {
		return fmt.Errorf("session model is required")
	}

	// Validate messages.
	if len(checkpoint.Messages) == 0 {
		return fmt.Errorf("checkpoint must contain at least one message")
	}

	for i, msg := range checkpoint.Messages {
		if msg.Role == "" {
			return fmt.Errorf("message %d: role is required", i+1)
		}

		if msg.Role != "user" && msg.Role != "assistant" && msg.Role != "system" {
			return fmt.Errorf("message %d: invalid role '%s' (expected: user, assistant, system)", i+1, msg.Role)
		}

		// Content can be empty for some system messages.
	}

	// Validate statistics match message count.
	if checkpoint.Statistics.MessageCount != len(checkpoint.Messages) {
		return fmt.Errorf("statistics message count (%d) does not match actual messages (%d)", checkpoint.Statistics.MessageCount, len(checkpoint.Messages))
	}

	return nil
}

// ValidateCheckpointFile validates a checkpoint file without importing.
// Useful for pre-import validation or CLI dry-run.
func ValidateCheckpointFile(path string) error {
	checkpoint, err := loadCheckpoint(path)
	if err != nil {
		return err
	}

	return validateCheckpoint(checkpoint)
}
