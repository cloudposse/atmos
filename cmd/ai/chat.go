package ai

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/instructions"
	"github.com/cloudposse/atmos/pkg/ai/session"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/ai/tui"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// chatParser handles flag parsing with Viper precedence for the chat command.
var chatParser *flags.StandardParser

// getProviderFromConfig returns the current provider from configuration.
func getProviderFromConfig(atmosConfig *schema.AtmosConfiguration) string {
	if atmosConfig.Settings.AI.DefaultProvider != "" {
		return atmosConfig.Settings.AI.DefaultProvider
	}
	return "anthropic"
}

// getModelFromConfig returns the model for the current provider from configuration.
func getModelFromConfig(atmosConfig *schema.AtmosConfiguration) string {
	provider := getProviderFromConfig(atmosConfig)
	if providerConfig, err := ai.GetProviderConfig(atmosConfig, provider); err == nil {
		return providerConfig.Model
	}
	return ""
}

// aiChatCmd represents the ai chat command.
var chatCmd = &cobra.Command{
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
		// Bind parsed flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := chatParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Initialize configuration.
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return err
		}

		// Check if AI is enabled.
		if !isAIEnabled(&atmosConfig) {
			return fmt.Errorf("%w: Set 'settings.ai.enabled: true' in your atmos.yaml configuration",
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
			manager = session.NewManager(storage, atmosConfig.BasePath, atmosConfig.Settings.AI.Sessions.MaxSessions, &atmosConfig)

			// Get session flag from Viper (supports CLI > ENV > config > defaults).
			sessionName := v.GetString("session")

			ctx := context.Background()
			if sessionName != "" {
				// Try to resume existing session.
				sess, err = manager.GetSessionByName(ctx, sessionName)
				if err != nil {
					// Session doesn't exist, create new one.
					log.Debugf("Session '%s' not found, creating new session", sessionName)
					sess, err = manager.CreateSession(ctx, session.CreateSessionParams{Name: sessionName, Model: getModelFromConfig(&atmosConfig), Provider: getProviderFromConfig(&atmosConfig)})
					if err != nil {
						return fmt.Errorf("failed to create session: %w", err)
					}
					log.Debugf("Created new session: %s", sessionName)
				} else {
					log.Debugf("Resumed session: %s (%d messages)", sess.Name, 0)
				}
			} else {
				// Create anonymous session with timestamp.
				sessionName = fmt.Sprintf("session-%s", time.Now().Format("20060102-150405"))
				sess, err = manager.CreateSession(ctx, session.CreateSessionParams{Name: sessionName, Model: getModelFromConfig(&atmosConfig), Provider: getProviderFromConfig(&atmosConfig)})
				if err != nil {
					return fmt.Errorf("failed to create session: %w", err)
				}
				log.Debugf("Created new session: %s", sessionName)
			}
		}

		// Initialize tool registry and executor if tools are enabled.
		var executor *tools.Executor
		if atmosConfig.Settings.AI.Tools.Enabled {
			_, executor, err = initializeAIToolsAndExecutor(&atmosConfig)
			if err != nil {
				log.Warnf("Failed to initialize AI tools: %v", err)
			}
		}

		// Initialize project instructions if enabled.
		ctx := context.Background()
		var memoryMgr *instructions.Manager
		if atmosConfig.Settings.AI.Instructions.Enabled {
			log.Debug("Initializing project instructions")

			// Create instructions config.
			memConfig := &instructions.Config{
				Enabled:      atmosConfig.Settings.AI.Instructions.Enabled,
				FilePath:     atmosConfig.Settings.AI.Instructions.FilePath,
				AutoUpdate:   atmosConfig.Settings.AI.Instructions.AutoUpdate,
				CreateIfMiss: atmosConfig.Settings.AI.Instructions.CreateIfMiss,
				Sections:     atmosConfig.Settings.AI.Instructions.Sections,
			}

			// Create instructions manager.
			memoryMgr = instructions.NewManager(atmosConfig.BasePath, memConfig)

			// Load instructions (creates default if missing and CreateIfMiss is true).
			_, err := memoryMgr.Load(ctx)
			if err != nil {
				log.Warnf("Failed to load project instructions: %v", err)
				memoryMgr = nil // Disable instructions on error
			} else {
				log.Debug("Project instructions loaded successfully")
			}
		}

		// Start chat TUI with session, tools, and instructions.
		if err := tui.RunChat(tui.ChatOptions{
			Client:      client,
			AtmosConfig: &atmosConfig,
			Manager:     manager,
			Session:     sess,
			Executor:    executor,
			MemoryMgr:   memoryMgr,
		}); err != nil {
			return fmt.Errorf("chat session failed: %w", err)
		}

		return nil
	},
}

func init() {
	// Create parser with chat-specific flags using functional options.
	chatParser = flags.NewStandardParser(
		flags.WithStringFlag("session", "", "", "Resume or create a named session"),
		flags.WithEnvVars("session", "ATMOS_AI_SESSION"),
	)

	// Register flags on the command.
	chatParser.RegisterFlags(chatCmd)

	// Bind flags to Viper for environment variable support.
	if err := chatParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	aiCmd.AddCommand(chatCmd)
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

	// Default behavior: require confirmation (prompt user).
	// Users can opt-out by setting require_confirmation: false.
	if atmosConfig.Settings.AI.Tools.RequireConfirmation == nil {
		// Not set - default to prompting for security.
		return permission.ModePrompt
	}

	if *atmosConfig.Settings.AI.Tools.RequireConfirmation {
		// Explicitly set to true - prompt.
		return permission.ModePrompt
	}

	// Explicitly set to false - opt-out of prompting.
	return permission.ModeAllow
}
