package ai

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/session"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	hoursPerDay  = 24
	daysPerWeek  = 7
	daysPerMonth = 30
)

// aiSessionsCmd represents the ai sessions command.
var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Manage AI chat sessions",
	Long: `Manage persistent AI chat sessions.

Sessions allow you to save and resume conversations with the AI assistant.
Each session maintains its own message history and context.

Available operations:
- List all sessions
- Clean old sessions
- Resume sessions with 'atmos ai chat --session <name>'`,
	RunE: listSessionsCommand,
}

// aiSessionsListCmd lists all sessions.
var sessionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all AI chat sessions",
	Long: `List all available AI chat sessions.

Shows session details including:
- Session name
- Created and last updated timestamps
- Number of messages
- AI model and provider used

Example:
  atmos ai sessions list`,
	RunE: listSessionsCommand,
}

// aiSessionsCleanCmd cleans old sessions.
var sessionsCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean old AI chat sessions",
	Long: `Remove old AI chat sessions based on retention policy.

Sessions older than the specified duration will be permanently deleted.
Use this command to free up space and remove outdated conversations.

Examples:
  atmos ai sessions clean --older-than 30d   # Delete sessions older than 30 days
  atmos ai sessions clean --older-than 7d    # Delete sessions older than 7 days
  atmos ai sessions clean --older-than 24h   # Delete sessions older than 24 hours`,
	RunE: cleanSessionsCommand,
}

func init() {
	// Add sessions command to ai command.
	aiCmd.AddCommand(sessionsCmd)

	// Add subcommands to sessions command.
	sessionsCmd.AddCommand(sessionsListCmd)
	sessionsCmd.AddCommand(sessionsCleanCmd)

	// Add flags for clean command.
	sessionsCleanCmd.Flags().String("older-than", "30d", "Delete sessions older than this duration (e.g., 30d, 7d, 24h)")
}

// initSessionManager initializes and validates session management.
func initSessionManager() (*session.Manager, func(), error) {
	// Initialize configuration.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, nil, err
	}

	// Check if AI is enabled.
	if !isAIEnabled(&atmosConfig) {
		return nil, nil, fmt.Errorf("%w: Set 'settings.ai.enabled: true' in atmos.yaml", errUtils.ErrAINotEnabled)
	}

	// Check if sessions are enabled.
	if !atmosConfig.Settings.AI.Sessions.Enabled {
		return nil, nil, fmt.Errorf("%w. Set 'settings.ai.sessions.enabled: true' in atmos.yaml", errUtils.ErrAISessionsNotEnabled)
	}

	// Initialize session storage.
	storagePath := getSessionStoragePath(&atmosConfig)
	storage, err := session.NewSQLiteStorage(storagePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize session storage: %w", err)
	}

	// Create session manager.
	manager := session.NewManager(storage, atmosConfig.BasePath, atmosConfig.Settings.AI.Sessions.MaxSessions, &atmosConfig)

	// Return cleanup function.
	cleanup := func() {
		storage.Close()
	}

	return manager, cleanup, nil
}

// listSessionsCommand lists all sessions.
func listSessionsCommand(cmd *cobra.Command, args []string) error {
	log.Debug("Listing AI sessions")

	manager, cleanup, err := initSessionManager()
	if err != nil {
		return err
	}
	defer cleanup()

	// Get all sessions.
	ctx := context.Background()
	sessions, err := manager.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	// Display sessions.
	if len(sessions) == 0 {
		u.PrintMessage("No sessions found.")
		u.PrintMessage("\nStart a new session with: atmos ai chat --session <name>")
		return nil
	}

	u.PrintMessage(fmt.Sprintf("Found %d session(s):\n", len(sessions)))

	for _, sess := range sessions {
		u.PrintMessage(fmt.Sprintf("Name: %s", sess.Name))
		u.PrintMessage(fmt.Sprintf("  ID: %s", sess.ID))
		u.PrintMessage(fmt.Sprintf("  Created: %s", sess.CreatedAt.Format("2006-01-02 15:04:05")))
		u.PrintMessage(fmt.Sprintf("  Updated: %s", sess.UpdatedAt.Format("2006-01-02 15:04:05")))

		// Get message count.
		count, err := manager.GetMessageCount(ctx, sess.ID)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to get message count for session %s: %v", sess.ID, err))
			count = 0
		}
		u.PrintMessage(fmt.Sprintf("  Messages: %d", count))
		u.PrintMessage(fmt.Sprintf("  Model: %s", sess.Model))
		u.PrintMessage(fmt.Sprintf("  Provider: %s", sess.Provider))
		u.PrintMessage("")
	}

	u.PrintMessage("Resume a session with: atmos ai chat --session <name>")

	return nil
}

// cleanSessionsCommand cleans old sessions.
func cleanSessionsCommand(cmd *cobra.Command, args []string) error {
	// Get older-than flag.
	olderThanStr, err := cmd.Flags().GetString("older-than")
	if err != nil {
		return fmt.Errorf("failed to get older-than flag: %w", err)
	}

	// Parse duration.
	retentionDays, err := parseDuration(olderThanStr)
	if err != nil {
		return fmt.Errorf("invalid duration format '%s': %w", olderThanStr, err)
	}

	log.Debug(fmt.Sprintf("Cleaning sessions older than %d days", retentionDays))

	manager, cleanup, err := initSessionManager()
	if err != nil {
		return err
	}
	defer cleanup()

	// Clean old sessions.
	ctx := context.Background()
	count, err := manager.CleanOldSessions(ctx, retentionDays)
	if err != nil {
		return fmt.Errorf("failed to clean sessions: %w", err)
	}

	if count == 0 {
		u.PrintMessage("No sessions to clean.")
	} else {
		u.PrintMessage(fmt.Sprintf("✅ Deleted %d session(s) older than %s", count, olderThanStr))
	}

	return nil
}

// parseDuration parses a duration string like "30d", "7d", "24h" into days.
func parseDuration(durationStr string) (int, error) {
	if durationStr == "" {
		return session.DefaultRetentionDays, nil
	}

	var value int
	var unit string

	// Parse the value and unit.
	n, err := fmt.Sscanf(durationStr, "%d%s", &value, &unit)
	if err != nil || n != 2 {
		return 0, fmt.Errorf("%w: format should be like '30d', '7d', or '24h'", errUtils.ErrAIInvalidDurationFormat)
	}

	switch unit {
	case "h":
		// Convert hours to days (round up).
		days := value / hoursPerDay
		if value%hoursPerDay != 0 {
			days++
		}
		return days, nil
	case "d":
		return value, nil
	case "w":
		return value * daysPerWeek, nil
	case "m":
		return value * daysPerMonth, nil
	default:
		return 0, fmt.Errorf("%w: '%s', use 'h' (hours), 'd' (days), 'w' (weeks), or 'm' (months)", errUtils.ErrAIUnsupportedDurationUnit, unit)
	}
}
