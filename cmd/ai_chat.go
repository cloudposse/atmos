package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/session"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	atmosTools "github.com/cloudposse/atmos/pkg/ai/tools/atmos"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/ai/tui"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// aiChatCmd represents the ai chat command.
var aiChatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start interactive AI chat session",
	Long: `Start an interactive chat session with the Atmos AI assistant.

This opens a terminal-based chat interface where you can ask questions about your
Atmos configuration, get help with infrastructure management, and receive guidance
on best practices.

The AI assistant has access to your current Atmos configuration and can help with:
- Explaining Atmos concepts
- Analyzing your specific components and stacks
- Suggesting optimizations
- Debugging configuration issues
- Providing implementation guidance`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize configuration.
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return err
		}

		// Check if AI is enabled.
		if !isAIEnabled(&atmosConfig) {
			return fmt.Errorf("%w: AI features are not enabled. Set 'ai.enabled: true' in your atmos.yaml configuration",
				errUtils.ErrAINotEnabled)
		}

		log.Debug("Starting AI chat session")

		// Create AI client using factory.
		client, err := ai.NewClient(&atmosConfig)
		if err != nil {
			return fmt.Errorf("failed to create AI client: %w", err)
		}

		// Initialize session management if enabled.
		var manager *session.Manager
		var sess *session.Session
		var storage session.Storage

		if atmosConfig.Settings.AI.Sessions.Enabled {
			// Initialize session storage.
			storagePath := getSessionStoragePath(&atmosConfig)
			storage, err = session.NewSQLiteStorage(storagePath)
			if err != nil {
				return fmt.Errorf("failed to initialize session storage: %w", err)
			}
			defer storage.Close()

			// Create session manager.
			manager = session.NewManager(storage, atmosConfig.BasePath, atmosConfig.Settings.AI.Sessions.MaxSessions)

			// Check for --session flag.
			sessionName, _ := cmd.Flags().GetString("session")

			ctx := context.Background()
			if sessionName != "" {
				// Try to resume existing session.
				sess, err = manager.GetSessionByName(ctx, sessionName)
				if err != nil {
					// Session doesn't exist, create new one.
					log.Debug(fmt.Sprintf("Session '%s' not found, creating new session", sessionName))
					sess, err = manager.CreateSession(ctx, sessionName, atmosConfig.Settings.AI.Model, atmosConfig.Settings.AI.Provider, nil)
					if err != nil {
						return fmt.Errorf("failed to create session: %w", err)
					}
					log.Info(fmt.Sprintf("Created new session: %s", sessionName))
				} else {
					log.Info(fmt.Sprintf("Resumed session: %s (%d messages)", sess.Name, 0))
				}
			} else {
				// Create anonymous session with timestamp.
				sessionName = fmt.Sprintf("session-%s", time.Now().Format("20060102-150405"))
				sess, err = manager.CreateSession(ctx, sessionName, atmosConfig.Settings.AI.Model, atmosConfig.Settings.AI.Provider, nil)
				if err != nil {
					return fmt.Errorf("failed to create session: %w", err)
				}
				log.Info(fmt.Sprintf("Created new session: %s", sessionName))
			}
		}

		// Initialize tool registry and executor if tools are enabled.
		var executor *tools.Executor
		if atmosConfig.Settings.AI.Tools.Enabled {
			log.Debug("Initializing AI tools")

			// Create tool registry.
			registry := tools.NewRegistry()

			// Register Atmos tools.
			if err := registry.Register(atmosTools.NewDescribeComponentTool(&atmosConfig)); err != nil {
				log.Warn(fmt.Sprintf("Failed to register describe_component tool: %v", err))
			}
			if err := registry.Register(atmosTools.NewListStacksTool(&atmosConfig)); err != nil {
				log.Warn(fmt.Sprintf("Failed to register list_stacks tool: %v", err))
			}
			if err := registry.Register(atmosTools.NewValidateStacksTool(&atmosConfig)); err != nil {
				log.Warn(fmt.Sprintf("Failed to register validate_stacks tool: %v", err))
			}

			log.Debug(fmt.Sprintf("Registered %d tools", registry.Count()))

			// Create permission checker.
			permConfig := &permission.Config{
				Mode:            getPermissionMode(&atmosConfig),
				AllowedTools:    atmosConfig.Settings.AI.Tools.AllowedTools,
				RestrictedTools: atmosConfig.Settings.AI.Tools.RestrictedTools,
				BlockedTools:    atmosConfig.Settings.AI.Tools.BlockedTools,
				YOLOMode:        atmosConfig.Settings.AI.Tools.YOLOMode,
			}
			permChecker := permission.NewChecker(permConfig, permission.NewCLIPrompter())

			// Create tool executor.
			executor = tools.NewExecutor(registry, permChecker, tools.DefaultTimeout)
			log.Debug("Tool executor initialized")
		}

		// Start chat TUI with session and tools.
		if err := tui.RunChat(client, manager, sess, executor); err != nil {
			return fmt.Errorf("chat session failed: %w", err)
		}

		return nil
	},
}

func init() {
	aiCmd.AddCommand(aiChatCmd)

	// Add session flag.
	aiChatCmd.Flags().String("session", "", "Resume or create a named session")
}

// getSessionStoragePath returns the path to the session storage file.
func getSessionStoragePath(atmosConfig *schema.AtmosConfiguration) string {
	sessionPath := atmosConfig.Settings.AI.Sessions.Path
	if sessionPath == "" {
		sessionPath = ".atmos/sessions"
	}

	// If path is relative, make it relative to base path.
	if !filepath.IsAbs(sessionPath) {
		sessionPath = filepath.Join(atmosConfig.BasePath, sessionPath)
	}

	return filepath.Join(sessionPath, "sessions.db")
}

// getPermissionMode returns the permission mode from configuration.
func getPermissionMode(atmosConfig *schema.AtmosConfiguration) permission.Mode {
	if atmosConfig.Settings.AI.Tools.YOLOMode {
		return permission.ModeYOLO
	}

	if atmosConfig.Settings.AI.Tools.RequireConfirmation {
		return permission.ModePrompt
	}

	return permission.ModeAllow
}
