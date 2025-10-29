# PRD: AI Session Auto-Compact

## Overview

Implement intelligent conversation history compaction for Atmos AI sessions to enable extended multi-day conversations without context loss. This feature automatically summarizes older messages when approaching context limits, preserving important infrastructure decisions and architectural context while preventing rate limiting and excessive token usage.

## Motivation

### Current Limitations

Atmos AI currently supports two manual truncation approaches:

1. **Message-based truncation** (`max_history_messages: 20`) - Drops oldest messages entirely when limit is reached
2. **Token-based truncation** (`max_history_tokens: 8000`) - Drops oldest messages based on estimated token count

**Problem**: Both approaches **discard** historical context rather than preserving it in compressed form.

### Why This Matters for Infrastructure Management

Infrastructure conversations have unique characteristics that make context preservation critical:

- **Long-term decision tracking**: "Why did we choose CIDR 10.0.0.0/16 three weeks ago?"
- **Multi-day workflows**: VPC migrations, security audits, architecture reviews span days or weeks
- **Dependency chains**: Later decisions depend on context from 30-50 messages earlier
- **Tool execution history**: Remembering which components were analyzed, which files were validated
- **Security context**: Understanding historical security decisions and their rationale

### User Pain Points

1. **Context Loss**: Users must re-explain background when conversation exceeds limits
2. **Manual Configuration**: Users must predict optimal `max_history_messages` and `max_history_tokens` values
3. **Rate Limiting**: Setting limits too high causes provider rate limits; too low loses context
4. **Fragmented Sessions**: Users forced to start new sessions, losing continuity
5. **Inefficient Workflows**: Re-explaining infrastructure architecture wastes time

### Inspiration: Claude Code's Auto-Compact

Claude Code implements intelligent auto-compact that:
- Automatically summarizes earlier conversation when context is 75-85% full
- Preserves important technical decisions and reasoning
- Allows conversations to continue indefinitely without manual intervention
- Users rarely notice compaction happening
- Trades verbatim history for semantic preservation

## Goals

### Primary Goals

1. **Enable Extended Conversations**: Support multi-day/week infrastructure conversations without context loss
2. **Intelligent Preservation**: Summarize older messages rather than dropping them
3. **Automatic Operation**: Work without user intervention once configured
4. **Transparent Experience**: Users shouldn't notice compaction happening
5. **Rate Limit Management**: Prevent hitting provider rate limits automatically

### Secondary Goals

1. **Configurable Behavior**: Allow users to tune compaction triggers and behavior
2. **Backward Compatibility**: Existing truncation settings continue to work
3. **Multi-Provider Support**: Work across all 7 AI providers (Anthropic, OpenAI, Gemini, Grok, Ollama, Bedrock, Azure)
4. **Cost Transparency**: Make token usage for summaries visible and controllable
5. **Session-Specific Settings**: Allow per-session compaction configuration

### Non-Goals

1. **Replace Existing Truncation**: Current `max_history_messages`/`max_history_tokens` remain available
2. **Guarantee Perfect Recall**: Compaction trades verbatim accuracy for semantic preservation
3. **Real-time Compaction**: Compaction happens between messages, not mid-response
4. **Context Reconstruction**: No ability to "uncompress" summaries back to original messages
5. **Cross-Session Compaction**: Each session manages its own history independently

## Current State

### Existing Context Management

Located in `pkg/ai/session/`:

```go
// Current truncation in session manager
func (m *Manager) GetMessagesForProvider(sessionID, providerName string) ([]Message, error) {
    messages := m.loadAllMessages(sessionID)

    // Filter by provider (multi-provider sessions)
    messages = filterByProvider(messages, providerName)

    // Apply message-based limit
    if m.config.MaxHistoryMessages > 0 {
        messages = truncateByMessageCount(messages, m.config.MaxHistoryMessages)
    }

    // Apply token-based limit
    if m.config.MaxHistoryTokens > 0 {
        messages = truncateByTokens(messages, m.config.MaxHistoryTokens)
    }

    return messages, nil
}
```

**Limitations**:
- Simple truncation: drops messages entirely
- No semantic preservation
- No awareness of message importance
- No summary generation

### Session Storage Schema

SQLite database at `.atmos/sessions/sessions.db`:

```sql
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    name TEXT,
    provider TEXT,
    model TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

CREATE TABLE messages (
    id TEXT PRIMARY KEY,
    session_id TEXT,
    role TEXT,              -- 'user' or 'assistant'
    content TEXT,
    provider TEXT,
    created_at TIMESTAMP,
    FOREIGN KEY(session_id) REFERENCES sessions(id)
);

CREATE TABLE session_context (
    id TEXT PRIMARY KEY,
    session_id TEXT,
    context_type TEXT,
    context_data TEXT,
    created_at TIMESTAMP,
    FOREIGN KEY(session_id) REFERENCES sessions(id)
);
```

**New table needed** for compaction metadata:

```sql
CREATE TABLE message_summaries (
    id TEXT PRIMARY KEY,
    session_id TEXT,
    original_message_ids TEXT,    -- JSON array of compacted message IDs
    summary_content TEXT,          -- The generated summary
    message_range TEXT,            -- "Messages 1-20" for debugging
    compacted_at TIMESTAMP,
    token_count INTEGER,
    FOREIGN KEY(session_id) REFERENCES sessions(id)
);
```

## Proposed Solution

### Feature Overview

Auto-compact intelligently summarizes conversation history when approaching configured limits:

1. **Monitor Context Usage**: Track message count and estimated token usage
2. **Trigger at Threshold**: When 75% of limit is reached, trigger compaction
3. **AI-Generated Summary**: Use AI to summarize oldest portion of history
4. **Replace Messages**: Swap original messages with compact summary
5. **Continue Transparently**: User conversation continues without interruption

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                   Atmos AI Chat Session                      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │ Session Manager │
                    └────────┬────────┘
                             │
                             ├──► Monitor History Size
                             │    (messages/tokens)
                             │
                             ├──► Check Threshold
                             │    (75% of limit?)
                             │
                             ├──► Trigger Compaction
                             │    (if needed)
                             │
                             ▼
                    ┌─────────────────┐
                    │ Auto-Compactor  │
                    └────────┬────────┘
                             │
                             ├──► Select Messages to Compact
                             │    (oldest 40%, preserve recent 10)
                             │
                             ├──► Generate Summary
                             │    (via AI provider)
                             │
                             ├──► Store Summary
                             │    (message_summaries table)
                             │
                             ├──► Mark Messages as Archived
                             │    (soft delete)
                             │
                             └──► Return Compacted History
```

### User Experience

#### Default Behavior (Auto-Compact Disabled)

```yaml
# atmos.yaml - current behavior preserved
settings:
  ai:
    sessions:
      max_history_messages: 50
```

**Result**: Hard truncation at 50 messages (oldest dropped entirely)

#### Auto-Compact Enabled

```yaml
# atmos.yaml - new intelligent behavior
settings:
  ai:
    sessions:
      max_history_messages: 50
      auto_compact:
        enabled: true
```

**Result**:
- Messages 1-38: No action
- Message 39 (78% full): Compaction triggered
- Messages 1-20: Summarized into 3-5 summary messages
- Messages 21-39: Kept verbatim
- **Total**: ~22-24 messages (5 summary + 19 recent)
- **User sees**: No interruption, conversation continues naturally

#### Example Conversation Flow

```
Session: vpc-migration-2025

[Messages 1-10: Planning VPC CIDR scheme]
User: "Let's use 10.0.0.0/16 for production"
AI: "Good choice. That gives 65,536 IPs for growth..."

[Messages 11-20: Discussing subnet layout]
User: "Create 3 public subnets in different AZs"
AI: "I'll create /20 subnets in us-east-1a, 1b, 1c..."

[Messages 21-38: Security group configuration, routing tables]

--- AUTO-COMPACT TRIGGERED AT MESSAGE 39 (78% of 50 limit) ---

Messages 1-20 replaced with summary:
┌────────────────────────────────────────────────────────────┐
│ SUMMARY (Messages 1-20)                                     │
│                                                             │
│ VPC Migration Planning:                                    │
│ - Chose 10.0.0.0/16 CIDR for production (growth capacity)  │
│ - Subnet design: 3 public /20 subnets across AZs           │
│ - Subnets: us-east-1a, us-east-1b, us-east-1c             │
│ - Discussed NAT gateway redundancy (one per AZ)           │
│ - Security: isolated private subnets for databases         │
└────────────────────────────────────────────────────────────┘

[Messages 21-38: Kept verbatim]
[Message 39+: New messages continue...]

--- WEEK LATER ---

User: "Why did we choose /16 instead of /24 again?"
AI: "Based on our earlier discussion (in the summary), we chose 10.0.0.0/16
     for growth capacity. A /24 would only give us 256 IPs, which wouldn't
     be sufficient for your planned EKS nodes and services."
```

### Configuration Schema

```yaml
settings:
  ai:
    sessions:
      enabled: true

      # Existing limits (unchanged)
      max_history_messages: 50
      max_history_tokens: 10000

      # New auto-compact configuration
      auto_compact:
        # Enable/disable auto-compact feature
        enabled: false                    # Default: false (opt-in)

        # When to trigger compaction (0.0-1.0)
        trigger_threshold: 0.75           # Default: 0.75 (75% full)

        # What percentage of oldest messages to compact (0.0-1.0)
        compact_ratio: 0.4                # Default: 0.4 (oldest 40%)

        # How many recent messages to never compact
        preserve_recent: 10               # Default: 10

        # Use AI to generate summaries (vs simple concatenation)
        use_ai_summary: true              # Default: true

        # Which provider to use for summarization
        # If not specified, uses the session's current provider
        summary_provider: "anthropic"     # Optional

        # Model to use for summarization (typically cheaper/faster)
        summary_model: "claude-3-haiku-20240307"  # Optional

        # Maximum tokens for summary generation
        summary_max_tokens: 2048          # Default: 2048

        # Include summary markers in conversation
        show_summary_markers: false       # Default: false

        # Automatically compact on session resume
        compact_on_resume: false          # Default: false
```

### Configuration Options Detailed

<dl>
  <dt><code>auto_compact.enabled</code></dt>
  <dd>Enable intelligent auto-compact feature. Default: <code>false</code> (opt-in for backward compatibility)</dd>

  <dt><code>auto_compact.trigger_threshold</code></dt>
  <dd>Percentage (0.0-1.0) of max history limit at which to trigger compaction. Default: <code>0.75</code> (75% full). Lower values trigger earlier, higher values wait longer.</dd>

  <dt><code>auto_compact.compact_ratio</code></dt>
  <dd>Percentage (0.0-1.0) of oldest messages to compact. Default: <code>0.4</code> (oldest 40%). Example: With 50 messages, compacts messages 1-20.</dd>

  <dt><code>auto_compact.preserve_recent</code></dt>
  <dd>Number of most recent messages to never compact, regardless of other settings. Default: <code>10</code>. Ensures immediate context is always available verbatim.</dd>

  <dt><code>auto_compact.use_ai_summary</code></dt>
  <dd>Use AI to generate intelligent summaries. If <code>false</code>, uses simple message concatenation. Default: <code>true</code> (recommended for quality).</dd>

  <dt><code>auto_compact.summary_provider</code></dt>
  <dd>AI provider to use for generating summaries. If not specified, uses session's current provider. Useful for using cheaper provider for summaries (e.g., Haiku instead of Sonnet).</dd>

  <dt><code>auto_compact.summary_model</code></dt>
  <dd>Model to use for summary generation. Defaults to provider's default model. Use cheaper/faster models to reduce costs.</dd>

  <dt><code>auto_compact.summary_max_tokens</code></dt>
  <dd>Maximum tokens for generated summaries. Default: <code>2048</code>. Limits summary length to control costs and preserve brevity.</dd>

  <dt><code>auto_compact.show_summary_markers</code></dt>
  <dd>Display visual markers showing where summaries were inserted. Default: <code>false</code>. Enable for debugging or transparency.</dd>

  <dt><code>auto_compact.compact_on_resume</code></dt>
  <dd>Automatically compact session history when resuming an old session. Default: <code>false</code>. Useful for cleaning up old sessions proactively.</dd>
</dl>

## Technical Design

### Components

#### 1. Auto-Compactor Interface

```go
// pkg/ai/session/compactor.go

// Compactor handles intelligent conversation history compaction.
type Compactor interface {
    // ShouldCompact determines if compaction is needed based on current history.
    ShouldCompact(messages []Message, config CompactConfig) (bool, CompactPlan)

    // Compact performs the compaction operation.
    Compact(ctx context.Context, plan CompactPlan) (*CompactResult, error)
}

// CompactConfig holds compaction configuration.
type CompactConfig struct {
    Enabled           bool
    TriggerThreshold  float64
    CompactRatio      float64
    PreserveRecent    int
    UseAISummary      bool
    SummaryProvider   string
    SummaryModel      string
    SummaryMaxTokens  int
    ShowSummaryMarkers bool
}

// CompactPlan describes what will be compacted.
type CompactPlan struct {
    SessionID          string
    TotalMessages      int
    MessagesToCompact  []Message
    MessagesToKeep     []Message
    EstimatedSavings   int // Token savings estimate
    Reason             string
}

// CompactResult contains the outcome of compaction.
type CompactResult struct {
    SummaryID          string
    OriginalMessageIDs []string
    SummaryContent     string
    TokenCount         int
    CompactedAt        time.Time
    Success            bool
    Error              error
}
```

#### 2. Compactor Implementation

```go
// pkg/ai/session/compactor_impl.go

type DefaultCompactor struct {
    sessionDB   *SessionDatabase
    aiFactory   ai.Factory
    logger      *zap.Logger
}

func NewCompactor(db *SessionDatabase, factory ai.Factory, logger *zap.Logger) Compactor {
    return &DefaultCompactor{
        sessionDB: db,
        aiFactory: factory,
        logger:    logger,
    }
}

func (c *DefaultCompactor) ShouldCompact(messages []Message, config CompactConfig) (bool, CompactPlan) {
    if !config.Enabled {
        return false, CompactPlan{}
    }

    totalMessages := len(messages)

    // Check against max_history_messages
    maxMessages := c.getMaxMessages() // from session config
    if maxMessages == 0 {
        return false, CompactPlan{} // No limit set
    }

    threshold := int(float64(maxMessages) * config.TriggerThreshold)
    if totalMessages < threshold {
        return false, CompactPlan{} // Not at threshold yet
    }

    // Calculate compaction plan
    numToCompact := int(float64(totalMessages) * config.CompactRatio)

    // Ensure we preserve recent messages
    if totalMessages - numToCompact < config.PreserveRecent {
        numToCompact = totalMessages - config.PreserveRecent
    }

    if numToCompact <= 0 {
        return false, CompactPlan{} // Not enough messages to compact
    }

    plan := CompactPlan{
        TotalMessages:     totalMessages,
        MessagesToCompact: messages[:numToCompact],
        MessagesToKeep:    messages[numToCompact:],
        Reason:            fmt.Sprintf("Reached %d%% capacity (%d/%d messages)",
                                       int(config.TriggerThreshold*100),
                                       totalMessages, maxMessages),
    }

    return true, plan
}

func (c *DefaultCompactor) Compact(ctx context.Context, plan CompactPlan) (*CompactResult, error) {
    // 1. Generate summary using AI
    summary, err := c.generateSummary(ctx, plan.MessagesToCompact)
    if err != nil {
        return nil, fmt.Errorf("failed to generate summary: %w", err)
    }

    // 2. Store summary in database
    summaryID, err := c.sessionDB.StoreSummary(plan.SessionID, summary, plan.MessagesToCompact)
    if err != nil {
        return nil, fmt.Errorf("failed to store summary: %w", err)
    }

    // 3. Mark original messages as archived
    originalIDs := extractMessageIDs(plan.MessagesToCompact)
    err = c.sessionDB.ArchiveMessages(originalIDs)
    if err != nil {
        return nil, fmt.Errorf("failed to archive messages: %w", err)
    }

    // 4. Log compaction event
    c.logger.Info("Session compacted",
        zap.String("session_id", plan.SessionID),
        zap.Int("messages_compacted", len(plan.MessagesToCompact)),
        zap.Int("tokens_saved", plan.EstimatedSavings),
        zap.String("summary_id", summaryID),
    )

    return &CompactResult{
        SummaryID:          summaryID,
        OriginalMessageIDs: originalIDs,
        SummaryContent:     summary.Content,
        TokenCount:         summary.TokenCount,
        CompactedAt:        time.Now(),
        Success:            true,
    }, nil
}

func (c *DefaultCompactor) generateSummary(ctx context.Context, messages []Message) (*Summary, error) {
    // Build summarization prompt
    prompt := c.buildSummarizationPrompt(messages)

    // Get AI client for summarization
    client, err := c.aiFactory.GetClient(c.config.SummaryProvider)
    if err != nil {
        return nil, err
    }

    // Generate summary
    response, err := client.Complete(ctx, &ai.CompletionRequest{
        Messages: []ai.Message{
            {Role: "user", Content: prompt},
        },
        MaxTokens: c.config.SummaryMaxTokens,
        Model:     c.config.SummaryModel,
    })
    if err != nil {
        return nil, err
    }

    return &Summary{
        Content:    response.Content,
        TokenCount: response.Usage.CompletionTokens,
    }, nil
}

func (c *DefaultCompactor) buildSummarizationPrompt(messages []Message) string {
    // Construct conversation to summarize
    var conversation strings.Builder
    for i, msg := range messages {
        conversation.WriteString(fmt.Sprintf("[Message %d] %s: %s\n\n",
                                              i+1, msg.Role, msg.Content))
    }

    return fmt.Sprintf(`You are summarizing a conversation about infrastructure management with Atmos.

IMPORTANT: Your summary will be used as context for future questions, so preserve:
- Infrastructure decisions and their reasoning (e.g., "Chose CIDR 10.0.0.0/16 for growth")
- Component configurations discussed (VPC, RDS, EKS, etc.)
- Security decisions and compliance requirements
- Architectural patterns and dependencies
- Any validation errors found and how they were resolved
- Key file paths and component names mentioned

DO NOT include:
- Pleasantries and conversational filler
- Repeated acknowledgments
- Step-by-step process details (focus on outcomes)

Organize the summary by topic (e.g., VPC Configuration, Security, Components) for easy reference.

CONVERSATION TO SUMMARIZE:
%s

Generate a concise but comprehensive summary in 150-300 words:`,
        conversation.String())
}
```

#### 3. Session Manager Integration

```go
// pkg/ai/session/manager.go

type Manager struct {
    db         *SessionDatabase
    compactor  Compactor
    config     *SessionConfig
    logger     *zap.Logger
}

func (m *Manager) GetMessagesForProvider(ctx context.Context, sessionID, providerName string) ([]Message, error) {
    // 1. Load all active messages (not archived)
    messages, err := m.db.LoadActiveMessages(sessionID, providerName)
    if err != nil {
        return nil, err
    }

    // 2. Load any summaries for this session
    summaries, err := m.db.LoadSummaries(sessionID)
    if err != nil {
        return nil, err
    }

    // 3. Combine summaries + active messages in chronological order
    allMessages := m.combineMessagesAndSummaries(summaries, messages)

    // 4. Check if compaction is needed
    if shouldCompact, plan := m.compactor.ShouldCompact(allMessages, m.config.AutoCompact); shouldCompact {
        m.logger.Debug("Auto-compact triggered",
            zap.String("session_id", sessionID),
            zap.String("reason", plan.Reason),
        )

        result, err := m.compactor.Compact(ctx, plan)
        if err != nil {
            m.logger.Error("Compaction failed", zap.Error(err))
            // Continue without compaction on error
        } else {
            m.logger.Info("Compaction successful",
                zap.String("summary_id", result.SummaryID),
                zap.Int("tokens_saved", result.TokenCount),
            )

            // Reload messages with new summary
            messages, _ = m.db.LoadActiveMessages(sessionID, providerName)
            summaries, _ = m.db.LoadSummaries(sessionID)
            allMessages = m.combineMessagesAndSummaries(summaries, messages)
        }
    }

    // 5. Apply any remaining truncation limits
    allMessages = m.applyTruncationLimits(allMessages)

    return allMessages, nil
}

func (m *Manager) combineMessagesAndSummaries(summaries []Summary, messages []Message) []Message {
    var combined []Message

    // Add summaries as special "assistant" messages
    for _, summary := range summaries {
        combined = append(combined, Message{
            ID:         summary.ID,
            Role:       "assistant",
            Content:    m.formatSummaryContent(summary),
            CreatedAt:  summary.CompactedAt,
            IsSummary:  true,
        })
    }

    // Add active messages
    combined = append(combined, messages...)

    // Sort by timestamp
    sort.Slice(combined, func(i, j int) bool {
        return combined[i].CreatedAt.Before(combined[j].CreatedAt)
    })

    return combined
}

func (m *Manager) formatSummaryContent(summary Summary) string {
    if m.config.AutoCompact.ShowSummaryMarkers {
        return fmt.Sprintf(`[SUMMARY: %s]

%s

[END SUMMARY]`, summary.MessageRange, summary.Content)
    }
    return summary.Content
}
```

#### 4. Database Schema Updates

```sql
-- New table for storing compaction summaries
CREATE TABLE IF NOT EXISTS message_summaries (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    original_message_ids TEXT NOT NULL,    -- JSON array: ["msg1", "msg2", ...]
    message_range TEXT NOT NULL,           -- Human-readable: "Messages 1-20"
    summary_content TEXT NOT NULL,
    token_count INTEGER NOT NULL,
    compacted_at TIMESTAMP NOT NULL,
    FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_summaries_session
    ON message_summaries(session_id);

-- Add archived flag to messages table
ALTER TABLE messages ADD COLUMN archived BOOLEAN DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_messages_archived
    ON messages(session_id, archived);

-- Migration function for existing databases
-- Sets all existing messages to archived=0
UPDATE messages SET archived = 0 WHERE archived IS NULL;
```

#### 5. Testing Utilities

```go
// pkg/ai/session/compactor_test.go

func TestCompactor_ShouldCompact(t *testing.T) {
    tests := []struct {
        name           string
        messageCount   int
        maxMessages    int
        threshold      float64
        expectCompact  bool
    }{
        {
            name:          "below threshold",
            messageCount:  30,
            maxMessages:   50,
            threshold:     0.75,
            expectCompact: false,
        },
        {
            name:          "at threshold",
            messageCount:  38,
            maxMessages:   50,
            threshold:     0.75,
            expectCompact: true,
        },
        {
            name:          "above threshold",
            messageCount:  45,
            maxMessages:   50,
            threshold:     0.75,
            expectCompact: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            compactor := NewCompactor(nil, nil, testLogger)
            messages := generateTestMessages(tt.messageCount)
            config := CompactConfig{
                Enabled:          true,
                TriggerThreshold: tt.threshold,
                CompactRatio:     0.4,
                PreserveRecent:   10,
            }

            shouldCompact, _ := compactor.ShouldCompact(messages, config)
            assert.Equal(t, tt.expectCompact, shouldCompact)
        })
    }
}

func TestCompactor_GenerateSummary(t *testing.T) {
    // Test summary generation with mock AI provider
    mockClient := &MockAIClient{
        CompletionFunc: func(ctx context.Context, req *ai.CompletionRequest) (*ai.CompletionResponse, error) {
            // Verify summarization prompt is properly formatted
            assert.Contains(t, req.Messages[0].Content, "CONVERSATION TO SUMMARIZE")

            return &ai.CompletionResponse{
                Content: "VPC Migration: Chose 10.0.0.0/16 for production...",
                Usage: ai.Usage{
                    CompletionTokens: 85,
                },
            }, nil
        },
    }

    factory := &MockAIFactory{
        GetClientFunc: func(provider string) (ai.Client, error) {
            return mockClient, nil
        },
    }

    compactor := NewCompactor(testDB, factory, testLogger)
    messages := generateConversationAboutVPC()

    summary, err := compactor.generateSummary(context.Background(), messages)
    assert.NoError(t, err)
    assert.Contains(t, summary.Content, "VPC Migration")
    assert.Greater(t, summary.TokenCount, 0)
}
```

## Implementation Phases

### Phase 1: Core Infrastructure (Week 1-2)

**Goal**: Build foundational compaction system without AI summarization.

**Tasks**:
1. Create `message_summaries` table and migration
2. Add `archived` column to `messages` table
3. Implement `Compactor` interface
4. Implement `ShouldCompact` logic with threshold checking
5. Implement simple concatenation-based compaction (without AI)
6. Add database methods: `StoreSummary`, `ArchiveMessages`, `LoadSummaries`
7. Unit tests for threshold detection and message selection

**Deliverables**:
- Working compaction system with simple summaries
- Database schema updated
- Configuration schema defined
- Tests passing

**Success Criteria**:
- Compaction triggers at correct threshold
- Messages properly archived
- Summaries stored and retrieved correctly

### Phase 2: AI-Generated Summaries (Week 3)

**Goal**: Integrate AI to generate intelligent summaries.

**Tasks**:
1. Implement `generateSummary` with AI provider integration
2. Create summarization prompt template
3. Add `summary_provider` and `summary_model` configuration
4. Implement token counting for summaries
5. Add error handling and fallback to concatenation
6. Integration tests with mock AI provider
7. Test with all 7 AI providers

**Deliverables**:
- AI-generated summaries working
- Multi-provider support verified
- Fallback mechanism implemented

**Success Criteria**:
- Summaries preserve key infrastructure decisions
- Works with Anthropic, OpenAI, Gemini, Grok, Ollama, Bedrock, Azure
- Graceful degradation on AI failure

### Phase 3: Session Manager Integration (Week 4)

**Goal**: Integrate compactor into existing session management.

**Tasks**:
1. Modify `SessionManager.GetMessagesForProvider` to trigger compaction
2. Implement `combineMessagesAndSummaries` to merge history
3. Add compaction logging and telemetry
4. Add `show_summary_markers` configuration
5. Implement `compact_on_resume` feature
6. Integration tests with full chat flow
7. Performance testing with large sessions

**Deliverables**:
- Full end-to-end compaction in chat sessions
- Transparent user experience
- Logging and observability

**Success Criteria**:
- Users don't notice compaction happening
- Session history correctly includes summaries
- Performance acceptable for 100+ message sessions

### Phase 4: User Experience & Polish (Week 5)

**Goal**: Refine UX, add debugging tools, optimize performance.

**Tasks**:
1. Add `atmos ai sessions compact --session <name>` command for manual compaction
2. Implement `atmos ai sessions show --session <name> --show-summaries` to view compaction status
3. Add compaction statistics to session list view
4. Optimize database queries with proper indexes
5. Add cost estimation for summaries
6. Create user documentation
7. Add troubleshooting guide

**Deliverables**:
- CLI commands for manual control
- Debugging and inspection tools
- User documentation
- Troubleshooting guide

**Success Criteria**:
- Users can manually trigger compaction
- Users can inspect compaction history
- Documentation is clear and comprehensive

### Phase 5: Advanced Features (Week 6+, Optional)

**Goal**: Advanced compaction strategies and optimizations.

**Tasks** (Future Enhancements):
1. Smart compaction: identify and preserve important messages
2. Incremental compaction: compact in smaller chunks
3. Configurable summarization prompts
4. Cross-session compaction (archive old sessions to summaries)
5. Summary caching to avoid re-summarization
6. A/B testing different summarization strategies
7. User feedback mechanism on summary quality

**Deliverables**:
- Advanced compaction strategies
- Performance optimizations
- Quality improvements

## Testing Strategy

### Unit Tests

**Location**: `pkg/ai/session/compactor_test.go`

**Coverage**:
1. Threshold detection logic
2. Message selection (compact ratio, preserve recent)
3. Summary generation (with mock AI)
4. Database operations (store, archive, load)
5. Configuration parsing
6. Edge cases (empty sessions, single message, etc.)

**Target**: 80%+ code coverage

### Integration Tests

**Location**: `tests/ai_session_compaction_test.go`

**Scenarios**:
1. Full chat session reaching compaction threshold
2. Multi-provider sessions with compaction
3. Session resume with existing summaries
4. Compaction failure handling
5. Token limit interactions with compaction
6. Multiple compaction rounds in long session

### Performance Tests

**Benchmarks**:
1. Compaction time for various session sizes (50, 100, 200 messages)
2. Database query performance with archived messages
3. Memory usage during compaction
4. Summary generation latency

**Targets**:
- Compaction completes in <5 seconds for 100 messages
- Database queries remain <100ms
- Memory usage <50MB for compaction

### End-to-End Tests

**Test Cases**:
1. User creates session, has 60-message conversation, compaction triggers automatically
2. User resumes old session with summaries, asks about early conversation
3. User manually triggers compaction with CLI command
4. User inspects compaction history and statistics
5. AI successfully references summarized context in responses

### User Acceptance Testing

**Scenarios**:
1. Infrastructure engineer has multi-week VPC migration conversation
2. Security auditor conducts extended security review across environments
3. Team member resumes colleague's session and asks about earlier decisions
4. Developer troubleshoots complex Terraform issues over multiple days

**Success Metrics**:
- Users can recall decisions from 50+ messages ago
- Conversation feels natural (no interruptions)
- Summary content is accurate and useful
- Cost impact is acceptable (<10% increase in API usage)

## Success Metrics

### Quantitative Metrics

1. **Context Retention**:
   - Measure: AI can answer questions about decisions from >50 messages ago
   - Target: >80% accuracy on historical questions

2. **User Satisfaction**:
   - Measure: Survey users on conversation continuity
   - Target: >90% report positive experience

3. **Token Efficiency**:
   - Measure: Token usage with vs. without compaction
   - Target: <20% increase in total tokens (summaries are smaller than originals)

4. **Compaction Performance**:
   - Measure: Time to compact 100 messages
   - Target: <5 seconds

5. **Adoption Rate**:
   - Measure: % of sessions with auto-compact enabled
   - Target: >30% within 3 months of release

### Qualitative Metrics

1. **Conversation Quality**: AI maintains context across compaction boundaries
2. **User Transparency**: Users don't notice compaction happening
3. **Summary Accuracy**: Summaries preserve important technical decisions
4. **Error Recovery**: Graceful handling of summarization failures

### Monitoring

**Telemetry Points**:
```go
// Log compaction events
telemetry.LogCompaction(sessionID, messagesCompacted, tokensSaved, duration)

// Track summary quality (tokens used vs. original)
telemetry.LogSummaryEfficiency(originalTokens, summaryTokens, compressionRatio)

// Monitor AI provider performance for summarization
telemetry.LogSummarizationLatency(provider, model, latency)

// Track user interactions with compacted history
telemetry.LogHistoricalQuestionAccuracy(sessionID, messageAge, answerQuality)
```

## Security and Privacy

### Data Handling

**What Gets Summarized**:
- User questions and AI responses from the session
- Infrastructure configurations, component names, stack details
- Potentially sensitive: AWS account IDs, CIDR blocks, resource names

**Where Summaries Are Generated**:
- Cloud providers (Anthropic, OpenAI, Google, xAI): Data sent to provider's API
- Local (Ollama): Data never leaves user's machine
- Enterprise (Bedrock, Azure): Data stays within user's cloud environment

**Privacy Considerations**:
1. **Summaries contain same data as original messages**: If original conversation had sensitive info, summary will too
2. **Summary generation uses AI provider**: Same privacy implications as normal chat
3. **Summaries stored locally**: In SQLite database on user's machine
4. **No external transmission**: Summaries not sent anywhere except during generation

### User Controls

**Opt-In by Default**:
```yaml
auto_compact:
  enabled: false  # Users must explicitly enable
```

**Provider Choice for Summaries**:
```yaml
auto_compact:
  summary_provider: "ollama"  # Use local provider for privacy
```

**Summary Inspection**:
```bash
# Users can view summaries before they're used
atmos ai sessions show --session vpc-migration --show-summaries
```

**Manual Deletion**:
```bash
# Delete specific summary
atmos ai sessions delete-summary --summary-id <id>

# Delete entire session including summaries
atmos ai sessions clean --older-than 0d
```

### Compliance

**Data Residency**:
- Summaries generated via Bedrock/Azure respect regional deployment
- Ollama keeps all data on-premises
- Cloud providers follow their standard data handling policies

**Audit Trail**:
- All compaction events logged
- Summary generation timestamps recorded
- Original message IDs tracked for audit purposes

**GDPR/Privacy Regulations**:
- Summaries are part of local session data
- Users can delete all session data including summaries
- No third-party data sharing beyond normal AI provider usage

## Migration and Backward Compatibility

### Database Migration

**Migration Script**: `pkg/ai/session/migrations/006_auto_compact.sql`

```sql
-- Add message_summaries table
CREATE TABLE IF NOT EXISTS message_summaries (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    original_message_ids TEXT NOT NULL,
    message_range TEXT NOT NULL,
    summary_content TEXT NOT NULL,
    token_count INTEGER NOT NULL,
    compacted_at TIMESTAMP NOT NULL,
    FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_summaries_session
    ON message_summaries(session_id);

-- Add archived column to messages
ALTER TABLE messages ADD COLUMN archived BOOLEAN DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_messages_archived
    ON messages(session_id, archived);

-- Set all existing messages to not archived
UPDATE messages SET archived = 0 WHERE archived IS NULL;
```

**Migration Safety**:
- Non-destructive: No existing data deleted
- Backward compatible: Old sessions continue working
- Automatic: Runs on first Atmos start after upgrade

### Backward Compatibility

**Existing Configurations**:
```yaml
# Old config continues working exactly as before
settings:
  ai:
    sessions:
      max_history_messages: 50
      max_history_tokens: 8000
```

**No Breaking Changes**:
- Auto-compact disabled by default
- Existing truncation logic unchanged
- Session database schema additions only (no modifications)
- All existing CLI commands work identically

**Upgrade Path**:
1. Upgrade Atmos to new version
2. Database migration runs automatically
3. Existing sessions continue with truncation
4. Users can enable auto-compact in config
5. New sessions benefit from compaction

### Rollback Plan

**If Issues Occur**:

1. **Disable Feature**:
   ```yaml
   auto_compact:
     enabled: false
   ```

2. **Database Rollback** (if necessary):
   ```sql
   -- Remove summaries table
   DROP TABLE IF EXISTS message_summaries;

   -- Remove archived column (if SQLite version supports)
   -- Otherwise, it's harmless to leave it
   ```

3. **Code Rollback**: Revert to previous Atmos version
   - Sessions still work (extra tables ignored)
   - No data loss

## Documentation Requirements

### User Documentation

**New Pages**:

1. **`website/docs/ai/session-auto-compact.mdx`**:
   - Feature overview
   - Configuration guide
   - How it works (with examples)
   - Cost considerations
   - Privacy implications
   - Troubleshooting

2. **Update `website/docs/ai/sessions.mdx`**:
   - Add auto-compact section
   - Link to detailed documentation
   - Show example configurations

3. **Update `website/docs/ai/configuration.mdx`**:
   - Add auto-compact configuration options
   - Explain interaction with existing limits

**CLI Help**:

```bash
atmos ai sessions compact --help
atmos ai sessions show --help
```

### Technical Documentation

**PRD**: This document

**Architecture Docs**:
- `docs/architecture/ai-session-compaction.md`
- Sequence diagrams for compaction flow
- Database schema documentation

**API Docs**:
- Godoc comments for all exported types and functions
- Example usage in comments

### Blog Post (Optional)

**Title**: "Introducing Session Auto-Compact: Extended AI Conversations for Infrastructure Management"

**Content**:
- Why we built this
- How it works (simplified)
- Example use case (VPC migration)
- Configuration guide
- Privacy and cost considerations

## Open Questions

### For Discussion

1. **Default Threshold**: Should default be 0.75 (75%) or higher/lower?
   - Higher = fewer compactions, more risk of hitting limits
   - Lower = more compactions, more summary cost

2. **Summary Quality Validation**: How do we measure if summaries are "good enough"?
   - User feedback mechanism?
   - Automated quality scoring?
   - A/B testing different prompts?

3. **Cost vs. Quality**: Should we default to cheaper models for summaries?
   - Pro: Lower cost, faster summaries
   - Con: Lower quality summaries, potential context loss

4. **Multi-Round Compaction**: What happens when summaries themselves need compaction?
   - Summarize summaries?
   - Hard limit on session length?
   - Warn user to start new session?

5. **Summary Format**: Should summaries be formatted specially for AI consumption?
   - Structured YAML/JSON?
   - Markdown with sections?
   - Plain prose?

6. **User Notification**: Should users be notified when compaction occurs?
   - Silent by default?
   - Optional notification?
   - Always visible in UI?

### Technical Decisions Needed

1. **Database Performance**: Do we need query optimization for 1000+ message sessions?
2. **Concurrency**: Can multiple compactions run simultaneously for different sessions?
3. **Error Handling**: What happens if summarization fails mid-conversation?
4. **Caching**: Should we cache summaries to avoid regenerating?
5. **Provider Fallback**: If summary provider fails, try another?

## Risks and Mitigations

### Risk 1: Poor Summary Quality

**Risk**: AI-generated summaries lose critical context.

**Likelihood**: Medium
**Impact**: High

**Mitigation**:
1. Carefully crafted summarization prompts emphasizing infrastructure decisions
2. Preserve recent messages verbatim (last 10+ messages)
3. Allow manual compaction review before it takes effect
4. Add `show_summary_markers` to visualize what was summarized
5. User feedback mechanism to report poor summaries

### Risk 2: Performance Impact

**Risk**: Compaction slows down chat experience.

**Likelihood**: Low
**Impact**: Medium

**Mitigation**:
1. Async compaction (don't block user messages)
2. Trigger compaction between messages, not during responses
3. Performance benchmarks and optimization
4. Use faster models for summarization (Haiku vs. Sonnet)

### Risk 3: Cost Increase

**Risk**: Summaries significantly increase API costs.

**Likelihood**: Medium
**Impact**: Medium

**Mitigation**:
1. Use cheaper models for summaries by default
2. Make feature opt-in (disabled by default)
3. Document cost implications clearly
4. Allow local summarization via Ollama
5. Token usage monitoring and limits

### Risk 4: Complexity for Users

**Risk**: Configuration is too complex, users don't understand.

**Likelihood**: Medium
**Impact**: Low

**Mitigation**:
1. Sensible defaults that work for most users
2. Clear documentation with examples
3. Simple on/off toggle for basic use
4. Advanced options for power users
5. Troubleshooting guide

### Risk 5: Database Migration Failures

**Risk**: Database migration fails on some systems.

**Likelihood**: Low
**Impact**: High

**Mitigation**:
1. Extensive testing across platforms (macOS, Linux, Windows)
2. Non-destructive migrations (additive only)
3. Automatic rollback on migration failure
4. Clear error messages with resolution steps
5. Manual migration option via CLI

## Future Enhancements (Post-MVP)

### Phase 2 Features

1. **Smart Message Selection**: ML-based identification of important messages to preserve
2. **Incremental Compaction**: Compact in smaller chunks rather than all-at-once
3. **Custom Summarization Prompts**: User-defined templates for domain-specific summaries
4. **Multi-Level Compaction**: Summarize summaries for ultra-long sessions
5. **Session Forking**: Create new session from specific point in compacted session

### Advanced Capabilities

1. **Cross-Session Memory**: Build knowledge graph across all sessions
2. **Semantic Search**: Search within compacted history by meaning, not just keywords
3. **Summary Regeneration**: Regenerate summaries with better prompts/models
4. **Export/Import**: Share compacted sessions with team members
5. **Analytics**: Session insights (topics discussed, decisions made, components modified)

### Integration Ideas

1. **Git Integration**: Include commit messages and PR context in summaries
2. **Terraform State**: Reference state changes in summaries
3. **Compliance Logging**: Audit trail of all infrastructure decisions
4. **Team Collaboration**: Shared session summaries across team members

## Success Criteria for Launch

### Must-Have (MVP)

- [ ] Auto-compact triggers at configured threshold
- [ ] AI-generated summaries preserve infrastructure decisions
- [ ] Works with all 7 AI providers
- [ ] Backward compatible with existing sessions
- [ ] Database migration succeeds on all platforms
- [ ] Performance: compaction completes in <5s for 100 messages
- [ ] Documentation complete (user guide, API docs)
- [ ] 80%+ test coverage
- [ ] Zero breaking changes to existing functionality

### Should-Have (Post-MVP, within 2 weeks of launch)

- [ ] Manual compaction CLI command
- [ ] Session inspection tools
- [ ] Cost estimation and monitoring
- [ ] Troubleshooting guide
- [ ] Blog post announcing feature

### Could-Have (Future)

- [ ] Smart message selection
- [ ] Incremental compaction
- [ ] Custom summarization prompts
- [ ] Cross-session analytics

## Conclusion

Auto-compact addresses a critical limitation in Atmos AI's session management: the inability to maintain extended conversations without losing context. By intelligently summarizing older messages rather than discarding them, we enable infrastructure engineers to have multi-day/week conversations about complex migrations, security audits, and architecture reviews.

**Key Benefits**:
1. Extended conversations without context loss
2. Automatic rate limit management
3. Better user experience (no manual session management)
4. Preservation of infrastructure decisions and reasoning
5. Cost-effective context management

**Implementation Risk**: Low to Medium
- Builds on existing session infrastructure
- Non-breaking, opt-in feature
- Well-defined scope with clear phases

**User Impact**: High
- Directly addresses user pain points
- Enables new use cases (multi-week migrations)
- Minimal configuration needed for basic use

**Recommendation**: Proceed with implementation following the phased approach outlined above.
