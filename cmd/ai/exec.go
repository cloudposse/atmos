package ai

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/executor"
	"github.com/cloudposse/atmos/pkg/ai/formatter"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// execCmd represents the ai exec command.
var execCmd = &cobra.Command{
	Use:   "exec [prompt]",
	Short: "Execute AI prompt non-interactively",
	Long: `Execute an AI prompt non-interactively and output the result.

This command is designed for automation, scripting, and CI/CD integration.
It executes a single prompt and outputs the result without any interactive UI.

The prompt can be provided as:
- Command arguments: atmos ai exec "your prompt here"
- Stdin (pipe): echo "your prompt" | atmos ai exec

Output can be formatted as:
- text (default): Plain text response
- json: Structured JSON with metadata
- markdown: Formatted markdown

Exit codes:
- 0: Success
- 1: AI error (API failure, invalid response)
- 2: Tool execution error

Examples:
  # Simple question
  atmos ai exec "List all available stacks"

  # With JSON output
  atmos ai exec "Describe the vpc component" --format json

  # Save output to file
  atmos ai exec "Analyze prod stack" --output analysis.json --format json

  # Disable tool execution
  atmos ai exec "Explain Atmos concepts" --no-tools

  # Pipe prompt from stdin
  echo "Validate stack configuration" | atmos ai exec --format json

  # Use in CI/CD pipeline
  result=$(atmos ai exec "Check for security issues" --format json)
  if echo "$result" | jq -e '.success == false'; then
    exit 1
  fi`,
	Args: cobra.MaximumNArgs(1), // Accept 0 or 1 argument (0 means stdin)
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flags.
		format, _ := cmd.Flags().GetString("format")
		outputFile, _ := cmd.Flags().GetString("output")
		noTools, _ := cmd.Flags().GetBool("no-tools")
		includeContext, _ := cmd.Flags().GetBool("context")
		providerName, _ := cmd.Flags().GetString("provider")
		sessionID, _ := cmd.Flags().GetString("session")

		// Initialize configuration.
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return exitWithError(1, "config_error", err)
		}

		// Check if AI is enabled.
		if !isAIEnabled(&atmosConfig) {
			return exitWithError(1, "config_error",
				fmt.Errorf("%w: Set 'settings.ai.enabled: true' in your atmos.yaml configuration", errUtils.ErrAINotEnabled))
		}

		// Override provider if specified.
		if providerName != "" {
			atmosConfig.Settings.AI.DefaultProvider = providerName
		}

		// Get prompt from args or stdin.
		prompt, err := getPrompt(args)
		if err != nil {
			return exitWithError(1, "input_error", err)
		}

		if prompt == "" {
			return exitWithError(1, "input_error", fmt.Errorf("no prompt provided: specify prompt as argument or pipe via stdin"))
		}

		log.Debug("Executing AI prompt", "prompt", prompt, "format", format, "tools_enabled", !noTools)

		// Create AI client.
		client, err := ai.NewClient(&atmosConfig)
		if err != nil {
			return exitWithError(1, "ai_error", fmt.Errorf("failed to create AI client: %w", err))
		}

		// Create tool executor (if tools are enabled).
		var toolExecutor *tools.Executor
		if !noTools && atmosConfig.Settings.AI.Tools.Enabled {
			// Use shared initialization function.
			_, toolExecutor, err = initializeAIToolsAndExecutor(&atmosConfig)
			if err != nil {
				log.Warn("Failed to initialize tools", "error", err)
				// Continue without tools rather than failing.
				toolExecutor = nil
			}
		}

		// Create non-interactive executor.
		exec := executor.NewExecutor(client, toolExecutor, &atmosConfig)

		// Set execution timeout.
		timeoutSeconds := 60
		if atmosConfig.Settings.AI.TimeoutSeconds > 0 {
			timeoutSeconds = atmosConfig.Settings.AI.TimeoutSeconds
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()

		// Execute prompt.
		result := exec.Execute(ctx, executor.Options{
			Prompt:         prompt,
			ToolsEnabled:   !noTools && toolExecutor != nil,
			SessionID:      sessionID,
			IncludeContext: includeContext,
		})

		// Format and output result.
		outputFormat := formatter.Format(format)
		formatterInstance := formatter.NewFormatter(outputFormat)

		// Determine output destination.
		var writer io.Writer = os.Stdout
		if outputFile != "" {
			file, err := os.Create(outputFile)
			if err != nil {
				return exitWithError(1, "io_error", fmt.Errorf("failed to create output file: %w", err))
			}
			defer file.Close()
			writer = file
		}

		// Format and write output.
		if err := formatterInstance.Format(writer, result); err != nil {
			return exitWithError(1, "format_error", fmt.Errorf("failed to format output: %w", err))
		}

		// Determine exit code based on result.
		if !result.Success {
			if result.Error != nil {
				switch result.Error.Type {
				case "tool_error":
					return exitWithError(2, result.Error.Type, fmt.Errorf("%s", result.Error.Message))
				default:
					return exitWithError(1, result.Error.Type, fmt.Errorf("%s", result.Error.Message))
				}
			}
			return exitWithError(1, "unknown_error", fmt.Errorf("execution failed"))
		}

		return nil
	},
}

// getPrompt gets the prompt from args or stdin.
func getPrompt(args []string) (string, error) {
	// If prompt provided as argument, use it.
	if len(args) > 0 {
		return strings.TrimSpace(args[0]), nil
	}

	// Check if stdin has data (pipe or redirect).
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat stdin: %w", err)
	}

	// If stdin is not a terminal (pipe or redirect), read from it.
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read from stdin: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	// No prompt from args or stdin.
	return "", nil
}

// exitWithError logs an error and returns an error that will cause the appropriate exit code.
func exitWithError(code int, errorType string, err error) error {
	log.Error("Execution failed", "error", err, "type", errorType, "exit_code", code)

	// Cobra will handle exit codes based on the error we return.
	// We use a wrapped error to indicate the exit code.
	return &execError{
		code:      code,
		errorType: errorType,
		err:       err,
	}
}

// execError wraps an error with an exit code.
type execError struct {
	code      int
	errorType string
	err       error
}

func (e *execError) Error() string {
	return e.err.Error()
}

func init() {
	// Register flags.
	execCmd.Flags().StringP("format", "f", "text", "Output format: text, json, markdown")
	execCmd.Flags().StringP("output", "o", "", "Output file (default: stdout)")
	execCmd.Flags().Bool("no-tools", false, "Disable tool execution")
	execCmd.Flags().Bool("context", false, "Include stack context in prompt")
	execCmd.Flags().StringP("provider", "p", "", "Override AI provider (anthropic, openai, gemini, etc.)")
	execCmd.Flags().StringP("session", "s", "", "Session ID for conversation context")

	// Add to ai command.
	aiCmd.AddCommand(execCmd)
}
