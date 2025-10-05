package cmd

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

// aiHelpCmd represents the ai help command.
var aiHelpCmd = &cobra.Command{
	Use:   "help [topic]",
	Short: "Get AI-powered help on Atmos topics",
	Long: `Get intelligent help on specific Atmos topics from the AI assistant.

The AI can provide detailed explanations, examples, and best practices for various
Atmos concepts and workflows.

Common topics:
- stacks: Learn about Atmos stack configuration
- components: Understand Atmos components
- templating: Learn about Go templating in Atmos
- workflows: Understand Atmos workflow orchestration
- validation: Learn about configuration validation
- vendoring: Understand component vendoring

Examples:
  atmos ai help stacks
  atmos ai help components
  atmos ai help templating
  atmos ai help "terraform integration"`,
	Args: cobra.MaximumNArgs(1),
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

		var topic string
		if len(args) > 0 {
			topic = args[0]
		} else {
			topic = "general"
		}

		// Prepare help question based on topic.
		var question string
		switch strings.ToLower(topic) {
		case "stacks":
			question = "Explain Atmos stacks in detail. What are they, how do they work, and what are best practices for organizing stacks?"
		case "components":
			question = "Explain Atmos components in detail. What are they, how do they relate to stacks, and what are best practices for creating reusable components?"
		case "templating":
			question = "Explain Atmos templating capabilities. How do Go templates work in Atmos, what functions are available, and how can I use them effectively?"
		case "workflows":
			question = "Explain Atmos workflow orchestration. How do workflows work, when should I use them, and what are some common patterns?"
		case "validation":
			question = "Explain Atmos configuration validation. How does schema validation work, how can I validate my configurations, and what are common validation issues?"
		case "vendoring":
			question = "Explain Atmos component vendoring. How does vendoring work, when should I use it, and what are best practices for managing external components?"
		case "general":
			question = "Provide a comprehensive overview of Atmos. Explain the key concepts, architecture, and how all the pieces fit together."
		default:
			question = fmt.Sprintf("Explain '%s' in the context of Atmos. Provide detailed information, examples, and best practices.", topic)
		}

		log.Debug("Getting AI help", "topic", topic)

		// Create AI client using factory.
		client, err := ai.NewClient(&atmosConfig)
		if err != nil {
			return fmt.Errorf("failed to create AI client: %w", err)
		}

		// Create context with timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Send question and get response.
		utils.PrintfMessageToTUI("👽 Thinking...\n")
		response, err := client.SendMessage(ctx, question)
		if err != nil {
			return fmt.Errorf("failed to get AI response: %w", err)
		}

		// Print response.
		fmt.Println(response)

		return nil
	},
}

func init() {
	aiCmd.AddCommand(aiHelpCmd)
}
