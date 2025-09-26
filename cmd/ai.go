package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/agent"
	"github.com/cloudposse/atmos/pkg/ai/tui"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

// aiCmd represents the ai command
var aiCmd = &cobra.Command{
	Use:   "ai",
	Short: "AI-powered assistant for Atmos operations",
	Long: `AI-powered assistant that helps with Atmos infrastructure management.

The AI assistant provides intelligent help with:
- Understanding Atmos concepts and architecture
- Analyzing component and stack configurations
- Suggesting best practices for infrastructure management
- Debugging configuration issues
- Guiding through complex workflows
- Explaining Terraform integration patterns

The assistant has access to your current Atmos configuration and can:
- Describe components and their configurations
- List available components and stacks
- Validate stack configurations
- Generate Terraform plans (read-only)
- Access Atmos settings and configuration`,
}

// aiChatCmd represents the ai chat command
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
		// Initialize configuration
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return err
		}

		// Check if AI is enabled
		if !isAIEnabled(&atmosConfig) {
			return fmt.Errorf("%w: AI features are not enabled. Set 'ai.enabled: true' in your atmos.yaml configuration",
				errUtils.ErrAINotEnabled)
		}

		log.Debug("Starting AI chat session")

		// Start chat TUI
		if err := tui.RunChat(&atmosConfig); err != nil {
			return fmt.Errorf("chat session failed: %w", err)
		}

		return nil
	},
}

// aiAskCmd represents the ai ask command
var aiAskCmd = &cobra.Command{
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
		// Initialize configuration
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return err
		}

		// Check if AI is enabled
		if !isAIEnabled(&atmosConfig) {
			return fmt.Errorf("%w: AI features are not enabled. Set 'ai.enabled: true' in your atmos.yaml configuration",
				errUtils.ErrAINotEnabled)
		}

		// Join all arguments as the question
		question := strings.Join(args, " ")

		log.Debug("Asking AI question", "question", question)

		// Create AI client
		client, err := agent.NewSimpleClient(&atmosConfig)
		if err != nil {
			return fmt.Errorf("failed to create AI client: %w", err)
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Send question and get response
		utils.PrintfMessageToTUI("🤔 Thinking...\n")
		response, err := client.SendMessage(ctx, question)
		if err != nil {
			return fmt.Errorf("failed to get AI response: %w", err)
		}

		// Print response
		utils.PrintfMessageToTUI("\n🤖 **Atmos AI:**\n\n")
		fmt.Println(response)

		return nil
	},
}

// aiHelpCmd represents the ai help command
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
		// Initialize configuration
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return err
		}

		// Check if AI is enabled
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

		// Prepare help question based on topic
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

		// Create AI client
		client, err := agent.NewSimpleClient(&atmosConfig)
		if err != nil {
			return fmt.Errorf("failed to create AI client: %w", err)
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Send question and get response
		utils.PrintfMessageToTUI("📚 Preparing help content...\n")
		response, err := client.SendMessage(ctx, question)
		if err != nil {
			return fmt.Errorf("failed to get AI response: %w", err)
		}

		// Print response
		utils.PrintfMessageToTUI("\n📖 **Help: %s**\n\n", strings.Title(topic))
		fmt.Println(response)

		return nil
	},
}

// isAIEnabled checks if AI features are enabled in the configuration
func isAIEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	if atmosConfig.Settings.AI == nil {
		return false
	}
	if enabled, ok := atmosConfig.Settings.AI["enabled"].(bool); ok {
		return enabled
	}
	return false
}

func init() {
	// Add subcommands to ai command
	aiCmd.AddCommand(aiChatCmd)
	aiCmd.AddCommand(aiAskCmd)
	aiCmd.AddCommand(aiHelpCmd)

	// Add ai command to root
	RootCmd.AddCommand(aiCmd)
}
