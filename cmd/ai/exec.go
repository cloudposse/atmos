package ai

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
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
)

//go:embed markdown/atmos_ai_exec.md
var execLongMarkdown string

// execParser handles flag parsing with Viper precedence for the exec command.
var execParser *flags.StandardParser

// execCmd represents the ai exec command.
var execCmd = &cobra.Command{
	Use:   "exec [prompt]",
	Short: "Execute AI prompt non-interactively",
	Long:  execLongMarkdown,
	Args:  cobra.MaximumNArgs(1), // Accept 0 or 1 argument (0 means stdin)
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bind parsed flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := execParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flags from Viper (supports CLI > ENV > config > defaults).
		format := v.GetString("format")
		outputFile := v.GetString("output")
		noTools := v.GetBool("no-tools")
		includeContext := v.GetBool("context")
		providerName := v.GetString("provider")
		sessionID := v.GetString("session")
		includePatterns := v.GetStringSlice("include")
		excludePatterns := v.GetStringSlice("exclude")
		noAutoContext := v.GetBool("no-auto-context")

		// Initialize configuration.
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return exitWithError(1, "config_error", err)
		}

		// Check if AI is enabled.
		if !isAIEnabled(&atmosConfig) {
			return exitWithError(1, "config_error",
				fmt.Errorf("%w: Set 'ai.enabled: true' in your atmos.yaml configuration", errUtils.ErrAINotEnabled))
		}

		// Override provider if specified.
		if providerName != "" {
			atmosConfig.AI.DefaultProvider = providerName
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

		// Get prompt from args or stdin.
		prompt, err := getPrompt(args)
		if err != nil {
			return exitWithError(1, "input_error", err)
		}

		if prompt == "" {
			return exitWithError(1, "input_error", fmt.Errorf("%w: specify prompt as argument or pipe via stdin", errUtils.ErrAIPromptRequired))
		}

		log.Debug("Executing AI prompt", "prompt", prompt, "format", format, "tools_enabled", !noTools)

		// Create AI client.
		client, err := ai.NewClient(&atmosConfig)
		if err != nil {
			return exitWithError(1, "ai_error", fmt.Errorf("failed to create AI client: %w", err))
		}

		// Create tool executor (if tools are enabled).
		var toolExecutor *tools.Executor
		if !noTools && atmosConfig.AI.Tools.Enabled {
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
		if atmosConfig.AI.TimeoutSeconds > 0 {
			timeoutSeconds = atmosConfig.AI.TimeoutSeconds
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
					return exitWithError(2, result.Error.Type, fmt.Errorf("%w: %s", errUtils.ErrAIToolExecutionFailed, result.Error.Message))
				default:
					return exitWithError(1, result.Error.Type, fmt.Errorf("%w: %s", errUtils.ErrAIExecutionFailed, result.Error.Message))
				}
			}
			return exitWithError(1, "unknown_error", errUtils.ErrAIExecutionFailed)
		}

		return nil
	},
}

// stdinSource represents a source for reading stdin data.
// It can be swapped in tests to simulate pipe input.
type stdinSource interface {
	Stat() (os.FileInfo, error)
	Read(p []byte) (n int, err error)
}

// stdinReader is the stdin source used by getPrompt.
// In production it uses os.Stdin; tests can swap it.
var stdinReader stdinSource = os.Stdin

// getPrompt gets the prompt from args or stdin.
func getPrompt(args []string) (string, error) {
	// If prompt provided as argument, use it.
	if len(args) > 0 {
		return strings.TrimSpace(args[0]), nil
	}

	// Check if stdin has data (pipe or redirect).
	stat, err := stdinReader.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat stdin: %w", err)
	}

	// If stdin is not a terminal (pipe or redirect), read from it.
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(stdinReader)
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
	// Create parser with exec-specific flags using functional options.
	execParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "f", "text", "Output format: text, json, markdown"),
		flags.WithStringFlag("output", "o", "", "Output file (default: stdout)"),
		flags.WithBoolFlag("no-tools", "", false, "Disable tool execution"),
		flags.WithBoolFlag("context", "", false, "Include stack context in prompt"),
		flags.WithStringFlag("provider", "p", "", "Override AI provider (anthropic, openai, gemini, etc.)"),
		flags.WithStringFlag("session", "s", "", "Session ID for conversation context"),
		flags.WithStringSliceFlag("include", "", nil, "Add glob patterns to include in context (can be repeated)"),
		flags.WithStringSliceFlag("exclude", "", nil, "Add glob patterns to exclude from context (can be repeated)"),
		flags.WithBoolFlag("no-auto-context", "", false, "Disable automatic context discovery"),
		flags.WithEnvVars("format", "ATMOS_AI_FORMAT"),
		flags.WithEnvVars("output", "ATMOS_AI_OUTPUT"),
		flags.WithEnvVars("no-tools", "ATMOS_AI_NO_TOOLS"),
		flags.WithEnvVars("context", "ATMOS_AI_CONTEXT"),
		flags.WithEnvVars("provider", "ATMOS_AI_PROVIDER"),
		flags.WithEnvVars("session", "ATMOS_AI_SESSION"),
		flags.WithEnvVars("include", "ATMOS_AI_INCLUDE"),
		flags.WithEnvVars("exclude", "ATMOS_AI_EXCLUDE"),
		flags.WithEnvVars("no-auto-context", "ATMOS_AI_NO_AUTO_CONTEXT"),
	)

	// Register flags on the command.
	execParser.RegisterFlags(execCmd)

	// Bind flags to Viper for environment variable support.
	if err := execParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add to ai command.
	aiCmd.AddCommand(execCmd)
}
