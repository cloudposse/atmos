package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Default ratio of oldest messages to compact (40%).
	defaultCompactRatio = 0.4
	// Default threshold ratio to trigger compaction (75% capacity).
	defaultTriggerThreshold = 0.75
	// Default minimum number of recent messages to always preserve.
	defaultMinMessages = 10
	// Maximum character length for message content in simple summaries.
	defaultMaxTokensPerMessage = 200
	// Default maximum number of tokens for AI-generated summaries.
	defaultSummaryMaxTokens = 2048
)

// Compactor handles intelligent conversation history compaction.
type Compactor interface {
	// ShouldCompact determines if compaction is needed based on current history.
	ShouldCompact(messages []*Message, maxMessages int, config *CompactConfig) (bool, *CompactPlan)

	// Compact performs the compaction operation.
	Compact(ctx context.Context, plan *CompactPlan, config *CompactConfig) (*CompactResult, error)
}

// DefaultCompactor implements Compactor with AI-powered summarization.
type DefaultCompactor struct {
	storage     Storage
	atmosConfig *schema.AtmosConfiguration
}

// NewCompactor creates a new compactor instance.
func NewCompactor(storage Storage, atmosConfig *schema.AtmosConfiguration) Compactor {
	return &DefaultCompactor{
		storage:     storage,
		atmosConfig: atmosConfig,
	}
}

// ShouldCompact determines if compaction is needed.
func (c *DefaultCompactor) ShouldCompact(messages []*Message, maxMessages int, config *CompactConfig) (bool, *CompactPlan) {
	// Auto-compact not enabled.
	if !config.Enabled {
		return false, nil
	}

	// No limit set - unlimited history.
	if maxMessages == 0 {
		return false, nil
	}

	totalMessages := len(messages)

	// Check if we've reached the trigger threshold.
	threshold := int(float64(maxMessages) * config.TriggerThreshold)
	if totalMessages < threshold {
		return false, nil // Not at threshold yet
	}

	// Calculate how many messages to compact.
	numToCompact := int(float64(totalMessages) * config.CompactRatio)

	// Ensure we preserve recent messages.
	if totalMessages-numToCompact < config.PreserveRecent {
		numToCompact = totalMessages - config.PreserveRecent
	}

	// Not enough messages to compact.
	if numToCompact <= 0 {
		return false, nil
	}

	// Build compaction plan.
	plan := &CompactPlan{
		TotalMessages:     totalMessages,
		MessagesToCompact: messages[:numToCompact],
		MessagesToKeep:    messages[numToCompact:],
		Reason: fmt.Sprintf("Reached %d%% capacity (%d/%d messages)",
			int(config.TriggerThreshold*100), totalMessages, maxMessages),
	}

	// Estimate token savings (rough approximation: ~1 token per 4 characters).
	originalTokens := 0
	for _, msg := range plan.MessagesToCompact {
		originalTokens += len(msg.Content) / 4
	}
	plan.EstimatedSavings = originalTokens

	return true, plan
}

// Compact performs the compaction operation with AI-powered summarization.
func (c *DefaultCompactor) Compact(ctx context.Context, plan *CompactPlan, config *CompactConfig) (*CompactResult, error) {
	if plan == nil {
		return nil, errUtils.ErrAICompactPlanNil
	}

	// Generate summary content.
	summaryContent := c.generateSummaryContent(ctx, plan.MessagesToCompact, config)

	// Extract message IDs.
	messageIDs := extractMessageIDs(plan.MessagesToCompact)

	// Build and store summary record.
	summary := c.buildSummary(plan, messageIDs, summaryContent)

	// Store summary in database.
	if err := c.storage.StoreSummary(ctx, summary); err != nil {
		return nil, fmt.Errorf("failed to store summary: %w", err)
	}

	// Archive original messages.
	if err := c.storage.ArchiveMessages(ctx, messageIDs); err != nil {
		return nil, fmt.Errorf("failed to archive messages: %w", err)
	}

	return &CompactResult{
		SummaryID:          summary.ID,
		OriginalMessageIDs: messageIDs,
		SummaryContent:     summaryContent,
		TokenCount:         summary.TokenCount,
		CompactedAt:        summary.CompactedAt,
		Success:            true,
	}, nil
}

// generateSummaryContent generates a summary using AI or simple concatenation.
func (c *DefaultCompactor) generateSummaryContent(ctx context.Context, messages []*Message, config *CompactConfig) string {
	if config.UseAISummary && c.atmosConfig != nil {
		if content, err := c.generateAISummary(ctx, messages, config); err == nil {
			return content
		}
	}
	return c.generateSimpleSummary(messages)
}

// extractMessageIDs extracts IDs from a slice of messages.
func extractMessageIDs(messages []*Message) []int64 {
	ids := make([]int64, len(messages))
	for i, msg := range messages {
		ids[i] = msg.ID
	}
	return ids
}

// buildSummary creates a Summary record from plan data.
func (c *DefaultCompactor) buildSummary(plan *CompactPlan, messageIDs []int64, content string) *Summary {
	messageRange := fmt.Sprintf("Messages %d-%d",
		plan.MessagesToCompact[0].ID,
		plan.MessagesToCompact[len(plan.MessagesToCompact)-1].ID)

	return &Summary{
		ID:                 uuid.New().String(),
		SessionID:          plan.SessionID,
		OriginalMessageIDs: messageIDs,
		MessageRange:       messageRange,
		SummaryContent:     content,
		TokenCount:         len(content) / 4, // Rough estimate.
		CompactedAt:        time.Now(),
	}
}

// generateAISummary creates an AI-powered summary of messages.
//
//nolint:unparam // config parameter reserved for future summary customization options
func (c *DefaultCompactor) generateAISummary(ctx context.Context, messages []*Message, config *CompactConfig) (string, error) {
	// Create AI client.
	client, err := ai.NewClient(c.atmosConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create AI client: %w", err)
	}

	// Build summarization prompt.
	prompt := c.buildSummarizationPrompt(messages)

	// Generate summary using AI.
	summary, err := client.SendMessage(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate AI summary: %w", err)
	}

	return summary, nil
}

// buildSummarizationPrompt creates the prompt for AI summarization.
func (c *DefaultCompactor) buildSummarizationPrompt(messages []*Message) string {
	var conversation strings.Builder

	// Build conversation transcript.
	for i, msg := range messages {
		var roleLabel string
		switch msg.Role {
		case RoleAssistant:
			roleLabel = "Assistant"
		case RoleSystem:
			roleLabel = "System"
		default:
			roleLabel = "User"
		}

		fmt.Fprintf(&conversation, "[Message %d] %s: %s\n\n",
			i+1, roleLabel, msg.Content)
	}

	// Create summarization prompt.
	prompt := fmt.Sprintf(`You are summarizing a conversation about infrastructure management with Atmos.

IMPORTANT: Your summary will be used as context for future questions, so preserve:
- Infrastructure decisions and their reasoning (e.g., "Chose CIDR 10.0.0.0/16 for growth capacity")
- Component configurations discussed (VPC, RDS, EKS, security groups, etc.)
- Security decisions and compliance requirements
- Architectural patterns and dependencies
- Stack names, component names, and environment names mentioned
- Any validation errors found and how they were resolved
- Key file paths, directory structures, and configuration values
- Terraform/Helmfile/component changes made

DO NOT include:
- Pleasantries and conversational filler
- Repeated acknowledgments
- Step-by-step process details (focus on outcomes and decisions)
- Tool execution details (focus on results)

Organize the summary by topic (e.g., "VPC Configuration", "Security", "Components") for easy reference.

CONVERSATION TO SUMMARIZE:
%s

Generate a concise but comprehensive summary in 150-400 words that captures all important infrastructure decisions, configurations, and context:`, conversation.String())

	return prompt
}

// generateSimpleSummary creates a basic summary by concatenating messages.
// Fallback implementation when AI summarization fails.
func (c *DefaultCompactor) generateSimpleSummary(messages []*Message) string {
	var summary strings.Builder

	summary.WriteString("SUMMARY OF EARLIER CONVERSATION:\n\n")

	for _, msg := range messages {
		// Format: [Role]: First defaultMaxTokensPerMessage chars of message.
		content := msg.Content
		if len(content) > defaultMaxTokensPerMessage {
			content = content[:defaultMaxTokensPerMessage] + "..."
		}

		fmt.Fprintf(&summary, "[%s]: %s\n\n", msg.Role, content)
	}

	fmt.Fprintf(&summary, "(Summarized %d messages)\n", len(messages))

	return summary.String()
}

// DefaultCompactConfig returns the default compaction configuration.
func DefaultCompactConfig() CompactConfig {
	return CompactConfig{
		Enabled:            false,                   // Opt-in by default
		TriggerThreshold:   defaultTriggerThreshold, // Trigger at 75% capacity
		CompactRatio:       defaultCompactRatio,     // Compact oldest 40%
		PreserveRecent:     defaultMinMessages,      // Always keep last 10 messages
		UseAISummary:       true,                    // Use AI (when implemented in Phase 2)
		SummaryMaxTokens:   defaultSummaryMaxTokens, // Max 2048 tokens for summary
		ShowSummaryMarkers: false,                   // Don't show markers by default
		CompactOnResume:    false,                   // Don't compact on resume by default
	}
}
