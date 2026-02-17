package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/executor"
	"github.com/cloudposse/atmos/pkg/ai/tools"
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
		// Get flags.
		includePatterns, _ := cmd.Flags().GetStringSlice("include")
		excludePatterns, _ := cmd.Flags().GetStringSlice("exclude")
		noAutoContext, _ := cmd.Flags().GetBool("no-auto-context")
		noTools, _ := cmd.Flags().GetBool("no-tools")

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

		// Apply context discovery overrides.
		if noAutoContext {
			atmosConfig.Settings.AI.Context.Enabled = false
		}
		if len(includePatterns) > 0 {
			atmosConfig.Settings.AI.Context.AutoInclude = append(atmosConfig.Settings.AI.Context.AutoInclude, includePatterns...)
		}
		if len(excludePatterns) > 0 {
			atmosConfig.Settings.AI.Context.Exclude = append(atmosConfig.Settings.AI.Context.Exclude, excludePatterns...)
		}

		// Join all arguments as the question.
		question := strings.Join(args, " ")

		log.Debug("Asking AI question", "question", question)

		// Create AI client using factory.
		client, err := ai.NewClient(&atmosConfig)
		if err != nil {
			return fmt.Errorf("failed to create AI client: %w", err)
		}

		// Gather context if explicitly configured (skip interactive prompt since tools handle introspection).
		finalQuestion := question
		if atmosConfig.Settings.AI.SendContext {
			stackContext, err := ai.GatherStackContext(&atmosConfig)
			if err != nil {
				log.Debug("Could not gather stack context", "error", err)
			} else {
				finalQuestion = fmt.Sprintf("%s\n\n%s", stackContext, question)
			}
		}

		// Create read-only tool executor (if tools are enabled).
		// The ask command uses only in-process, read-only tools (no subprocess execution).
		var toolExecutor *tools.Executor
		if !noTools && atmosConfig.Settings.AI.Tools.Enabled {
			_, toolExecutor, err = initializeAIReadOnlyTools(&atmosConfig)
			if err != nil {
				log.Warn("Failed to initialize tools", "error", err)
				// Continue without tools rather than failing.
				toolExecutor = nil
			}
		}

		// Create non-interactive executor.
		exec := executor.NewExecutor(client, toolExecutor, &atmosConfig)

		// Create context with timeout (default 60 seconds if not configured).
		timeoutSeconds := 60
		if atmosConfig.Settings.AI.TimeoutSeconds > 0 {
			timeoutSeconds = atmosConfig.Settings.AI.TimeoutSeconds
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()

		// Execute question with tool support.
		utils.PrintfMessageToTUI("👽 Thinking...\n")
		result := exec.Execute(ctx, executor.Options{
			Prompt:       finalQuestion,
			ToolsEnabled: !noTools && toolExecutor != nil,
		})

		if !result.Success {
			if result.Error != nil {
				return fmt.Errorf("%w: %s", errUtils.ErrAIExecutionFailed, result.Error.Message)
			}
			return errUtils.ErrAIExecutionFailed
		}

		// Render response as Markdown for rich terminal output.
		utils.PrintfMarkdown("%s", result.Response)

		return nil
	},
}

func init() {
	// Context discovery flags.
	askCmd.Flags().StringSlice("include", nil, "Add glob patterns to include in context (can be repeated)")
	askCmd.Flags().StringSlice("exclude", nil, "Add glob patterns to exclude from context (can be repeated)")
	askCmd.Flags().Bool("no-auto-context", false, "Disable automatic context discovery")
	askCmd.Flags().Bool("no-tools", false, "Disable tool execution")

	aiCmd.AddCommand(askCmd)
}
