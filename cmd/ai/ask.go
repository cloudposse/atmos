package ai

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/executor"
	"github.com/cloudposse/atmos/pkg/ai/formatter"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

//go:embed markdown/atmos_ai_ask.md
var askLongMarkdown string

// askParser handles flag parsing with Viper precedence for the ask command.
var askParser *flags.StandardParser

// aiAskCmd represents the ai ask command.
var askCmd = &cobra.Command{
	Use:   "ask [question]",
	Short: "Ask the AI assistant a question",
	Long:  askLongMarkdown,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bind parsed flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := askParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flags from Viper (supports CLI > ENV > config > defaults).
		includePatterns := v.GetStringSlice("include")
		excludePatterns := v.GetStringSlice("exclude")
		noAutoContext := v.GetBool("no-auto-context")
		noTools := v.GetBool("no-tools")
		mcpServers := v.GetStringSlice("mcp")

		// Initialize configuration.
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return err
		}

		// Check if AI is enabled.
		if !isAIEnabled(&atmosConfig) {
			return fmt.Errorf("%w: Set 'ai.enabled: true' in your atmos.yaml configuration",
				errUtils.ErrAINotEnabled)
		}

		// Apply context discovery overrides.
		if noAutoContext {
			atmosConfig.AI.Context.Enabled = false
		}
		if len(includePatterns) > 0 {
			atmosConfig.AI.Context.AutoInclude = append(atmosConfig.AI.Context.AutoInclude, includePatterns...)
		}
		if len(excludePatterns) > 0 {
			atmosConfig.AI.Context.Exclude = append(atmosConfig.AI.Context.Exclude, excludePatterns...)
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
		if atmosConfig.AI.SendContext {
			stackContext, err := ai.GatherStackContext(&atmosConfig)
			if err != nil {
				log.Debug("Could not gather stack context", "error", err)
			} else {
				finalQuestion = fmt.Sprintf("%s\n\n%s", stackContext, question)
			}
		}

		// Create tool executor (if tools are enabled).
		var toolExecutor *tools.Executor
		if !noTools && atmosConfig.AI.Tools.Enabled {
			toolsResult, toolsErr := initializeAIToolsAndExecutor(&atmosConfig, mcpServers, question)
			if toolsErr != nil {
				log.Warn("Failed to initialize tools", "error", toolsErr)
				// Continue without tools rather than failing.
			}
			if toolsResult != nil {
				toolExecutor = toolsResult.Executor
				if toolsResult.MCPMgr != nil {
					defer toolsResult.MCPMgr.StopAll() //nolint:errcheck // Best-effort MCP server cleanup.
				}
			}
		}

		// Create non-interactive executor.
		exec := executor.NewExecutor(client, toolExecutor, &atmosConfig)

		// Create context with timeout (default 60 seconds if not configured).
		timeoutSeconds := 60
		if atmosConfig.AI.TimeoutSeconds > 0 {
			timeoutSeconds = atmosConfig.AI.TimeoutSeconds
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

		// Render response with tool execution details as Markdown.
		var buf bytes.Buffer
		mdFormatter := formatter.NewFormatter(formatter.FormatMarkdown)
		if err := mdFormatter.Format(&buf, result); err != nil {
			return fmt.Errorf("failed to format response: %w", err)
		}
		utils.PrintfMarkdown("%s", buf.String())

		return nil
	},
}

func init() {
	// Create parser with ask-specific flags using functional options.
	askParser = flags.NewStandardParser(
		flags.WithStringSliceFlag("include", "", nil, "Add glob patterns to include in context (can be repeated)"),
		flags.WithStringSliceFlag("exclude", "", nil, "Add glob patterns to exclude from context (can be repeated)"),
		flags.WithBoolFlag("no-auto-context", "", false, "Disable automatic context discovery"),
		flags.WithBoolFlag("no-tools", "", false, "Disable tool execution"),
		flags.WithStringSliceFlag("mcp", "", nil, "MCP servers to use (comma-separated, skips auto-routing)"),
		flags.WithEnvVars("include", "ATMOS_AI_INCLUDE"),
		flags.WithEnvVars("exclude", "ATMOS_AI_EXCLUDE"),
		flags.WithEnvVars("no-auto-context", "ATMOS_AI_NO_AUTO_CONTEXT"),
		flags.WithEnvVars("no-tools", "ATMOS_AI_NO_TOOLS"),
		flags.WithEnvVars("mcp", "ATMOS_AI_MCP"),
	)

	// Register flags on the command.
	askParser.RegisterFlags(askCmd)

	// Bind flags to Viper for environment variable support.
	if err := askParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	aiCmd.AddCommand(askCmd)
}
