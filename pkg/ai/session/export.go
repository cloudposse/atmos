package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

// ExportSession exports a session to a checkpoint file.
func (m *Manager) ExportSession(ctx context.Context, sessionID string, outputPath string, opts ExportOptions) error {
	// Get session.
	session, err := m.storage.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("%w: failed to get session: %v", errUtils.ErrAISessionNotFound, err)
	}

	// Get all messages (limit 0 = no limit).
	messages, err := m.storage.GetMessages(ctx, sessionID, 0)
	if err != nil {
		return fmt.Errorf("failed to get messages: %w", err)
	}

	// Build checkpoint.
	checkpoint := m.buildCheckpoint(session, messages, opts)

	// Determine format from file extension if not specified.
	format := opts.Format
	if format == "" {
		format = detectFormatFromPath(outputPath)
	}

	// Export based on format.
	switch format {
	case "json":
		return exportJSON(checkpoint, outputPath)
	case "yaml", "yml":
		return exportYAML(checkpoint, outputPath)
	case "markdown", "md":
		return exportMarkdown(checkpoint, outputPath)
	default:
		return fmt.Errorf("unsupported export format: %s (supported: json, yaml, markdown)", format)
	}
}

// ExportSessionByName exports a session by name.
func (m *Manager) ExportSessionByName(ctx context.Context, sessionName string, outputPath string, opts ExportOptions) error {
	// Get session by name.
	session, err := m.storage.GetSessionByName(ctx, m.projectPath, sessionName)
	if err != nil {
		return fmt.Errorf("%w: failed to get session: %v", errUtils.ErrAISessionNotFound, err)
	}

	return m.ExportSession(ctx, session.ID, outputPath, opts)
}

// buildCheckpoint builds a checkpoint from a session and messages.
func (m *Manager) buildCheckpoint(session *Session, messages []*Message, opts ExportOptions) *Checkpoint {
	checkpoint := &Checkpoint{
		Version:    CheckpointVersion,
		ExportedAt: time.Now(),
		Session: CheckpointSession{
			Name:        session.Name,
			Provider:    session.Provider,
			Model:       session.Model,
			Agent:       session.Agent,
			ProjectPath: session.ProjectPath,
			CreatedAt:   session.CreatedAt,
			UpdatedAt:   session.UpdatedAt,
		},
		Messages: make([]CheckpointMessage, 0, len(messages)),
		Statistics: CheckpointStatistics{
			MessageCount: len(messages),
		},
	}

	// Add metadata if requested.
	if opts.IncludeMetadata && session.Metadata != nil {
		checkpoint.Session.Metadata = session.Metadata
	}

	// Convert messages.
	var userCount, assistantCount int
	for _, msg := range messages {
		checkpoint.Messages = append(checkpoint.Messages, CheckpointMessage{
			Role:      msg.Role,
			Content:   msg.Content,
			CreatedAt: msg.CreatedAt,
			Archived:  msg.Archived,
		})

		// Count message types.
		switch msg.Role {
		case "user":
			userCount++
		case "assistant":
			assistantCount++
		}
	}

	checkpoint.Statistics.UserMessages = userCount
	checkpoint.Statistics.AssistantMessages = assistantCount

	// Add context if requested.
	if opts.IncludeContext {
		checkpoint.Context = m.extractContext()
	}

	// Get current user for exported_by field.
	if user := os.Getenv("USER"); user != "" {
		checkpoint.ExportedBy = user
	}

	return checkpoint
}

// extractContext extracts project context for the checkpoint.
func (m *Manager) extractContext() *CheckpointContext {
	context := &CheckpointContext{}

	// Try to load project memory (ATMOS.md).
	if m.atmosConfig != nil && m.atmosConfig.Settings.AI.Memory.Enabled {
		// TODO: Load ATMOS.md content from memory manager.
		// For now, leave empty - will be enhanced in future.
	}

	// Get working directory.
	if wd, err := os.Getwd(); err == nil {
		context.WorkingDirectory = wd
	}

	return context
}

// exportJSON exports checkpoint as JSON.
func exportJSON(checkpoint *Checkpoint, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(checkpoint); err != nil {
		return fmt.Errorf("failed to encode checkpoint: %w", err)
	}

	return nil
}

// exportYAML exports checkpoint as YAML.
func exportYAML(checkpoint *Checkpoint, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(2)

	if err := encoder.Encode(checkpoint); err != nil {
		return fmt.Errorf("failed to encode checkpoint: %w", err)
	}

	return nil
}

// exportMarkdown exports checkpoint as Markdown (human-readable).
func exportMarkdown(checkpoint *Checkpoint, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Write markdown header.
	fmt.Fprintf(file, "# Atmos AI Session: %s\n\n", checkpoint.Session.Name)
	fmt.Fprintf(file, "**Exported:** %s\n", checkpoint.ExportedAt.Format(time.RFC3339))
	if checkpoint.ExportedBy != "" {
		fmt.Fprintf(file, "**Exported By:** %s\n", checkpoint.ExportedBy)
	}
	fmt.Fprintf(file, "**Provider:** %s\n", checkpoint.Session.Provider)
	fmt.Fprintf(file, "**Model:** %s\n", checkpoint.Session.Model)
	if checkpoint.Session.Agent != "" {
		fmt.Fprintf(file, "**Agent:** %s\n", checkpoint.Session.Agent)
	}
	fmt.Fprintf(file, "**Created:** %s\n", checkpoint.Session.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(file, "**Messages:** %d\n\n", checkpoint.Statistics.MessageCount)

	// Write statistics.
	fmt.Fprintf(file, "## Statistics\n\n")
	fmt.Fprintf(file, "- Total Messages: %d\n", checkpoint.Statistics.MessageCount)
	fmt.Fprintf(file, "- User Messages: %d\n", checkpoint.Statistics.UserMessages)
	fmt.Fprintf(file, "- Assistant Messages: %d\n", checkpoint.Statistics.AssistantMessages)
	if checkpoint.Statistics.TotalTokens > 0 {
		fmt.Fprintf(file, "- Total Tokens: %d\n", checkpoint.Statistics.TotalTokens)
	}
	if checkpoint.Statistics.ToolCalls > 0 {
		fmt.Fprintf(file, "- Tool Calls: %d\n", checkpoint.Statistics.ToolCalls)
	}
	fmt.Fprintf(file, "\n")

	// Write context if present.
	if checkpoint.Context != nil {
		fmt.Fprintf(file, "## Context\n\n")
		if checkpoint.Context.WorkingDirectory != "" {
			fmt.Fprintf(file, "**Working Directory:** `%s`\n\n", checkpoint.Context.WorkingDirectory)
		}
		if checkpoint.Context.ProjectMemory != "" {
			fmt.Fprintf(file, "### Project Memory\n\n```\n%s\n```\n\n", checkpoint.Context.ProjectMemory)
		}
		if len(checkpoint.Context.FilesAccessed) > 0 {
			fmt.Fprintf(file, "### Files Accessed\n\n")
			for _, f := range checkpoint.Context.FilesAccessed {
				fmt.Fprintf(file, "- `%s`\n", f)
			}
			fmt.Fprintf(file, "\n")
		}
	}

	// Write conversation.
	fmt.Fprintf(file, "## Conversation\n\n")
	for i, msg := range checkpoint.Messages {
		roleLabel := strings.ToUpper(msg.Role)
		if msg.Archived {
			roleLabel += " (COMPACTED)"
		}

		fmt.Fprintf(file, "### Message %d - %s\n", i+1, roleLabel)
		fmt.Fprintf(file, "*%s*\n\n", msg.CreatedAt.Format(time.RFC3339))
		fmt.Fprintf(file, "%s\n\n", msg.Content)
		fmt.Fprintf(file, "---\n\n")
	}

	return nil
}

// detectFormatFromPath detects export format from file extension.
func detectFormatFromPath(path string) string {
	if strings.HasSuffix(path, ".json") {
		return "json"
	}
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		return "yaml"
	}
	if strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".markdown") {
		return "markdown"
	}

	// Default to JSON.
	return "json"
}
