package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	// DefaultMaxSessions is the default maximum number of sessions to keep.
	DefaultMaxSessions = 40
	// DefaultRetentionDays is the default number of days to retain sessions.
	DefaultRetentionDays = 30
)

// Manager handles session lifecycle and operations.
type Manager struct {
	storage     Storage
	projectPath string
	maxSessions int
}

// NewManager creates a new session manager.
func NewManager(storage Storage, projectPath string, maxSessions int) *Manager {
	if maxSessions <= 0 {
		maxSessions = DefaultMaxSessions
	}

	return &Manager{
		storage:     storage,
		projectPath: projectPath,
		maxSessions: maxSessions,
	}
}

// CreateSession creates a new session.
func (m *Manager) CreateSession(ctx context.Context, name, model, provider, agent string, metadata map[string]interface{}) (*Session, error) {
	// Generate unique ID.
	id := uuid.New().String()

	// If no name provided, generate timestamp-based name.
	if name == "" {
		name = fmt.Sprintf("session-%s", time.Now().Format("20060102-150405"))
	}

	session := &Session{
		ID:          id,
		Name:        name,
		ProjectPath: m.projectPath,
		Model:       model,
		Provider:    provider,
		Agent:       agent,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Metadata:    metadata,
	}

	if err := m.storage.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// GetSession retrieves a session by ID.
func (m *Manager) GetSession(ctx context.Context, id string) (*Session, error) {
	session, err := m.storage.GetSession(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return session, nil
}

// GetSessionByName retrieves a session by name within the current project.
func (m *Manager) GetSessionByName(ctx context.Context, name string) (*Session, error) {
	session, err := m.storage.GetSessionByName(ctx, m.projectPath, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get session by name: %w", err)
	}

	return session, nil
}

// ListSessions returns all sessions for the current project.
func (m *Manager) ListSessions(ctx context.Context) ([]*Session, error) {
	sessions, err := m.storage.ListSessions(ctx, m.projectPath, m.maxSessions)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	return sessions, nil
}

// AddMessage adds a message to a session.
func (m *Manager) AddMessage(ctx context.Context, sessionID, role, content string) error {
	// Verify session exists.
	if _, err := m.storage.GetSession(ctx, sessionID); err != nil {
		return errUtils.ErrAISessionNotFound
	}

	message := &Message{
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	}

	if err := m.storage.AddMessage(ctx, message); err != nil {
		return fmt.Errorf("failed to add message: %w", err)
	}

	// Update session timestamp.
	session, _ := m.storage.GetSession(ctx, sessionID)
	session.UpdatedAt = time.Now()
	_ = m.storage.UpdateSession(ctx, session)

	return nil
}

// GetMessages retrieves messages for a session.
func (m *Manager) GetMessages(ctx context.Context, sessionID string, limit int) ([]*Message, error) {
	messages, err := m.storage.GetMessages(ctx, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	return messages, nil
}

// GetMessageCount returns the number of messages in a session.
func (m *Manager) GetMessageCount(ctx context.Context, sessionID string) (int, error) {
	count, err := m.storage.GetMessageCount(ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("failed to get message count: %w", err)
	}

	return count, nil
}

// AddContext adds a context item to a session.
func (m *Manager) AddContext(ctx context.Context, sessionID, contextType, key, value string) error {
	item := &ContextItem{
		SessionID:    sessionID,
		ContextType:  contextType,
		ContextKey:   key,
		ContextValue: value,
	}

	if err := m.storage.AddContext(ctx, item); err != nil {
		return fmt.Errorf("failed to add context: %w", err)
	}

	return nil
}

// GetContext retrieves all context items for a session.
func (m *Manager) GetContext(ctx context.Context, sessionID string) ([]*ContextItem, error) {
	items, err := m.storage.GetContext(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get context: %w", err)
	}

	return items, nil
}

// UpdateSession updates an existing session.
func (m *Manager) UpdateSession(ctx context.Context, sess *Session) error {
	if err := m.storage.UpdateSession(ctx, sess); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// DeleteSession deletes a session and all its data.
func (m *Manager) DeleteSession(ctx context.Context, id string) error {
	// Delete context first.
	_ = m.storage.DeleteContext(ctx, id)

	// Delete session.
	if err := m.storage.DeleteSession(ctx, id); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// CleanOldSessions removes sessions older than the specified duration.
func (m *Manager) CleanOldSessions(ctx context.Context, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		retentionDays = DefaultRetentionDays
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	count, err := m.storage.DeleteOldSessions(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to clean old sessions: %w", err)
	}

	return count, nil
}

// Close closes the storage backend.
func (m *Manager) Close() error {
	return m.storage.Close()
}
