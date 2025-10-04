package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
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

		// Start chat TUI.
		if err := tui.RunChat(client); err != nil {
			return fmt.Errorf("chat session failed: %w", err)
		}

		return nil
	},
}

func init() {
	aiCmd.AddCommand(aiChatCmd)
}
