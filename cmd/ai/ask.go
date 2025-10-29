package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

// aiAskCmd represents the ai ask command.
var askCmd = &cobra.Command{
	Use:   "ask [question]",
	Short: "Ask the AI assistant a question",
	Long: `Ask the AI assistant a specific question and get a response.

This command allows you to ask questions directly from the command line without
entering an interactive chat session. The AI has access to your Atmos configuration
and can provide context-aware responses.

Examples:
  atmos ai ask "What components are available?"
  atmos ai ask "How do I validate my stack configuration?"
  atmos ai ask "Explain the difference between components and stacks"
  atmos ai ask "Describe the vpc component in the dev stack"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
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

		// Join all arguments as the question.
		question := strings.Join(args, " ")

		log.Debug("Asking AI question", "question", question)

		// Create AI client using factory.
		client, err := ai.NewClient(&atmosConfig)
		if err != nil {
			return fmt.Errorf("failed to create AI client: %w", err)
		}

		// Check if we should send context with the question.
		sendContext, prompted, err := ai.ShouldSendContext(&atmosConfig, question)
		if err != nil {
			return fmt.Errorf("failed to determine context requirements: %w", err)
		}

		// Prepare the final question with optional context.
		finalQuestion := question
		if sendContext {
			if prompted {
				utils.PrintfMessageToTUI("📖 Reading stack configurations...\n")
			}

			stackContext, err := ai.GatherStackContext(&atmosConfig)
			if err != nil {
				utils.PrintfMessageToTUI("⚠️  Warning: Could not gather stack context: %v\n", err)
				utils.PrintfMessageToTUI("Proceeding without context...\n\n")
			} else {
				// Combine context and question.
				finalQuestion = fmt.Sprintf("%s\n\n%s", stackContext, question)
			}
		}

		// Create context with timeout (default 60 seconds if not configured).
		timeoutSeconds := 60
		if atmosConfig.Settings.AI.TimeoutSeconds > 0 {
			timeoutSeconds = atmosConfig.Settings.AI.TimeoutSeconds
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()

		// Send question and get response.
		utils.PrintfMessageToTUI("👽 Thinking...\n")
		response, err := client.SendMessage(ctx, finalQuestion)
		if err != nil {
			return fmt.Errorf("failed to get AI response: %w", err)
		}

		// Print response.
		fmt.Println(response)

		return nil
	},
}

func init() {
	aiCmd.AddCommand(askCmd)
}
