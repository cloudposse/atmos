package session

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultMaxSessions is the default maximum number of sessions to keep.
	DefaultMaxSessions = 40
	// DefaultRetentionDays is the default number of days to retain sessions.
	DefaultRetentionDays = 30
)

// Manager handles session lifecycle and operations.
type Manager struct {
	storage          Storage
	projectPath      string
	maxSessions      int
	compactor        Compactor
	atmosConfig      *schema.AtmosConfiguration
	compactCallback  CompactStatusCallback
}

// NewManager creates a new session manager.
func NewManager(storage Storage, projectPath string, maxSessions int, atmosConfig *schema.AtmosConfiguration) *Manager {
	if maxSessions <= 0 {
		maxSessions = DefaultMaxSessions
	}

	// Create compactor.
	compactor := NewCompactor(storage, atmosConfig)

	return &Manager{
		storage:         storage,
		projectPath:     projectPath,
		maxSessions:     maxSessions,
		compactor:       compactor,
		atmosConfig:     atmosConfig,
		compactCallback: nil,
	}
}

// SetCompactStatusCallback sets the callback for compaction status updates.
// This allows UI components to be notified when compaction starts/completes.
func (m *Manager) SetCompactStatusCallback(callback CompactStatusCallback) {
	m.compactCallback = callback
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

// GetMessagesWithCompaction retrieves messages for a session with auto-compact support.
func (m *Manager) GetMessagesWithCompaction(ctx context.Context, sessionID string, limit int) ([]*Message, error) {
	// Get auto-compact configuration.
	compactConfig := m.getCompactConfig()

	// Load active (non-archived) messages.
	activeMessages, err := m.storage.GetActiveMessages(ctx, sessionID, 0) // No limit, we need all for compaction check
	if err != nil {
		return nil, fmt.Errorf("failed to get active messages: %w", err)
	}

	// Load summaries.
	summaries, err := m.storage.GetSummaries(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get summaries: %w", err)
	}

	// Check if compaction is needed.
	maxMessages := m.getMaxMessages()
	if shouldCompact, plan := m.compactor.ShouldCompact(activeMessages, maxMessages, compactConfig); shouldCompact {
		// Set session ID in plan.
		plan.SessionID = sessionID

		// Notify UI that compaction is starting.
		if m.compactCallback != nil {
			m.compactCallback(CompactStatus{
				Stage:            "starting",
				MessageCount:     len(plan.MessagesToCompact),
				EstimatedSavings: plan.EstimatedSavings,
			})
		}

		// Perform compaction.
		result, err := m.compactor.Compact(ctx, plan, compactConfig)
		if err != nil {
			// Notify UI of failure.
			if m.compactCallback != nil {
				m.compactCallback(CompactStatus{
					Stage:        "failed",
					MessageCount: len(plan.MessagesToCompact),
					Error:        err,
				})
			}
			// Log error but continue - compaction failure shouldn't break the session.
			// In production, this would use proper logging.
			fmt.Printf("Warning: auto-compact failed: %v\n", err)
		} else {
			// Notify UI of success.
			if m.compactCallback != nil {
				m.compactCallback(CompactStatus{
					Stage:            "completed",
					MessageCount:     len(plan.MessagesToCompact),
					EstimatedSavings: result.TokenCount,
				})
			}
			// Reload active messages and summaries after compaction.
			activeMessages, _ = m.storage.GetActiveMessages(ctx, sessionID, 0)
			summaries, _ = m.storage.GetSummaries(ctx, sessionID)
		}
	}

	// Combine summaries and active messages.
	combinedMessages := m.combineMessagesAndSummaries(summaries, activeMessages, compactConfig)

	// Apply limit if specified.
	if limit > 0 && len(combinedMessages) > limit {
		// Take the most recent messages.
		combinedMessages = combinedMessages[len(combinedMessages)-limit:]
	}

	return combinedMessages, nil
}

// combineMessagesAndSummaries combines summaries and active messages in chronological order.
func (m *Manager) combineMessagesAndSummaries(summaries []*Summary, messages []*Message, config CompactConfig) []*Message {
	var combined []*Message

	// Add summaries as special assistant messages.
	for _, summary := range summaries {
		content := summary.SummaryContent

		// Add markers if configured.
		if config.ShowSummaryMarkers {
			content = fmt.Sprintf("[SUMMARY: %s]\n\n%s\n\n[END SUMMARY]",
				summary.MessageRange, summary.SummaryContent)
		}

		combined = append(combined, &Message{
			ID:        -1, // Negative ID to indicate it's a summary.
			SessionID: summary.SessionID,
			Role:      RoleAssistant,
			Content:   content,
			CreatedAt: summary.CompactedAt,
			Archived:  false,
			IsSummary: true,
		})
	}

	// Add active messages.
	combined = append(combined, messages...)

	// Sort by timestamp.
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].CreatedAt.Before(combined[j].CreatedAt)
	})

	return combined
}

// getCompactConfig returns the auto-compact configuration.
func (m *Manager) getCompactConfig() CompactConfig {
	if m.atmosConfig == nil || !m.atmosConfig.Settings.AI.Sessions.AutoCompact.Enabled {
		return DefaultCompactConfig()
	}

	config := m.atmosConfig.Settings.AI.Sessions.AutoCompact

	// Convert schema config to session config.
	return CompactConfig{
		Enabled:            config.Enabled,
		TriggerThreshold:   getOrDefault(config.TriggerThreshold, 0.75),
		CompactRatio:       getOrDefault(config.CompactRatio, 0.4),
		PreserveRecent:     getOrDefaultInt(config.PreserveRecent, 10),
		UseAISummary:       getOrDefaultBool(config.UseAISummary, true),
		SummaryProvider:    config.SummaryProvider,
		SummaryModel:       config.SummaryModel,
		SummaryMaxTokens:   getOrDefaultInt(config.SummaryMaxTokens, 2048),
		ShowSummaryMarkers: config.ShowSummaryMarkers,
		CompactOnResume:    config.CompactOnResume,
	}
}

// getMaxMessages returns the configured max messages limit.
func (m *Manager) getMaxMessages() int {
	if m.atmosConfig == nil {
		return 0 // Unlimited
	}

	return m.atmosConfig.Settings.AI.MaxHistoryMessages
}

// Helper functions for config defaults.
func getOrDefault(value float64, defaultValue float64) float64 {
	if value == 0 {
		return defaultValue
	}
	return value
}

func getOrDefaultInt(value int, defaultValue int) int {
	if value == 0 {
		return defaultValue
	}
	return value
}

func getOrDefaultBool(value bool, defaultValue bool) bool {
	// Note: false is the zero value, so we can't distinguish between false and unset.
	// For now, we'll use the provided value directly.
	return value
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
