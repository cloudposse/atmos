package ai

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/session"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	// Handles flag parsing for the sessions clean command.
	cleanParser *flags.StandardParser
	// Handles flag parsing for the sessions export command.
	exportParser *flags.StandardParser
	// Handles flag parsing for the sessions import command.
	importParser *flags.StandardParser
)

const (
	hoursPerDay  = 24
	daysPerWeek  = 7
	daysPerMonth = 30
	contextFlag  = "context"
	outputFlag   = "output"
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

// aiSessionsExportCmd exports a session to a checkpoint file.
var sessionsExportCmd = &cobra.Command{
	Use:   "export <session-name>",
	Short: "Export an AI chat session to a checkpoint file",
	Long: `Export an AI chat session to a checkpoint file for backup or sharing.

The checkpoint file contains the complete session including:
- Session metadata (name, model, provider, timestamps)
- Complete message history
- Project context (optional)
- Statistics

Supports multiple formats: JSON (default), YAML, Markdown

Examples:
  atmos ai sessions export vpc-migration --output session.json
  atmos ai sessions export my-session --output backup.yaml --context
  atmos ai sessions export analysis --output report.md --format markdown`,
	Args: cobra.ExactArgs(1),
	RunE: exportSessionCommand,
}

// aiSessionsImportCmd imports a session from a checkpoint file.
var sessionsImportCmd = &cobra.Command{
	Use:   "import <checkpoint-file>",
	Short: "Import an AI chat session from a checkpoint file",
	Long: `Import an AI chat session from a checkpoint file.

Restores a session from a checkpoint file created with 'atmos ai sessions export'.
The imported session can be resumed with 'atmos ai chat --session <name>'.

Supports JSON and YAML checkpoint files.

Examples:
  atmos ai sessions import session.json
  atmos ai sessions import backup.yaml --name restored-session
  atmos ai sessions import session.json --overwrite`,
	Args: cobra.ExactArgs(1),
	RunE: importSessionCommand,
}

func init() {
	// Add sessions command to ai command.
	aiCmd.AddCommand(sessionsCmd)

	// Add subcommands to sessions command.
	sessionsCmd.AddCommand(sessionsListCmd)
	sessionsCmd.AddCommand(sessionsCleanCmd)
	sessionsCmd.AddCommand(sessionsExportCmd)
	sessionsCmd.AddCommand(sessionsImportCmd)

	// Create parser for clean command.
	cleanParser = flags.NewStandardParser(
		flags.WithStringFlag("older-than", "", "30d", "Delete sessions older than this duration (e.g., 30d, 7d, 24h)"),
		flags.WithEnvVars("older-than", "ATMOS_AI_SESSIONS_OLDER_THAN"),
	)
	cleanParser.RegisterFlags(sessionsCleanCmd)
	if err := cleanParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Create parser for export command.
	exportParser = flags.NewStandardParser(
		flags.WithStringFlag(outputFlag, "o", "", "Output file path (required)"),
		flags.WithStringFlag("format", "f", "", "Output format: json, yaml, markdown (auto-detected from file extension if not specified)"),
		flags.WithBoolFlag(contextFlag, "", false, "Include project context (ATMOS.md, files accessed)"),
		flags.WithBoolFlag("metadata", "", true, "Include session metadata"),
		flags.WithEnvVars(outputFlag, "ATMOS_AI_SESSIONS_OUTPUT"),
		flags.WithEnvVars("format", "ATMOS_AI_SESSIONS_FORMAT"),
		flags.WithEnvVars(contextFlag, "ATMOS_AI_SESSIONS_CONTEXT"),
		flags.WithEnvVars("metadata", "ATMOS_AI_SESSIONS_METADATA"),
	)
	exportParser.RegisterFlags(sessionsExportCmd)
	if err := exportParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	_ = sessionsExportCmd.MarkFlagRequired(outputFlag)

	// Create parser for import command.
	importParser = flags.NewStandardParser(
		flags.WithStringFlag("name", "n", "", "Name for the imported session (uses checkpoint name if not specified)"),
		flags.WithBoolFlag("overwrite", "", false, "Overwrite existing session with the same name"),
		flags.WithBoolFlag(contextFlag, "", true, "Include project context from checkpoint"),
		flags.WithEnvVars("name", "ATMOS_AI_SESSIONS_NAME"),
		flags.WithEnvVars("overwrite", "ATMOS_AI_SESSIONS_OVERWRITE"),
		flags.WithEnvVars(contextFlag, "ATMOS_AI_SESSIONS_CONTEXT"),
	)
	importParser.RegisterFlags(sessionsImportCmd)
	if err := importParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
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

// sessionsToListData converts sessions to the []map[string]any format used by the list abstraction.
func sessionsToListData(ctx context.Context, manager *session.Manager, sessions []*session.Session) []map[string]any {
	data := make([]map[string]any, 0, len(sessions))
	for _, sess := range sessions {
		count, countErr := manager.GetMessageCount(ctx, sess.ID)
		if countErr != nil {
			log.Warnf("Failed to get message count for session %s: %v", sess.ID, countErr)
			count = 0
		}
		data = append(data, map[string]any{
			"name":     sess.Name,
			"created":  sess.CreatedAt.Format("2006-01-02 15:04:05"),
			"updated":  sess.UpdatedAt.Format("2006-01-02 15:04:05"),
			"messages": count,
			"model":    sess.Model,
			"provider": sess.Provider,
		})
	}

	return data
}

// listSessionsCommand lists all sessions using the standard list abstraction.
func listSessionsCommand(cmd *cobra.Command, args []string) error {
	log.Debug("Listing AI sessions")

	manager, cleanup, err := initSessionManager()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	sessions, err := manager.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(sessions) == 0 {
		ui.Info("No sessions found. Start a new session with: atmos ai chat --session <name>")
		return nil
	}

	data := sessionsToListData(ctx, manager, sessions)

	columns := []column.Config{
		{Name: "Name", Value: "{{ .name }}"},
		{Name: "Created", Value: "{{ .created }}"},
		{Name: "Updated", Value: "{{ .updated }}"},
		{Name: "Messages", Value: "{{ .messages }}"},
		{Name: "Model", Value: "{{ .model }}"},
		{Name: "Provider", Value: "{{ .provider }}"},
	}

	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	sorters := []*listSort.Sorter{
		listSort.NewSorter("Name", listSort.Ascending),
	}

	r := renderer.New(nil, selector, sorters, format.FormatTable, "")

	return r.Render(data)
}

// cleanSessionsCommand cleans old sessions.
func cleanSessionsCommand(cmd *cobra.Command, args []string) error {
	// Bind parsed flags to Viper for precedence handling.
	v := viper.GetViper()
	if err := cleanParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Get older-than flag from Viper (supports CLI > ENV > config > defaults).
	olderThanStr := v.GetString("older-than")

	// Parse duration.
	retentionDays, err := parseDuration(olderThanStr)
	if err != nil {
		return fmt.Errorf("invalid duration format '%s': %w", olderThanStr, err)
	}

	log.Debugf("Cleaning sessions older than %d days", retentionDays)

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

// exportSessionCommand exports a session to a checkpoint file.
func exportSessionCommand(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Bind parsed flags to Viper for precedence handling.
	v := viper.GetViper()
	if err := exportParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Get flags from Viper (supports CLI > ENV > config > defaults).
	outputPath := v.GetString(outputFlag)
	format := v.GetString("format")
	includeContext := v.GetBool(contextFlag)
	includeMetadata := v.GetBool("metadata")

	log.Debugf("Exporting session '%s' to '%s'", sessionName, outputPath)

	manager, cleanup, err := initSessionManager()
	if err != nil {
		return err
	}
	defer cleanup()

	// Export session.
	ctx := context.Background()
	opts := session.ExportOptions{
		IncludeContext:  includeContext,
		IncludeMetadata: includeMetadata,
		Format:          format,
	}

	if err := manager.ExportSessionByName(ctx, sessionName, outputPath, opts); err != nil {
		return fmt.Errorf("failed to export session: %w", err)
	}

	u.PrintMessage(fmt.Sprintf("✅ Session '%s' exported to '%s'", sessionName, outputPath))

	return nil
}

// importSessionCommand imports a session from a checkpoint file.
func importSessionCommand(cmd *cobra.Command, args []string) error {
	checkpointPath := args[0]

	// Bind parsed flags to Viper for precedence handling.
	v := viper.GetViper()
	if err := importParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Get flags from Viper (supports CLI > ENV > config > defaults).
	sessionName := v.GetString("name")
	overwrite := v.GetBool("overwrite")
	includeContext := v.GetBool(contextFlag)

	log.Debugf("Importing session from '%s'", checkpointPath)

	manager, cleanup, err := initSessionManager()
	if err != nil {
		return err
	}
	defer cleanup()

	// Import session.
	ctx := context.Background()
	opts := session.ImportOptions{
		Name:              sessionName,
		OverwriteExisting: overwrite,
		IncludeContext:    includeContext,
	}

	importedSession, err := manager.ImportSession(ctx, checkpointPath, opts)
	if err != nil {
		return fmt.Errorf("failed to import session: %w", err)
	}

	u.PrintMessage(fmt.Sprintf("✅ Session '%s' imported successfully", importedSession.Name))
	u.PrintMessage(fmt.Sprintf("  ID: %s", importedSession.ID))
	u.PrintMessage(fmt.Sprintf("  Model: %s", importedSession.Model))
	u.PrintMessage(fmt.Sprintf("  Provider: %s", importedSession.Provider))
	u.PrintMessage(fmt.Sprintf("  Messages: %d", importedSession.MessageCount))
	u.PrintMessage("")
	u.PrintMessage("Resume the session with: atmos ai chat --session " + importedSession.Name)

	return nil
}
