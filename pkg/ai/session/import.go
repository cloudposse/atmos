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
	// Load and validate checkpoint.
	checkpoint, err := loadAndValidateCheckpoint(checkpointPath)
	if err != nil {
		return nil, err
	}

	// Resolve import parameters.
	sessionName := resolveImportName(opts.Name, checkpoint.Session.Name)
	projectPath := resolveImportPath(opts.ProjectPath, m.projectPath)

	// Handle existing session conflicts.
	if err := m.handleExistingSession(ctx, projectPath, sessionName, opts.OverwriteExisting); err != nil {
		return nil, err
	}

	// Create and persist session.
	session := m.buildImportedSession(sessionName, projectPath, checkpoint)
	if err := m.storage.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Import messages.
	if err := m.importMessages(ctx, session.ID, checkpoint.Messages); err != nil {
		_ = m.storage.DeleteSession(ctx, session.ID)
		return nil, err
	}

	session.MessageCount = len(checkpoint.Messages)
	return session, nil
}

// loadAndValidateCheckpoint loads and validates a checkpoint file.
func loadAndValidateCheckpoint(path string) (*Checkpoint, error) {
	checkpoint, err := loadCheckpoint(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}
	if err := validateCheckpoint(checkpoint); err != nil {
		return nil, fmt.Errorf("invalid checkpoint: %w", err)
	}
	return checkpoint, nil
}

// resolveImportName returns the override name or falls back to the checkpoint name.
func resolveImportName(override, fallback string) string {
	if override != "" {
		return override
	}
	return fallback
}

// resolveImportPath returns the override path or falls back to the manager path.
func resolveImportPath(override, fallback string) string {
	if override != "" {
		return override
	}
	return fallback
}

// handleExistingSession checks for an existing session and deletes it if overwrite is allowed.
func (m *Manager) handleExistingSession(ctx context.Context, projectPath, sessionName string, overwrite bool) error {
	existing, err := m.storage.GetSessionByName(ctx, projectPath, sessionName)
	if err != nil || existing == nil {
		return nil //nolint:nilerr // If lookup fails, treat as no existing session.
	}
	if !overwrite {
		return fmt.Errorf("%w: session '%s' already exists in project '%s'", errUtils.ErrAISessionAlreadyExists, sessionName, projectPath)
	}
	if err := m.storage.DeleteSession(ctx, existing.ID); err != nil {
		return fmt.Errorf("failed to delete existing session: %w", err)
	}
	return nil
}

// buildImportedSession creates a Session from checkpoint data.
func (m *Manager) buildImportedSession(name, projectPath string, checkpoint *Checkpoint) *Session {
	return &Session{
		ID:          uuid.New().String(),
		Name:        name,
		ProjectPath: projectPath,
		Model:       checkpoint.Session.Model,
		Provider:    checkpoint.Session.Provider,
		Skill:       checkpoint.Session.Skill,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Metadata:    checkpoint.Session.Metadata,
	}
}

// importMessages imports checkpoint messages into storage for a session.
func (m *Manager) importMessages(ctx context.Context, sessionID string, checkpointMsgs []CheckpointMessage) error {
	for _, checkpointMsg := range checkpointMsgs {
		msg := &Message{
			SessionID: sessionID,
			Role:      checkpointMsg.Role,
			Content:   checkpointMsg.Content,
			CreatedAt: checkpointMsg.CreatedAt,
			Archived:  checkpointMsg.Archived,
		}
		if err := m.storage.AddMessage(ctx, msg); err != nil {
			return fmt.Errorf("failed to import message: %w", err)
		}
	}
	return nil
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
		return nil, fmt.Errorf("%w: %s (supported: json, yaml)", errUtils.ErrAIUnsupportedCheckpointFormat, format)
	}

	return &checkpoint, nil
}

// validateCheckpoint validates a checkpoint before import.
func validateCheckpoint(checkpoint *Checkpoint) error {
	if checkpoint == nil {
		return errUtils.ErrAICheckpointNil
	}

	if err := validateCheckpointVersion(checkpoint.Version); err != nil {
		return err
	}

	if err := validateCheckpointSession(&checkpoint.Session); err != nil {
		return err
	}

	if err := validateMessages(checkpoint.Messages); err != nil {
		return err
	}

	// Validate statistics match message count.
	if checkpoint.Statistics.MessageCount != len(checkpoint.Messages) {
		return fmt.Errorf("%w: count is %d but found %d", errUtils.ErrAIStatisticsMismatch, checkpoint.Statistics.MessageCount, len(checkpoint.Messages))
	}

	return nil
}

// validateCheckpointVersion checks the checkpoint version is supported.
func validateCheckpointVersion(version string) error {
	if version == "" {
		return errUtils.ErrAICheckpointVersionMissing
	}
	if version != CheckpointVersion {
		return fmt.Errorf("%w: %s (expected: %s)", errUtils.ErrAIUnsupportedCheckpointVersion, version, CheckpointVersion)
	}
	return nil
}

// validateCheckpointSession validates required session fields.
func validateCheckpointSession(session *CheckpointSession) error {
	if session.Name == "" {
		return errUtils.ErrAISessionNameRequired
	}
	if session.Provider == "" {
		return errUtils.ErrAISessionProviderRequired
	}
	if session.Model == "" {
		return errUtils.ErrAISessionModelRequired
	}
	return nil
}

// validateMessages validates the checkpoint message list.
func validateMessages(messages []CheckpointMessage) error {
	if len(messages) == 0 {
		return errUtils.ErrAICheckpointNoMessages
	}
	for i, msg := range messages {
		if msg.Role == "" {
			return fmt.Errorf("%w: message %d", errUtils.ErrAIMessageRoleRequired, i+1)
		}
		if msg.Role != "user" && msg.Role != "assistant" && msg.Role != "system" {
			return fmt.Errorf("%w: message %d has role '%s' (expected: user, assistant, system)", errUtils.ErrAIMessageInvalidRole, i+1, msg.Role)
		}
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
