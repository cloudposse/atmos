package session

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockStorage implements Storage interface for testing.
type MockStorage struct {
	summaries        []*Summary
	archivedMessages []int64
	activeMessages   []*Message
}

func (m *MockStorage) StoreSummary(ctx context.Context, summary *Summary) error {
	m.summaries = append(m.summaries, summary)
	return nil
}

func (m *MockStorage) GetSummaries(ctx context.Context, sessionID string) ([]*Summary, error) {
	return m.summaries, nil
}

func (m *MockStorage) ArchiveMessages(ctx context.Context, messageIDs []int64) error {
	m.archivedMessages = append(m.archivedMessages, messageIDs...)
	return nil
}

func (m *MockStorage) GetActiveMessages(ctx context.Context, sessionID string, limit int) ([]*Message, error) {
	return m.activeMessages, nil
}

// Implement remaining Storage interface methods as no-ops for testing.
func (m *MockStorage) CreateSession(ctx context.Context, session *Session) error { return nil }

func (m *MockStorage) GetSession(ctx context.Context, id string) (*Session, error) {
	return nil, nil
}

func (m *MockStorage) GetSessionByName(ctx context.Context, projectPath, name string) (*Session, error) {
	return nil, nil
}

func (m *MockStorage) ListSessions(ctx context.Context, projectPath string, limit int) ([]*Session, error) {
	return nil, nil
}
func (m *MockStorage) UpdateSession(ctx context.Context, session *Session) error { return nil }
func (m *MockStorage) DeleteSession(ctx context.Context, id string) error        { return nil }
func (m *MockStorage) DeleteOldSessions(ctx context.Context, olderThan time.Time) (int, error) {
	return 0, nil
}
func (m *MockStorage) AddMessage(ctx context.Context, message *Message) error { return nil }
func (m *MockStorage) GetMessages(ctx context.Context, sessionID string, limit int) ([]*Message, error) {
	return nil, nil
}

func (m *MockStorage) GetMessageCount(ctx context.Context, sessionID string) (int, error) {
	return 0, nil
}
func (m *MockStorage) AddContext(ctx context.Context, item *ContextItem) error { return nil }
func (m *MockStorage) GetContext(ctx context.Context, sessionID string) ([]*ContextItem, error) {
	return nil, nil
}
func (m *MockStorage) DeleteContext(ctx context.Context, sessionID string) error { return nil }
func (m *MockStorage) Close() error                                              { return nil }
func (m *MockStorage) Migrate() error                                            { return nil }

// generateTestMessages creates test messages for compaction testing.
func generateTestMessages(count int) []*Message {
	messages := make([]*Message, count)
	baseTime := time.Now().Add(-time.Hour * time.Duration(count))

	for i := 0; i < count; i++ {
		role := RoleUser
		if i%2 == 1 {
			role = RoleAssistant
		}

		messages[i] = &Message{
			ID:        int64(i + 1),
			SessionID: "test-session",
			Role:      role,
			Content:   "Test message " + string(rune(i+1)),
			CreatedAt: baseTime.Add(time.Minute * time.Duration(i)),
			Archived:  false,
		}
	}

	return messages
}

func TestCompactor_ShouldCompact(t *testing.T) {
	storage := &MockStorage{}
	compactor := NewCompactor(storage, nil)

	tests := []struct {
		name               string
		messageCount       int
		maxMessages        int
		threshold          float64
		compactRatio       float64
		preserveRecent     int
		expectCompact      bool
		expectNumToCompact int
	}{
		{
			name:           "below threshold - no compaction",
			messageCount:   30,
			maxMessages:    50,
			threshold:      0.75,
			compactRatio:   0.4,
			preserveRecent: 10,
			expectCompact:  false,
		},
		{
			name:               "at threshold - trigger compaction",
			messageCount:       38,
			maxMessages:        50,
			threshold:          0.75,
			compactRatio:       0.4,
			preserveRecent:     10,
			expectCompact:      true,
			expectNumToCompact: 15, // 38 * 0.4 = 15.2, rounded down
		},
		{
			name:               "above threshold - trigger compaction",
			messageCount:       45,
			maxMessages:        50,
			threshold:          0.75,
			compactRatio:       0.4,
			preserveRecent:     10,
			expectCompact:      true,
			expectNumToCompact: 18, // 45 * 0.4 = 18
		},
		{
			name:               "preserve recent limits compaction",
			messageCount:       25,
			maxMessages:        30,
			threshold:          0.75,
			compactRatio:       0.8, // Want to compact 20 messages (80%)
			preserveRecent:     20,  // But must preserve 20
			expectCompact:      true,
			expectNumToCompact: 5, // Can only compact 5 (25 - 20 = 5)
		},
		{
			name:           "disabled - no compaction",
			messageCount:   45,
			maxMessages:    50,
			threshold:      0.75,
			compactRatio:   0.4,
			preserveRecent: 10,
			expectCompact:  false, // Explicitly disabled below
		},
		{
			name:           "unlimited messages - no compaction",
			messageCount:   100,
			maxMessages:    0, // Unlimited
			threshold:      0.75,
			compactRatio:   0.4,
			preserveRecent: 10,
			expectCompact:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := generateTestMessages(tt.messageCount)
			config := CompactConfig{
				Enabled:          tt.name != "disabled - no compaction",
				TriggerThreshold: tt.threshold,
				CompactRatio:     tt.compactRatio,
				PreserveRecent:   tt.preserveRecent,
			}

			shouldCompact, plan := compactor.ShouldCompact(messages, tt.maxMessages, config)

			assert.Equal(t, tt.expectCompact, shouldCompact, "ShouldCompact mismatch")

			if tt.expectCompact {
				require.NotNil(t, plan, "Plan should not be nil when compaction is needed")
				assert.Equal(t, tt.messageCount, plan.TotalMessages, "Total messages mismatch")
				assert.Equal(t, tt.expectNumToCompact, len(plan.MessagesToCompact), "Messages to compact count mismatch")
				assert.Equal(t, tt.messageCount-tt.expectNumToCompact, len(plan.MessagesToKeep), "Messages to keep count mismatch")
				assert.NotEmpty(t, plan.Reason, "Reason should be provided")
			} else {
				if plan != nil {
					assert.Nil(t, plan, "Plan should be nil when no compaction needed")
				}
			}
		})
	}
}

func TestCompactor_Compact_SimpleSummary(t *testing.T) {
	storage := &MockStorage{}
	compactor := NewCompactor(storage, nil)

	messages := generateTestMessages(20)
	plan := &CompactPlan{
		SessionID:         "test-session",
		TotalMessages:     20,
		MessagesToCompact: messages[:10],
		MessagesToKeep:    messages[10:],
		Reason:            "Test compaction",
	}

	config := CompactConfig{
		Enabled:      true,
		UseAISummary: false, // Force simple summary
	}

	result, err := compactor.Compact(context.Background(), plan, config)

	require.NoError(t, err, "Compact should not error")
	require.NotNil(t, result, "Result should not be nil")

	// Verify result.
	assert.True(t, result.Success, "Compaction should succeed")
	assert.NotEmpty(t, result.SummaryID, "Summary ID should be generated")
	assert.NotEmpty(t, result.SummaryContent, "Summary content should not be empty")
	assert.Equal(t, 10, len(result.OriginalMessageIDs), "Should have 10 archived message IDs")

	// Verify storage was called.
	assert.Len(t, storage.summaries, 1, "Should store one summary")
	assert.Len(t, storage.archivedMessages, 10, "Should archive 10 messages")

	// Verify summary content format.
	assert.Contains(t, result.SummaryContent, "SUMMARY OF EARLIER CONVERSATION", "Summary should have header")
	assert.Contains(t, result.SummaryContent, "Summarized 10 messages", "Summary should include count")
}

func TestCompactor_GenerateSimpleSummary(t *testing.T) {
	storage := &MockStorage{}
	compactor := NewCompactor(storage, nil).(*DefaultCompactor)

	messages := []*Message{
		{
			ID:      1,
			Role:    RoleUser,
			Content: "What is the VPC CIDR?",
		},
		{
			ID:      2,
			Role:    RoleAssistant,
			Content: "The VPC CIDR is 10.0.0.0/16 for production",
		},
		{
			ID:      3,
			Role:    RoleUser,
			Content: strings.Repeat("Very long message content ", 20), // Long message to test truncation
		},
	}

	summary := compactor.generateSimpleSummary(messages)

	// Verify summary structure.
	assert.Contains(t, summary, "SUMMARY OF EARLIER CONVERSATION", "Should have header")
	assert.Contains(t, summary, "[user]:", "Should include user role")
	assert.Contains(t, summary, "[assistant]:", "Should include assistant role")
	assert.Contains(t, summary, "Summarized 3 messages", "Should include message count")

	// Verify truncation of long messages.
	assert.Contains(t, summary, "...", "Long messages should be truncated")

	// Verify actual content is included.
	assert.Contains(t, summary, "VPC CIDR", "Should include actual message content")
}

func TestCompactor_BuildSummarizationPrompt(t *testing.T) {
	storage := &MockStorage{}
	compactor := NewCompactor(storage, nil).(*DefaultCompactor)

	messages := []*Message{
		{
			ID:      1,
			Role:    RoleUser,
			Content: "Configure VPC for production",
		},
		{
			ID:      2,
			Role:    RoleAssistant,
			Content: "I'll help configure the VPC. What CIDR should we use?",
		},
		{
			ID:      3,
			Role:    RoleUser,
			Content: "Use 10.0.0.0/16 for growth capacity",
		},
	}

	prompt := compactor.buildSummarizationPrompt(messages)

	// Verify prompt structure.
	assert.Contains(t, prompt, "infrastructure management with Atmos", "Should mention Atmos")
	assert.Contains(t, prompt, "Infrastructure decisions", "Should emphasize infrastructure decisions")
	assert.Contains(t, prompt, "VPC", "Should mention VPC in context")
	assert.Contains(t, prompt, "Security decisions", "Should mention security")
	assert.Contains(t, prompt, "CONVERSATION TO SUMMARIZE:", "Should have conversation section")

	// Verify message content is included.
	assert.Contains(t, prompt, "Configure VPC for production", "Should include message 1")
	assert.Contains(t, prompt, "10.0.0.0/16", "Should include message 3")

	// Verify formatting.
	assert.Contains(t, prompt, "[Message 1] User:", "Should format user messages")
	assert.Contains(t, prompt, "[Message 2] Assistant:", "Should format assistant messages")

	// Verify instructions.
	assert.Contains(t, prompt, "DO NOT include:", "Should have exclusion instructions")
	assert.Contains(t, prompt, "150-400 words", "Should specify summary length")
}

func TestCompactor_DefaultCompactConfig(t *testing.T) {
	config := DefaultCompactConfig()

	// Verify defaults.
	assert.False(t, config.Enabled, "Should be disabled by default")
	assert.Equal(t, 0.75, config.TriggerThreshold, "Default threshold should be 0.75")
	assert.Equal(t, 0.4, config.CompactRatio, "Default ratio should be 0.4")
	assert.Equal(t, 10, config.PreserveRecent, "Default preserve should be 10")
	assert.True(t, config.UseAISummary, "Should use AI by default")
	assert.Equal(t, 2048, config.SummaryMaxTokens, "Default max tokens should be 2048")
	assert.False(t, config.ShowSummaryMarkers, "Should not show markers by default")
	assert.False(t, config.CompactOnResume, "Should not compact on resume by default")
}

func TestCompactor_Compact_WithNilPlan(t *testing.T) {
	storage := &MockStorage{}
	compactor := NewCompactor(storage, nil)

	config := CompactConfig{
		Enabled: true,
	}

	result, err := compactor.Compact(context.Background(), nil, config)

	assert.Error(t, err, "Should error with nil plan")
	assert.Nil(t, result, "Result should be nil")
	assert.Contains(t, err.Error(), "plan is nil", "Error should mention nil plan")
}

func TestCompactor_MessageSelection(t *testing.T) {
	storage := &MockStorage{}
	compactor := NewCompactor(storage, nil)

	// Create 50 messages.
	messages := generateTestMessages(50)

	config := CompactConfig{
		Enabled:          true,
		TriggerThreshold: 0.75, // Trigger at 38
		CompactRatio:     0.4,  // Compact 20 messages (40% of 50)
		PreserveRecent:   10,   // Keep last 10
	}

	shouldCompact, plan := compactor.ShouldCompact(messages, 50, config)

	require.True(t, shouldCompact, "Should trigger compaction")
	require.NotNil(t, plan, "Plan should not be nil")

	// Verify message selection.
	assert.Equal(t, 20, len(plan.MessagesToCompact), "Should select 20 messages to compact")
	assert.Equal(t, 30, len(plan.MessagesToKeep), "Should keep 30 messages")

	// Verify oldest messages are selected.
	assert.Equal(t, int64(1), plan.MessagesToCompact[0].ID, "First message should be ID 1")
	assert.Equal(t, int64(20), plan.MessagesToCompact[len(plan.MessagesToCompact)-1].ID, "Last compacted should be ID 20")

	// Verify kept messages are the newer ones.
	assert.Equal(t, int64(21), plan.MessagesToKeep[0].ID, "First kept message should be ID 21")
	assert.Equal(t, int64(50), plan.MessagesToKeep[len(plan.MessagesToKeep)-1].ID, "Last kept message should be ID 50")
}

func TestCompactor_SummaryStorage(t *testing.T) {
	storage := &MockStorage{}
	compactor := NewCompactor(storage, nil)

	messages := generateTestMessages(10)
	plan := &CompactPlan{
		SessionID:         "test-session-123",
		TotalMessages:     10,
		MessagesToCompact: messages,
		MessagesToKeep:    []*Message{},
		Reason:            "Test storage",
	}

	config := CompactConfig{
		Enabled:      true,
		UseAISummary: false,
	}

	result, err := compactor.Compact(context.Background(), plan, config)

	require.NoError(t, err, "Compact should succeed")
	require.NotNil(t, result, "Result should not be nil")

	// Verify summary was stored.
	require.Len(t, storage.summaries, 1, "Should store exactly one summary")

	summary := storage.summaries[0]
	assert.Equal(t, "test-session-123", summary.SessionID, "Session ID should match")
	assert.Equal(t, result.SummaryID, summary.ID, "Summary ID should match result")
	assert.NotEmpty(t, summary.SummaryContent, "Summary content should not be empty")
	assert.Equal(t, "Messages 1-10", summary.MessageRange, "Message range should be formatted correctly")
	assert.Len(t, summary.OriginalMessageIDs, 10, "Should have 10 original message IDs")

	// Verify messages were archived.
	assert.Len(t, storage.archivedMessages, 10, "Should archive 10 messages")
	for i := 0; i < 10; i++ {
		assert.Contains(t, storage.archivedMessages, int64(i+1), "Message %d should be archived", i+1)
	}
}

func TestCompactor_AIFallback(t *testing.T) {
	storage := &MockStorage{}

	// Create compactor with nil config to force AI failure.
	compactor := NewCompactor(storage, nil)

	messages := generateTestMessages(5)
	plan := &CompactPlan{
		SessionID:         "test-session",
		TotalMessages:     5,
		MessagesToCompact: messages,
		MessagesToKeep:    []*Message{},
		Reason:            "Test AI fallback",
	}

	config := CompactConfig{
		Enabled:      true,
		UseAISummary: true, // Try AI but should fail
	}

	// Should succeed with fallback to simple summary.
	result, err := compactor.Compact(context.Background(), plan, config)

	require.NoError(t, err, "Should not error even when AI fails")
	require.NotNil(t, result, "Result should not be nil")
	assert.True(t, result.Success, "Should succeed with fallback")
	assert.Contains(t, result.SummaryContent, "SUMMARY OF EARLIER CONVERSATION", "Should use simple summary")
}

func TestCompactor_EmptyMessages(t *testing.T) {
	storage := &MockStorage{}
	compactor := NewCompactor(storage, nil)

	var messages []*Message

	config := CompactConfig{
		Enabled:          true,
		TriggerThreshold: 0.75,
		CompactRatio:     0.4,
		PreserveRecent:   10,
	}

	shouldCompact, plan := compactor.ShouldCompact(messages, 50, config)

	assert.False(t, shouldCompact, "Should not compact empty messages")
	assert.Nil(t, plan, "Plan should be nil for empty messages")
}

func TestCompactor_TokenEstimation(t *testing.T) {
	storage := &MockStorage{}
	compactor := NewCompactor(storage, nil)

	messages := []*Message{
		{
			ID:      1,
			Role:    RoleUser,
			Content: strings.Repeat("test ", 100), // ~500 chars = ~125 tokens
		},
		{
			ID:      2,
			Role:    RoleAssistant,
			Content: strings.Repeat("response ", 100), // ~900 chars = ~225 tokens
		},
	}

	plan := &CompactPlan{
		SessionID:         "test-session",
		TotalMessages:     2,
		MessagesToCompact: messages,
		MessagesToKeep:    []*Message{},
	}

	config := CompactConfig{
		Enabled:      true,
		UseAISummary: false,
	}

	result, err := compactor.Compact(context.Background(), plan, config)

	require.NoError(t, err, "Compact should succeed")
	require.NotNil(t, result, "Result should not be nil")

	// Token count should be estimated (rough: chars / 4).
	assert.Greater(t, result.TokenCount, 0, "Token count should be estimated")
}
