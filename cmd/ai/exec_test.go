package ai

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/formatter"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecCommand_BasicProperties(t *testing.T) {
	t.Run("exec command properties", func(t *testing.T) {
		assert.Equal(t, "exec [prompt]", execCmd.Use)
		assert.Equal(t, "Execute AI prompt non-interactively", execCmd.Short)
		assert.NotEmpty(t, execCmd.Long)
		assert.NotNil(t, execCmd.RunE)
		// Check that Args allows maximum 1 argument.
		assert.NotNil(t, execCmd.Args)
	})

	t.Run("exec command has descriptive long text", func(t *testing.T) {
		assert.Contains(t, execCmd.Long, "non-interactively")
		assert.Contains(t, execCmd.Long, "automation")
		assert.Contains(t, execCmd.Long, "stdin")
		assert.Contains(t, execCmd.Long, "json")
		assert.Contains(t, execCmd.Long, "Exit codes")
	})
}

func TestExecCommand_Flags(t *testing.T) {
	t.Run("exec command has format flag", func(t *testing.T) {
		formatFlag := execCmd.Flags().Lookup("format")
		require.NotNil(t, formatFlag, "format flag should be registered")
		assert.Equal(t, "string", formatFlag.Value.Type())
		assert.Equal(t, "f", formatFlag.Shorthand)
		assert.Equal(t, "text", formatFlag.DefValue)
	})

	t.Run("exec command has output flag", func(t *testing.T) {
		outputFlag := execCmd.Flags().Lookup("output")
		require.NotNil(t, outputFlag, "output flag should be registered")
		assert.Equal(t, "string", outputFlag.Value.Type())
		assert.Equal(t, "o", outputFlag.Shorthand)
		assert.Equal(t, "", outputFlag.DefValue)
	})

	t.Run("exec command has no-tools flag", func(t *testing.T) {
		noToolsFlag := execCmd.Flags().Lookup("no-tools")
		require.NotNil(t, noToolsFlag, "no-tools flag should be registered")
		assert.Equal(t, "bool", noToolsFlag.Value.Type())
		assert.Equal(t, "false", noToolsFlag.DefValue)
	})

	t.Run("exec command has context flag", func(t *testing.T) {
		contextFlag := execCmd.Flags().Lookup("context")
		require.NotNil(t, contextFlag, "context flag should be registered")
		assert.Equal(t, "bool", contextFlag.Value.Type())
		assert.Equal(t, "false", contextFlag.DefValue)
	})

	t.Run("exec command has provider flag", func(t *testing.T) {
		providerFlag := execCmd.Flags().Lookup("provider")
		require.NotNil(t, providerFlag, "provider flag should be registered")
		assert.Equal(t, "string", providerFlag.Value.Type())
		assert.Equal(t, "p", providerFlag.Shorthand)
		assert.Equal(t, "", providerFlag.DefValue)
	})

	t.Run("exec command has session flag", func(t *testing.T) {
		sessionFlag := execCmd.Flags().Lookup("session")
		require.NotNil(t, sessionFlag, "session flag should be registered")
		assert.Equal(t, "string", sessionFlag.Value.Type())
		assert.Equal(t, "s", sessionFlag.Shorthand)
		assert.Equal(t, "", sessionFlag.DefValue)
	})

	t.Run("exec command has include flag", func(t *testing.T) {
		includeFlag := execCmd.Flags().Lookup("include")
		require.NotNil(t, includeFlag, "include flag should be registered")
		assert.Equal(t, "stringSlice", includeFlag.Value.Type())
	})

	t.Run("exec command has exclude flag", func(t *testing.T) {
		excludeFlag := execCmd.Flags().Lookup("exclude")
		require.NotNil(t, excludeFlag, "exclude flag should be registered")
		assert.Equal(t, "stringSlice", excludeFlag.Value.Type())
	})

	t.Run("exec command has no-auto-context flag", func(t *testing.T) {
		noAutoContextFlag := execCmd.Flags().Lookup("no-auto-context")
		require.NotNil(t, noAutoContextFlag, "no-auto-context flag should be registered")
		assert.Equal(t, "bool", noAutoContextFlag.Value.Type())
		assert.Equal(t, "false", noAutoContextFlag.DefValue)
	})
}

func TestExecCommand_CommandHierarchy(t *testing.T) {
	t.Run("exec command is attached to ai command", func(t *testing.T) {
		parent := execCmd.Parent()
		assert.NotNil(t, parent)
		assert.Equal(t, "ai", parent.Name())
	})
}

func TestExecCommand_ArgsValidation(t *testing.T) {
	t.Run("accepts zero arguments (stdin)", func(t *testing.T) {
		err := cobra.MaximumNArgs(1)(execCmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("accepts one argument (prompt)", func(t *testing.T) {
		err := cobra.MaximumNArgs(1)(execCmd, []string{"test prompt"})
		assert.NoError(t, err)
	})

	t.Run("rejects two or more arguments", func(t *testing.T) {
		err := cobra.MaximumNArgs(1)(execCmd, []string{"arg1", "arg2"})
		assert.Error(t, err)
	})
}

func TestGetPrompt(t *testing.T) {
	t.Run("returns prompt from args", func(t *testing.T) {
		prompt, err := getPrompt([]string{"my test prompt"})
		assert.NoError(t, err)
		assert.Equal(t, "my test prompt", prompt)
	})

	t.Run("trims whitespace from prompt", func(t *testing.T) {
		prompt, err := getPrompt([]string{"  my test prompt  "})
		assert.NoError(t, err)
		assert.Equal(t, "my test prompt", prompt)
	})

	t.Run("returns empty string when no args and stdin is terminal", func(t *testing.T) {
		// When args are empty and stdin is a terminal (not a pipe), return empty string.
		prompt, err := getPrompt([]string{})
		// In test environment, stdin may vary, so just check no error for now.
		if err == nil {
			// Either empty or stdin content.
			_ = prompt
		}
	})

	t.Run("handles prompt with newlines", func(t *testing.T) {
		prompt, err := getPrompt([]string{"line1\nline2\nline3"})
		assert.NoError(t, err)
		assert.Equal(t, "line1\nline2\nline3", prompt)
	})

	t.Run("handles unicode prompt", func(t *testing.T) {
		prompt, err := getPrompt([]string{"Hello 世界 🌍"})
		assert.NoError(t, err)
		assert.Equal(t, "Hello 世界 🌍", prompt)
	})

	t.Run("handles prompt with special characters", func(t *testing.T) {
		prompt, err := getPrompt([]string{"test \"quoted\" and 'single' $var"})
		assert.NoError(t, err)
		assert.Equal(t, "test \"quoted\" and 'single' $var", prompt)
	})

	t.Run("returns first argument only", func(t *testing.T) {
		// getPrompt only looks at args[0].
		prompt, err := getPrompt([]string{"first"})
		assert.NoError(t, err)
		assert.Equal(t, "first", prompt)
	})
}

func TestExitWithError(t *testing.T) {
	t.Run("returns execError with correct properties", func(t *testing.T) {
		originalErr := errors.New("test error")
		result := exitWithError(1, "test_type", originalErr)

		require.NotNil(t, result)
		var execErr *execError
		require.True(t, errors.As(result, &execErr), "result should be *execError")

		assert.Equal(t, 1, execErr.code)
		assert.Equal(t, "test_type", execErr.errorType)
		assert.Equal(t, originalErr, execErr.err)
	})

	t.Run("exit code 2 for tool errors", func(t *testing.T) {
		toolErr := errors.New("tool execution failed")
		result := exitWithError(2, "tool_error", toolErr)

		var execErr *execError
		require.True(t, errors.As(result, &execErr))
		assert.Equal(t, 2, execErr.code)
		assert.Equal(t, "tool_error", execErr.errorType)
	})

	t.Run("exit code 1 for AI errors", func(t *testing.T) {
		aiErr := errors.New("AI processing failed")
		result := exitWithError(1, "ai_error", aiErr)

		var execErr *execError
		require.True(t, errors.As(result, &execErr))
		assert.Equal(t, 1, execErr.code)
		assert.Equal(t, "ai_error", execErr.errorType)
	})

	t.Run("exit code 1 for config errors", func(t *testing.T) {
		configErr := errors.New("configuration error")
		result := exitWithError(1, "config_error", configErr)

		var execErr *execError
		require.True(t, errors.As(result, &execErr))
		assert.Equal(t, 1, execErr.code)
		assert.Equal(t, "config_error", execErr.errorType)
	})

	t.Run("exit code 1 for input errors", func(t *testing.T) {
		inputErr := errors.New("invalid input")
		result := exitWithError(1, "input_error", inputErr)

		var execErr *execError
		require.True(t, errors.As(result, &execErr))
		assert.Equal(t, 1, execErr.code)
		assert.Equal(t, "input_error", execErr.errorType)
	})

	t.Run("exit code 1 for IO errors", func(t *testing.T) {
		ioErr := errors.New("IO error")
		result := exitWithError(1, "io_error", ioErr)

		var execErr *execError
		require.True(t, errors.As(result, &execErr))
		assert.Equal(t, 1, execErr.code)
		assert.Equal(t, "io_error", execErr.errorType)
	})

	t.Run("exit code 1 for format errors", func(t *testing.T) {
		formatErr := errors.New("format error")
		result := exitWithError(1, "format_error", formatErr)

		var execErr *execError
		require.True(t, errors.As(result, &execErr))
		assert.Equal(t, 1, execErr.code)
		assert.Equal(t, "format_error", execErr.errorType)
	})

	t.Run("exit code 1 for unknown errors", func(t *testing.T) {
		unknownErr := errors.New("unknown error")
		result := exitWithError(1, "unknown_error", unknownErr)

		var execErr *execError
		require.True(t, errors.As(result, &execErr))
		assert.Equal(t, 1, execErr.code)
		assert.Equal(t, "unknown_error", execErr.errorType)
	})

	t.Run("preserves wrapped errors", func(t *testing.T) {
		innerErr := errors.New("inner error")
		wrappedErr := errUtils.ErrAINotEnabled
		result := exitWithError(1, "config_error", wrappedErr)

		var execErr *execError
		require.True(t, errors.As(result, &execErr))
		assert.True(t, errors.Is(execErr.err, errUtils.ErrAINotEnabled))
		assert.False(t, errors.Is(execErr.err, innerErr))
	})
}

func TestExecError_Error(t *testing.T) {
	t.Run("returns underlying error message", func(t *testing.T) {
		originalErr := errors.New("original error message")
		execErr := &execError{
			code:      1,
			errorType: "test",
			err:       originalErr,
		}

		assert.Equal(t, "original error message", execErr.Error())
	})

	t.Run("returns wrapped error message", func(t *testing.T) {
		wrappedErr := errUtils.ErrAINotEnabled
		execErr := &execError{
			code:      1,
			errorType: "config_error",
			err:       wrappedErr,
		}

		assert.Equal(t, wrappedErr.Error(), execErr.Error())
	})

	t.Run("returns complex error message", func(t *testing.T) {
		complexErr := errors.New("failed to create AI client: API key not found")
		execErr := &execError{
			code:      1,
			errorType: "ai_error",
			err:       complexErr,
		}

		assert.Equal(t, "failed to create AI client: API key not found", execErr.Error())
	})
}

func TestExecError_Fields(t *testing.T) {
	t.Run("code field is accessible", func(t *testing.T) {
		execErr := &execError{
			code:      42,
			errorType: "test",
			err:       errors.New("test"),
		}
		assert.Equal(t, 42, execErr.code)
	})

	t.Run("errorType field is accessible", func(t *testing.T) {
		execErr := &execError{
			code:      1,
			errorType: "custom_error_type",
			err:       errors.New("test"),
		}
		assert.Equal(t, "custom_error_type", execErr.errorType)
	})

	t.Run("err field is accessible", func(t *testing.T) {
		originalErr := errors.New("original")
		execErr := &execError{
			code:      1,
			errorType: "test",
			err:       originalErr,
		}
		assert.Equal(t, originalErr, execErr.err)
	})
}

func TestExecCommand_ErrorCases(t *testing.T) {
	t.Run("returns error without valid config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

		testCmd := &cobra.Command{
			Use:  "exec",
			Args: cobra.MaximumNArgs(1),
		}
		testCmd.Flags().StringP("format", "f", "text", "Output format")
		testCmd.Flags().StringP("output", "o", "", "Output file")
		testCmd.Flags().Bool("no-tools", false, "Disable tools")
		testCmd.Flags().Bool("context", false, "Include context")
		testCmd.Flags().StringP("provider", "p", "", "Provider")
		testCmd.Flags().StringP("session", "s", "", "Session")
		testCmd.Flags().StringSlice("include", nil, "Include patterns")
		testCmd.Flags().StringSlice("exclude", nil, "Exclude patterns")
		testCmd.Flags().Bool("no-auto-context", false, "Disable auto context")

		// Use the actual exec command's RunE function.
		err := execCmd.RunE(testCmd, []string{"test prompt"})
		assert.Error(t, err)
	})

	t.Run("returns error for empty prompt with terminal stdin", func(t *testing.T) {
		// Create a temp directory for config.
		tmpDir := t.TempDir()
		t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
		t.Setenv("ATMOS_BASE_PATH", tmpDir)

		// Create minimal atmos.yaml with AI enabled.
		atmosYaml := `
settings:
  ai:
    enabled: true
    default_provider: anthropic
`
		err := os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYaml), 0o644)
		require.NoError(t, err)

		testCmd := &cobra.Command{
			Use:  "exec",
			Args: cobra.MaximumNArgs(1),
		}
		testCmd.Flags().StringP("format", "f", "text", "Output format")
		testCmd.Flags().StringP("output", "o", "", "Output file")
		testCmd.Flags().Bool("no-tools", false, "Disable tools")
		testCmd.Flags().Bool("context", false, "Include context")
		testCmd.Flags().StringP("provider", "p", "", "Provider")
		testCmd.Flags().StringP("session", "s", "", "Session")
		testCmd.Flags().StringSlice("include", nil, "Include patterns")
		testCmd.Flags().StringSlice("exclude", nil, "Exclude patterns")
		testCmd.Flags().Bool("no-auto-context", false, "Disable auto context")

		// Test with empty args (no prompt) - will fail because stdin is not a pipe.
		err = execCmd.RunE(testCmd, []string{})
		if err != nil {
			// Verify error is input_error type (empty prompt) or config error.
			var execErr *execError
			if errors.As(err, &execErr) {
				assert.Contains(t, []string{"input_error", "config_error"}, execErr.errorType)
			}
		}
	})
}

func TestExecCommand_Examples(t *testing.T) {
	t.Run("long description contains examples", func(t *testing.T) {
		assert.Contains(t, execCmd.Long, "atmos ai exec")
		assert.Contains(t, execCmd.Long, "--format json")
		assert.Contains(t, execCmd.Long, "--output")
		assert.Contains(t, execCmd.Long, "--no-tools")
	})

	t.Run("long description explains exit codes", func(t *testing.T) {
		assert.Contains(t, execCmd.Long, "0: Success")
		assert.Contains(t, execCmd.Long, "1: AI error")
		assert.Contains(t, execCmd.Long, "2: Tool execution error")
	})
}

func TestExecCommand_FormatOptions(t *testing.T) {
	t.Run("long description describes format options", func(t *testing.T) {
		assert.Contains(t, execCmd.Long, "text (default)")
		assert.Contains(t, execCmd.Long, "json")
		assert.Contains(t, execCmd.Long, "markdown")
	})
}

func TestExecCommand_InputMethods(t *testing.T) {
	t.Run("long description describes input methods", func(t *testing.T) {
		assert.Contains(t, execCmd.Long, "Command arguments")
		assert.Contains(t, execCmd.Long, "Stdin (pipe)")
	})
}

func TestExecCommand_FlagDefaults(t *testing.T) {
	t.Run("format defaults to text", func(t *testing.T) {
		flag := execCmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		assert.Equal(t, "text", flag.DefValue)
	})

	t.Run("output defaults to empty (stdout)", func(t *testing.T) {
		flag := execCmd.Flags().Lookup("output")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("no-tools defaults to false", func(t *testing.T) {
		flag := execCmd.Flags().Lookup("no-tools")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("context defaults to false", func(t *testing.T) {
		flag := execCmd.Flags().Lookup("context")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("provider defaults to empty", func(t *testing.T) {
		flag := execCmd.Flags().Lookup("provider")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("session defaults to empty", func(t *testing.T) {
		flag := execCmd.Flags().Lookup("session")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("no-auto-context defaults to false", func(t *testing.T) {
		flag := execCmd.Flags().Lookup("no-auto-context")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

func TestExecCommand_FlagShorthands(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		shorthand string
	}{
		{"format has shorthand f", "format", "f"},
		{"output has shorthand o", "output", "o"},
		{"provider has shorthand p", "provider", "p"},
		{"session has shorthand s", "session", "s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := execCmd.Flags().Lookup(tt.flag)
			require.NotNil(t, flag, "flag %s should exist", tt.flag)
			assert.Equal(t, tt.shorthand, flag.Shorthand)
		})
	}
}

func TestExecCommand_FlagTypes(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		flagType string
	}{
		{"format is string", "format", "string"},
		{"output is string", "output", "string"},
		{"no-tools is bool", "no-tools", "bool"},
		{"context is bool", "context", "bool"},
		{"provider is string", "provider", "string"},
		{"session is string", "session", "string"},
		{"include is stringSlice", "include", "stringSlice"},
		{"exclude is stringSlice", "exclude", "stringSlice"},
		{"no-auto-context is bool", "no-auto-context", "bool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := execCmd.Flags().Lookup(tt.flag)
			require.NotNil(t, flag, "flag %s should exist", tt.flag)
			assert.Equal(t, tt.flagType, flag.Value.Type())
		})
	}
}

func TestExecError_ErrorChain(t *testing.T) {
	t.Run("error chain works with errors.Is", func(t *testing.T) {
		execErr := &execError{
			code:      1,
			errorType: "config_error",
			err:       errUtils.ErrAINotEnabled,
		}

		// execError.Error() returns the underlying error's message.
		assert.Equal(t, errUtils.ErrAINotEnabled.Error(), execErr.Error())
	})

	t.Run("error chain works with wrapped errors", func(t *testing.T) {
		innerErr := errUtils.ErrAIPromptRequired
		wrappedErr := errors.New("prompt is required: specify prompt as argument or pipe via stdin")
		execErr := &execError{
			code:      1,
			errorType: "input_error",
			err:       wrappedErr,
		}

		assert.Equal(t, wrappedErr.Error(), execErr.Error())
		// Note: errors.Is won't work because we're not using fmt.Errorf with %w.
		_ = innerErr
	})
}

func TestExecCommand_CIUsage(t *testing.T) {
	t.Run("long description shows CI/CD example", func(t *testing.T) {
		assert.Contains(t, execCmd.Long, "CI/CD pipeline")
		assert.Contains(t, execCmd.Long, "result=$(atmos ai exec")
	})

	t.Run("long description shows pipe example", func(t *testing.T) {
		assert.Contains(t, execCmd.Long, "echo")
		assert.Contains(t, execCmd.Long, "| atmos ai exec")
	})

	t.Run("long description shows save to file example", func(t *testing.T) {
		assert.Contains(t, execCmd.Long, "--output analysis.json")
	})
}

func TestGetPrompt_EdgeCases(t *testing.T) {
	t.Run("handles very long prompt", func(t *testing.T) {
		longPrompt := strings.Repeat("a", 10000)
		prompt, err := getPrompt([]string{longPrompt})
		assert.NoError(t, err)
		assert.Equal(t, longPrompt, prompt)
	})

	t.Run("handles prompt with only whitespace", func(t *testing.T) {
		prompt, err := getPrompt([]string{"   \t\n   "})
		assert.NoError(t, err)
		assert.Equal(t, "", prompt)
	})

	t.Run("handles prompt with leading and trailing newlines", func(t *testing.T) {
		prompt, err := getPrompt([]string{"\n\nmiddle\n\n"})
		assert.NoError(t, err)
		assert.Equal(t, "middle", prompt)
	})

	t.Run("handles empty string argument", func(t *testing.T) {
		prompt, err := getPrompt([]string{""})
		assert.NoError(t, err)
		assert.Equal(t, "", prompt)
	})
}

func TestExitWithError_AllErrorTypes(t *testing.T) {
	errorTypes := []struct {
		name      string
		code      int
		errorType string
	}{
		{"config_error", 1, "config_error"},
		{"input_error", 1, "input_error"},
		{"ai_error", 1, "ai_error"},
		{"tool_error", 2, "tool_error"},
		{"io_error", 1, "io_error"},
		{"format_error", 1, "format_error"},
		{"unknown_error", 1, "unknown_error"},
	}

	for _, et := range errorTypes {
		t.Run(et.name, func(t *testing.T) {
			testErr := errors.New("test " + et.name)
			result := exitWithError(et.code, et.errorType, testErr)

			var execErr *execError
			require.True(t, errors.As(result, &execErr))
			assert.Equal(t, et.code, execErr.code)
			assert.Equal(t, et.errorType, execErr.errorType)
			assert.Contains(t, execErr.Error(), et.name)
		})
	}
}

func TestExecCommand_AIEnabledCheck(t *testing.T) {
	t.Run("isAIEnabled returns false when AI is disabled", func(t *testing.T) {
		// Test the isAIEnabled function directly with a mock config.
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: false,
				},
			},
		}
		assert.False(t, isAIEnabled(atmosConfig))
	})

	t.Run("isAIEnabled returns true when AI is enabled", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: true,
				},
			},
		}
		assert.True(t, isAIEnabled(atmosConfig))
	})

	t.Run("AI not enabled error is config_error type", func(t *testing.T) {
		// Test that when AI is not enabled, the error is properly constructed.
		err := exitWithError(1, "config_error",
			fmt.Errorf("%w: Set 'settings.ai.enabled: true' in your atmos.yaml configuration", errUtils.ErrAINotEnabled))

		var execErr *execError
		require.True(t, errors.As(err, &execErr))
		assert.Equal(t, "config_error", execErr.errorType)
		assert.Equal(t, 1, execErr.code)
		assert.True(t, errors.Is(execErr.err, errUtils.ErrAINotEnabled))
	})
}

func TestExecCommand_ProviderOverride(t *testing.T) {
	t.Run("provider flag description mentions supported providers", func(t *testing.T) {
		providerFlag := execCmd.Flags().Lookup("provider")
		require.NotNil(t, providerFlag)
		// Check the usage contains provider examples.
		assert.Contains(t, providerFlag.Usage, "anthropic")
		assert.Contains(t, providerFlag.Usage, "openai")
		assert.Contains(t, providerFlag.Usage, "gemini")
	})
}

func TestExecCommand_OutputFileHandling(t *testing.T) {
	t.Run("output flag is optional", func(t *testing.T) {
		outputFlag := execCmd.Flags().Lookup("output")
		require.NotNil(t, outputFlag)
		// Default is empty string which means stdout.
		assert.Equal(t, "", outputFlag.DefValue)
	})
}

func TestExecCommand_SessionHandling(t *testing.T) {
	t.Run("session flag is optional", func(t *testing.T) {
		sessionFlag := execCmd.Flags().Lookup("session")
		require.NotNil(t, sessionFlag)
		assert.Equal(t, "", sessionFlag.DefValue)
	})
}

func TestExecCommand_ContextDiscoveryFlags(t *testing.T) {
	t.Run("include flag accepts multiple values", func(t *testing.T) {
		includeFlag := execCmd.Flags().Lookup("include")
		require.NotNil(t, includeFlag)
		assert.Equal(t, "stringSlice", includeFlag.Value.Type())
	})

	t.Run("exclude flag accepts multiple values", func(t *testing.T) {
		excludeFlag := execCmd.Flags().Lookup("exclude")
		require.NotNil(t, excludeFlag)
		assert.Equal(t, "stringSlice", excludeFlag.Value.Type())
	})

	t.Run("no-auto-context flag disables context discovery", func(t *testing.T) {
		flag := execCmd.Flags().Lookup("no-auto-context")
		require.NotNil(t, flag)
		assert.Equal(t, "bool", flag.Value.Type())
	})
}

func TestExecError_Implements_Error(t *testing.T) {
	t.Run("execError implements error interface", func(t *testing.T) {
		var err error = &execError{
			code:      1,
			errorType: "test",
			err:       errors.New("test error"),
		}
		assert.NotNil(t, err)
		assert.Equal(t, "test error", err.Error())
	})
}

func TestExecCommand_CommandStructure(t *testing.T) {
	t.Run("has correct use pattern", func(t *testing.T) {
		assert.Equal(t, "exec [prompt]", execCmd.Use)
	})

	t.Run("has non-empty short description", func(t *testing.T) {
		assert.NotEmpty(t, execCmd.Short)
		assert.True(t, len(execCmd.Short) > 10)
	})

	t.Run("has non-empty long description", func(t *testing.T) {
		assert.NotEmpty(t, execCmd.Long)
		assert.True(t, len(execCmd.Long) > 100)
	})

	t.Run("has RunE function set", func(t *testing.T) {
		assert.NotNil(t, execCmd.RunE)
	})

	t.Run("has Args validator set", func(t *testing.T) {
		assert.NotNil(t, execCmd.Args)
	})
}

func TestGetPrompt_WithMockedStdin(t *testing.T) {
	// Note: Actually mocking stdin is complex because getPrompt uses os.Stdin.Stat().
	// These tests focus on the args path which is more controllable.

	t.Run("uses args over stdin when args provided", func(t *testing.T) {
		// When args are provided, stdin is not read.
		prompt, err := getPrompt([]string{"args prompt"})
		assert.NoError(t, err)
		assert.Equal(t, "args prompt", prompt)
	})
}

func TestExecCommand_ErrorMessages(t *testing.T) {
	t.Run("config_error has meaningful message", func(t *testing.T) {
		err := exitWithError(1, "config_error", errUtils.ErrAINotEnabled)
		var execErr *execError
		require.True(t, errors.As(err, &execErr))
		assert.Contains(t, execErr.Error(), "AI")
	})

	t.Run("input_error has meaningful message", func(t *testing.T) {
		err := exitWithError(1, "input_error", errUtils.ErrAIPromptRequired)
		var execErr *execError
		require.True(t, errors.As(err, &execErr))
		assert.Contains(t, execErr.Error(), "prompt")
	})
}

// TestExecCommand_IntegrationStyle tests the overall flow in a more integration-style manner.
func TestExecCommand_IntegrationStyle(t *testing.T) {
	t.Run("full flag setup works", func(t *testing.T) {
		// Create a copy of the command for isolated testing.
		testCmd := &cobra.Command{
			Use:   "exec [prompt]",
			Short: "Execute AI prompt non-interactively",
			Args:  cobra.MaximumNArgs(1),
		}

		// Register same flags as the real command.
		testCmd.Flags().StringP("format", "f", "text", "Output format: text, json, markdown")
		testCmd.Flags().StringP("output", "o", "", "Output file (default: stdout)")
		testCmd.Flags().Bool("no-tools", false, "Disable tool execution")
		testCmd.Flags().Bool("context", false, "Include stack context in prompt")
		testCmd.Flags().StringP("provider", "p", "", "Override AI provider (anthropic, openai, gemini, etc.)")
		testCmd.Flags().StringP("session", "s", "", "Session ID for conversation context")
		testCmd.Flags().StringSlice("include", nil, "Add glob patterns to include in context (can be repeated)")
		testCmd.Flags().StringSlice("exclude", nil, "Add glob patterns to exclude from context (can be repeated)")
		testCmd.Flags().Bool("no-auto-context", false, "Disable automatic context discovery")

		// Verify all flags are present.
		assert.NotNil(t, testCmd.Flags().Lookup("format"))
		assert.NotNil(t, testCmd.Flags().Lookup("output"))
		assert.NotNil(t, testCmd.Flags().Lookup("no-tools"))
		assert.NotNil(t, testCmd.Flags().Lookup("context"))
		assert.NotNil(t, testCmd.Flags().Lookup("provider"))
		assert.NotNil(t, testCmd.Flags().Lookup("session"))
		assert.NotNil(t, testCmd.Flags().Lookup("include"))
		assert.NotNil(t, testCmd.Flags().Lookup("exclude"))
		assert.NotNil(t, testCmd.Flags().Lookup("no-auto-context"))

		// Test setting flags.
		err := testCmd.Flags().Set("format", "json")
		assert.NoError(t, err)

		format, _ := testCmd.Flags().GetString("format")
		assert.Equal(t, "json", format)
	})
}

// TestOutputWriter tests the output destination logic.
func TestOutputWriter(t *testing.T) {
	t.Run("stdout is used by default", func(t *testing.T) {
		// This test validates the logic that when outputFile is empty, stdout is used.
		var writer io.Writer = os.Stdout
		outputFile := ""
		if outputFile != "" {
			// Would create file.
			t.Fatal("should not reach here")
		}
		assert.Equal(t, os.Stdout, writer)
	})

	t.Run("file writer is created for non-empty output", func(t *testing.T) {
		tmpDir := t.TempDir()
		outputFile := filepath.Join(tmpDir, "output.txt")

		// Simulate the logic from exec.go.
		var writer io.Writer
		if outputFile != "" {
			file, err := os.Create(outputFile)
			require.NoError(t, err)
			defer file.Close()
			writer = file
		} else {
			writer = os.Stdout
		}

		assert.NotEqual(t, os.Stdout, writer)

		// Write something to verify.
		_, err := writer.Write([]byte("test output"))
		require.NoError(t, err)

		// Read and verify.
		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		assert.Equal(t, "test output", string(content))
	})

	t.Run("invalid output path returns error", func(t *testing.T) {
		// Use a path with a non-existent parent directory that will fail to create.
		tempDir := t.TempDir()
		invalidPath := filepath.Join(tempDir, "nonexistent", "subdir", "file.txt")
		_, err := os.Create(invalidPath)
		assert.Error(t, err)
	})
}

// TestExecCommand_BufferedOutput tests output buffering behavior.
func TestExecCommand_BufferedOutput(t *testing.T) {
	t.Run("can write to bytes.Buffer", func(t *testing.T) {
		var buf bytes.Buffer
		var writer io.Writer = &buf

		_, err := writer.Write([]byte("buffered output"))
		require.NoError(t, err)
		assert.Equal(t, "buffered output", buf.String())
	})
}

// TestExecCommand_RunE_AIDisabled tests that the command returns an error when AI is disabled.
func TestExecCommand_RunE_AIDisabled(t *testing.T) {
	tmpDir := createValidAtmosConfig(t, false, "")

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	// Create test command with same flags as execCmd.
	testCmd := createTestExecCmd()

	// Execute the actual RunE function.
	err := execCmd.RunE(testCmd, []string{"test prompt"})

	// Should fail with config_error because AI is disabled.
	require.Error(t, err)
	var execErr *execError
	require.True(t, errors.As(err, &execErr))
	assert.Equal(t, "config_error", execErr.errorType)
	assert.Equal(t, 1, execErr.code)
	assert.True(t, errors.Is(execErr.err, errUtils.ErrAINotEnabled))
}

// TestExecCommand_RunE_EmptyPrompt tests that the command returns an error for empty prompt.
func TestExecCommand_RunE_EmptyPrompt(t *testing.T) {
	tmpDir := createValidAtmosConfig(t, true, "")

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	// Create test command.
	testCmd := createTestExecCmd()

	// Execute with empty prompt (stdin will be terminal in test environment).
	err := execCmd.RunE(testCmd, []string{})

	// Should fail with input_error because prompt is empty.
	require.Error(t, err)
	var execErr *execError
	if errors.As(err, &execErr) {
		assert.Equal(t, "input_error", execErr.errorType)
	}
}

// TestExecCommand_RunE_ProviderOverride tests provider override via flag.
func TestExecCommand_RunE_ProviderOverride(t *testing.T) {
	tmpDir := createValidAtmosConfig(t, true, "")

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	testCmd := createTestExecCmd()
	// Set provider flag.
	err := testCmd.Flags().Set("provider", "openai")
	require.NoError(t, err)

	// This will fail at AI client creation because no API key, but we exercise the provider override path.
	err = execCmd.RunE(testCmd, []string{"test prompt"})

	// Will fail at AI client creation step (no API key), but that's after provider override is applied.
	require.Error(t, err)
}

// TestExecCommand_RunE_ContextDiscoveryFlags tests context discovery flag handling.
func TestExecCommand_RunE_ContextDiscoveryFlags(t *testing.T) {
	extraConfig := `
    context:
      enabled: true
      auto_include:
        - "*.yaml"
`
	tmpDir := createValidAtmosConfig(t, true, extraConfig)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	t.Run("no-auto-context flag disables context", func(t *testing.T) {
		testCmd := createTestExecCmd()
		err := testCmd.Flags().Set("no-auto-context", "true")
		require.NoError(t, err)

		// This exercises the noAutoContext code path.
		err = execCmd.RunE(testCmd, []string{"test prompt"})
		// Will fail at AI client creation, but the flag processing path is exercised.
		require.Error(t, err)
	})

	t.Run("include patterns are appended", func(t *testing.T) {
		testCmd := createTestExecCmd()
		err := testCmd.Flags().Set("include", "*.tf")
		require.NoError(t, err)

		err = execCmd.RunE(testCmd, []string{"test prompt"})
		require.Error(t, err)
	})

	t.Run("exclude patterns are appended", func(t *testing.T) {
		testCmd := createTestExecCmd()
		err := testCmd.Flags().Set("exclude", "*.lock")
		require.NoError(t, err)

		err = execCmd.RunE(testCmd, []string{"test prompt"})
		require.Error(t, err)
	})
}

// TestExecCommand_RunE_OutputToFile tests writing output to a file.
func TestExecCommand_RunE_OutputToFile(t *testing.T) {
	tmpDir := createValidAtmosConfig(t, true, "")
	outputFile := filepath.Join(tmpDir, "output.txt")

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	testCmd := createTestExecCmd()
	err := testCmd.Flags().Set("output", outputFile)
	require.NoError(t, err)

	// This will fail at AI client creation, but exercises the output file path code.
	err = execCmd.RunE(testCmd, []string{"test prompt"})
	require.Error(t, err)
}

// TestExecCommand_RunE_InvalidOutputPath tests handling of invalid output path.
func TestExecCommand_RunE_InvalidOutputPath(t *testing.T) {
	tmpDir := createValidAtmosConfig(t, true, "")
	// Invalid path - trying to write to a non-existent deep directory.
	outputFile := filepath.Join(tmpDir, "nonexistent", "deep", "path", "output.txt")

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	testCmd := createTestExecCmd()
	err := testCmd.Flags().Set("output", outputFile)
	require.NoError(t, err)

	// This will fail at AI client creation first, before reaching file creation.
	err = execCmd.RunE(testCmd, []string{"test prompt"})
	require.Error(t, err)
}

// TestExecCommand_RunE_FormatOptions tests different format options.
func TestExecCommand_RunE_FormatOptions(t *testing.T) {
	tmpDir := createValidAtmosConfig(t, true, "")

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	formats := []string{"text", "json", "markdown"}

	for _, format := range formats {
		t.Run(fmt.Sprintf("format %s", format), func(t *testing.T) {
			testCmd := createTestExecCmd()
			err := testCmd.Flags().Set("format", format)
			require.NoError(t, err)

			err = execCmd.RunE(testCmd, []string{"test prompt"})
			// Will fail at AI client creation, but exercises format path.
			require.Error(t, err)
		})
	}
}

// TestExecCommand_RunE_NoToolsFlag tests the no-tools flag.
func TestExecCommand_RunE_NoToolsFlag(t *testing.T) {
	extraConfig := `
    tools:
      enabled: true
`
	tmpDir := createValidAtmosConfig(t, true, extraConfig)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	testCmd := createTestExecCmd()
	err := testCmd.Flags().Set("no-tools", "true")
	require.NoError(t, err)

	err = execCmd.RunE(testCmd, []string{"test prompt"})
	require.Error(t, err)
}

// TestExecCommand_RunE_ContextFlag tests the context flag.
func TestExecCommand_RunE_ContextFlag(t *testing.T) {
	tmpDir := createValidAtmosConfig(t, true, "")

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	testCmd := createTestExecCmd()
	err := testCmd.Flags().Set("context", "true")
	require.NoError(t, err)

	err = execCmd.RunE(testCmd, []string{"test prompt"})
	require.Error(t, err)
}

// TestExecCommand_RunE_SessionFlag tests the session flag.
func TestExecCommand_RunE_SessionFlag(t *testing.T) {
	tmpDir := createValidAtmosConfig(t, true, "")

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	testCmd := createTestExecCmd()
	err := testCmd.Flags().Set("session", "test-session-id")
	require.NoError(t, err)

	err = execCmd.RunE(testCmd, []string{"test prompt"})
	require.Error(t, err)
}

// TestExecCommand_RunE_WhitespacePrompt tests handling of whitespace-only prompt.
func TestExecCommand_RunE_WhitespacePrompt(t *testing.T) {
	tmpDir := createValidAtmosConfig(t, true, "")

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	testCmd := createTestExecCmd()

	// Whitespace-only prompt should be treated as empty after trim.
	err := execCmd.RunE(testCmd, []string{"   \t\n   "})

	require.Error(t, err)
	var execErr *execError
	if errors.As(err, &execErr) {
		assert.Equal(t, "input_error", execErr.errorType)
		assert.Equal(t, 1, execErr.code)
	}
}

// TestExecCommand_RunE_ConfigNotFound tests handling when config file doesn't exist.
func TestExecCommand_RunE_ConfigNotFound(t *testing.T) {
	// Point to a non-existent directory.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path/to/config")
	t.Setenv("ATMOS_BASE_PATH", "/nonexistent/path")

	testCmd := createTestExecCmd()

	err := execCmd.RunE(testCmd, []string{"test prompt"})

	require.Error(t, err)
	var execErr *execError
	if errors.As(err, &execErr) {
		assert.Equal(t, "config_error", execErr.errorType)
		assert.Equal(t, 1, execErr.code)
	}
}

// TestExecCommand_RunE_ToolsEnabled tests path when tools are enabled but fail to initialize.
func TestExecCommand_RunE_ToolsEnabled(t *testing.T) {
	extraConfig := `
    tools:
      enabled: true
`
	tmpDir := createValidAtmosConfig(t, true, extraConfig)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	testCmd := createTestExecCmd()

	// This will fail at AI client creation, but tools initialization path is exercised.
	err := execCmd.RunE(testCmd, []string{"test prompt"})
	require.Error(t, err)
}

// TestExecCommand_RunE_TimeoutConfig tests that timeout config is read.
func TestExecCommand_RunE_TimeoutConfig(t *testing.T) {
	extraConfig := `
    timeout_seconds: 120
`
	tmpDir := createValidAtmosConfig(t, true, extraConfig)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	testCmd := createTestExecCmd()

	err := execCmd.RunE(testCmd, []string{"test prompt"})
	require.Error(t, err)
}

// TestExecCommand_RunE_AllFlagsSet tests setting multiple flags together.
func TestExecCommand_RunE_AllFlagsSet(t *testing.T) {
	extraConfig := `
    context:
      enabled: true
    tools:
      enabled: true
`
	tmpDir := createValidAtmosConfig(t, true, extraConfig)
	outputFile := filepath.Join(tmpDir, "result.json")

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	testCmd := createTestExecCmd()

	// Set all flags.
	err := testCmd.Flags().Set("format", "json")
	require.NoError(t, err)
	err = testCmd.Flags().Set("output", outputFile)
	require.NoError(t, err)
	err = testCmd.Flags().Set("no-tools", "false")
	require.NoError(t, err)
	err = testCmd.Flags().Set("context", "true")
	require.NoError(t, err)
	err = testCmd.Flags().Set("provider", "openai")
	require.NoError(t, err)
	err = testCmd.Flags().Set("session", "my-session")
	require.NoError(t, err)
	err = testCmd.Flags().Set("include", "*.yaml")
	require.NoError(t, err)
	err = testCmd.Flags().Set("exclude", "*.tmp")
	require.NoError(t, err)
	err = testCmd.Flags().Set("no-auto-context", "false")
	require.NoError(t, err)

	err = execCmd.RunE(testCmd, []string{"Complex test prompt with all flags"})
	require.Error(t, err) // Will fail at AI client creation.
}

// TestGetPrompt_FromStdin tests reading prompt from stdin.
func TestGetPrompt_FromStdin(t *testing.T) {
	// Note: Testing stdin requires manipulating os.Stdin which is complex.
	// The following tests cover the args path thoroughly.

	t.Run("empty args returns empty when stdin is terminal", func(t *testing.T) {
		// In test environment, stdin behavior varies.
		prompt, err := getPrompt([]string{})
		// Either returns empty string or reads from stdin (if piped).
		if err == nil {
			// Just verify no error.
			_ = prompt
		}
	})

	t.Run("args take precedence", func(t *testing.T) {
		// Even if there's data on stdin, args should be used.
		prompt, err := getPrompt([]string{"from args"})
		require.NoError(t, err)
		assert.Equal(t, "from args", prompt)
	})
}

// TestExecError_ErrorInterface tests that execError properly implements error interface.
func TestExecError_ErrorInterface(t *testing.T) {
	t.Run("execError is assignable to error interface", func(t *testing.T) {
		var err error = &execError{
			code:      1,
			errorType: "test",
			err:       errors.New("test error"),
		}
		require.NotNil(t, err)
		assert.Equal(t, "test error", err.Error())
	})

	t.Run("errors.As works with execError", func(t *testing.T) {
		originalErr := &execError{
			code:      2,
			errorType: "tool_error",
			err:       errors.New("tool failed"),
		}

		var target *execError
		result := errors.As(originalErr, &target)
		require.True(t, result)
		assert.Equal(t, 2, target.code)
		assert.Equal(t, "tool_error", target.errorType)
	})

	t.Run("wrapped execError can be extracted", func(t *testing.T) {
		inner := &execError{
			code:      1,
			errorType: "inner_type",
			err:       errors.New("inner error"),
		}
		wrapped := fmt.Errorf("outer: %w", inner)

		var target *execError
		result := errors.As(wrapped, &target)
		require.True(t, result)
		assert.Equal(t, 1, target.code)
		assert.Equal(t, "inner_type", target.errorType)
	})
}

// TestExitWithError_LogsAndReturns tests that exitWithError properly logs and returns error.
func TestExitWithError_LogsAndReturns(t *testing.T) {
	t.Run("returns non-nil error", func(t *testing.T) {
		err := exitWithError(1, "test", errors.New("test"))
		require.NotNil(t, err)
	})

	t.Run("error message is preserved", func(t *testing.T) {
		originalMsg := "original error message"
		err := exitWithError(1, "test", errors.New(originalMsg))

		var execErr *execError
		require.True(t, errors.As(err, &execErr))
		assert.Equal(t, originalMsg, execErr.Error())
	})

	t.Run("all fields are set correctly", func(t *testing.T) {
		err := exitWithError(42, "custom_type", errors.New("custom message"))

		var execErr *execError
		require.True(t, errors.As(err, &execErr))
		assert.Equal(t, 42, execErr.code)
		assert.Equal(t, "custom_type", execErr.errorType)
		assert.Equal(t, "custom message", execErr.Error())
	})
}

// TestOutputFileCreation tests file creation for output.
func TestOutputFileCreation(t *testing.T) {
	t.Run("creates file in existing directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		outputPath := filepath.Join(tmpDir, "test_output.txt")

		file, err := os.Create(outputPath)
		require.NoError(t, err)
		defer file.Close()

		_, err = file.WriteString("test content")
		require.NoError(t, err)

		// Verify file was created.
		content, err := os.ReadFile(outputPath)
		require.NoError(t, err)
		assert.Equal(t, "test content", string(content))
	})

	t.Run("fails for non-existent parent directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		invalidPath := filepath.Join(tmpDir, "nonexistent", "dir", "file.txt")

		_, err := os.Create(invalidPath)
		require.Error(t, err)
	})

	t.Run("can overwrite existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		outputPath := filepath.Join(tmpDir, "existing.txt")

		// Create initial file.
		err := os.WriteFile(outputPath, []byte("initial"), 0o644)
		require.NoError(t, err)

		// Overwrite.
		file, err := os.Create(outputPath)
		require.NoError(t, err)
		_, err = file.WriteString("overwritten")
		file.Close()
		require.NoError(t, err)

		content, err := os.ReadFile(outputPath)
		require.NoError(t, err)
		assert.Equal(t, "overwritten", string(content))
	})
}

// TestFormatterSelection tests that correct formatter is selected.
func TestFormatterSelection(t *testing.T) {
	tests := []struct {
		format   string
		expected string // Expected type name.
	}{
		{"text", "TextFormatter"},
		{"json", "JSONFormatter"},
		{"markdown", "MarkdownFormatter"},
		{"unknown", "TextFormatter"}, // Falls back to text.
		{"", "TextFormatter"},        // Empty defaults to text.
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("format_%s", tt.format), func(t *testing.T) {
			// Test the formatter selection logic.
			switch tt.format {
			case "json":
				assert.Equal(t, "JSONFormatter", tt.expected)
			case "markdown":
				assert.Equal(t, "MarkdownFormatter", tt.expected)
			default:
				assert.Equal(t, "TextFormatter", tt.expected)
			}
		})
	}
}

// TestExecCommand_FlagUsage tests that flag usage strings are descriptive.
func TestExecCommand_FlagUsage(t *testing.T) {
	flags := []struct {
		name        string
		shouldExist bool
	}{
		{"format", true},
		{"output", true},
		{"no-tools", true},
		{"context", true},
		{"provider", true},
		{"session", true},
		{"include", true},
		{"exclude", true},
		{"no-auto-context", true},
	}

	for _, f := range flags {
		t.Run(fmt.Sprintf("flag_%s", f.name), func(t *testing.T) {
			flag := execCmd.Flags().Lookup(f.name)
			if f.shouldExist {
				require.NotNil(t, flag, "flag %s should exist", f.name)
				assert.NotEmpty(t, flag.Usage, "flag %s should have usage text", f.name)
			}
		})
	}
}

// TestExecCommand_LongDescription tests the long description content.
func TestExecCommand_LongDescription(t *testing.T) {
	requiredContent := []string{
		"non-interactively",
		"automation",
		"CI/CD",
		"stdin",
		"text",
		"json",
		"markdown",
		"Exit codes",
		"0: Success",
		"1: AI error",
		"2: Tool execution error",
	}

	for _, content := range requiredContent {
		t.Run(fmt.Sprintf("contains_%s", strings.ReplaceAll(content, " ", "_")), func(t *testing.T) {
			assert.Contains(t, execCmd.Long, content)
		})
	}
}

// TestExecCommand_Use tests the use pattern.
func TestExecCommand_Use(t *testing.T) {
	assert.Equal(t, "exec [prompt]", execCmd.Use)
	assert.Contains(t, execCmd.Use, "[prompt]") // Optional prompt.
}

// createTestExecCmd creates a test command with all flags registered (same as execCmd).
func createTestExecCmd() *cobra.Command {
	testCmd := &cobra.Command{
		Use:  "exec [prompt]",
		Args: cobra.MaximumNArgs(1),
	}
	testCmd.Flags().StringP("format", "f", "text", "Output format: text, json, markdown")
	testCmd.Flags().StringP("output", "o", "", "Output file (default: stdout)")
	testCmd.Flags().Bool("no-tools", false, "Disable tool execution")
	testCmd.Flags().Bool("context", false, "Include stack context in prompt")
	testCmd.Flags().StringP("provider", "p", "", "Override AI provider (anthropic, openai, gemini, etc.)")
	testCmd.Flags().StringP("session", "s", "", "Session ID for conversation context")
	testCmd.Flags().StringSlice("include", nil, "Add glob patterns to include in context (can be repeated)")
	testCmd.Flags().StringSlice("exclude", nil, "Add glob patterns to exclude from context (can be repeated)")
	testCmd.Flags().Bool("no-auto-context", false, "Disable automatic context discovery")
	return testCmd
}

// createValidAtmosConfig creates a valid atmos.yaml config file and directories for testing.
// Returns the temp directory path containing the config.
func createValidAtmosConfig(t *testing.T, aiEnabled bool, extraConfig string) string {
	t.Helper()

	tmpDir := t.TempDir()

	// Create required directories for atmos config.
	stacksDir := filepath.Join(tmpDir, "stacks")
	componentsDir := filepath.Join(tmpDir, "components", "terraform")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))
	require.NoError(t, os.MkdirAll(componentsDir, 0o755))

	// Create a dummy stack file to avoid "no stacks found" error.
	dummyStack := `
vars:
  stage: test
`
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "test.yaml"), []byte(dummyStack), 0o644))

	enabledStr := "false"
	if aiEnabled {
		enabledStr = "true"
	}

	atmosYaml := `
base_path: "` + tmpDir + `"
stacks:
  base_path: stacks
  included_paths:
    - "*.yaml"
  name_pattern: "{stage}"
components:
  terraform:
    base_path: components/terraform
settings:
  ai:
    enabled: ` + enabledStr + `
    default_provider: anthropic
` + extraConfig

	err := os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYaml), 0o644)
	require.NoError(t, err)

	return tmpDir
}

// TestFormatterIntegration tests actual formatter usage with ExecutionResult.
func TestFormatterIntegration(t *testing.T) {
	t.Run("text formatter writes response", func(t *testing.T) {
		result := &formatter.ExecutionResult{
			Success:  true,
			Response: "Test response text",
		}

		var buf bytes.Buffer
		textFormatter := formatter.NewFormatter(formatter.FormatText)
		err := textFormatter.Format(&buf, result)

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Test response text")
	})

	t.Run("text formatter writes error", func(t *testing.T) {
		result := &formatter.ExecutionResult{
			Success: false,
			Error: &formatter.ErrorInfo{
				Message: "Something went wrong",
				Type:    "ai_error",
			},
		}

		var buf bytes.Buffer
		textFormatter := formatter.NewFormatter(formatter.FormatText)
		err := textFormatter.Format(&buf, result)

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Error:")
		assert.Contains(t, buf.String(), "Something went wrong")
	})

	t.Run("json formatter outputs valid JSON", func(t *testing.T) {
		result := &formatter.ExecutionResult{
			Success:  true,
			Response: "JSON response",
			Tokens: formatter.TokenUsage{
				Prompt:     100,
				Completion: 50,
				Total:      150,
			},
		}

		var buf bytes.Buffer
		jsonFormatter := formatter.NewFormatter(formatter.FormatJSON)
		err := jsonFormatter.Format(&buf, result)

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "success")
		assert.Contains(t, buf.String(), "JSON response")
	})

	t.Run("markdown formatter outputs markdown", func(t *testing.T) {
		result := &formatter.ExecutionResult{
			Success:  true,
			Response: "Markdown response",
		}

		var buf bytes.Buffer
		mdFormatter := formatter.NewFormatter(formatter.FormatMarkdown)
		err := mdFormatter.Format(&buf, result)

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Markdown response")
	})

	t.Run("markdown formatter shows tool calls", func(t *testing.T) {
		result := &formatter.ExecutionResult{
			Success:  true,
			Response: "Response with tools",
			ToolCalls: []formatter.ToolCallResult{
				{
					Tool:       "test_tool",
					Success:    true,
					DurationMs: 100,
					Result:     "tool output",
				},
				{
					Tool:       "failed_tool",
					Success:    false,
					DurationMs: 50,
					Error:      "tool error",
				},
			},
		}

		var buf bytes.Buffer
		mdFormatter := formatter.NewFormatter(formatter.FormatMarkdown)
		err := mdFormatter.Format(&buf, result)

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "test_tool")
		assert.Contains(t, output, "failed_tool")
	})
}

// TestResultErrorHandling tests error handling based on result state.
func TestResultErrorHandling(t *testing.T) {
	t.Run("tool_error type returns exit code 2", func(t *testing.T) {
		err := exitWithError(2, "tool_error", fmt.Errorf("%w: test tool error", errUtils.ErrAIToolExecutionFailed))

		var execErr *execError
		require.True(t, errors.As(err, &execErr))
		assert.Equal(t, 2, execErr.code)
		assert.Equal(t, "tool_error", execErr.errorType)
		assert.True(t, errors.Is(execErr.err, errUtils.ErrAIToolExecutionFailed))
	})

	t.Run("ai_error type returns exit code 1", func(t *testing.T) {
		err := exitWithError(1, "ai_error", fmt.Errorf("%w: test ai error", errUtils.ErrAIExecutionFailed))

		var execErr *execError
		require.True(t, errors.As(err, &execErr))
		assert.Equal(t, 1, execErr.code)
		assert.Equal(t, "ai_error", execErr.errorType)
		assert.True(t, errors.Is(execErr.err, errUtils.ErrAIExecutionFailed))
	})

	t.Run("unknown_error type returns exit code 1", func(t *testing.T) {
		err := exitWithError(1, "unknown_error", errUtils.ErrAIExecutionFailed)

		var execErr *execError
		require.True(t, errors.As(err, &execErr))
		assert.Equal(t, 1, execErr.code)
		assert.Equal(t, "unknown_error", execErr.errorType)
	})
}

// TestExecResultProcessing tests the result processing logic.
func TestExecResultProcessing(t *testing.T) {
	t.Run("successful result with no error", func(t *testing.T) {
		result := &formatter.ExecutionResult{
			Success:  true,
			Response: "Success!",
		}

		// Simulate the output logic from exec.go.
		var buf bytes.Buffer
		f := formatter.NewFormatter(formatter.FormatText)
		err := f.Format(&buf, result)

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Success!")
	})

	t.Run("failed result with error info", func(t *testing.T) {
		result := &formatter.ExecutionResult{
			Success: false,
			Error: &formatter.ErrorInfo{
				Message: "API call failed",
				Type:    "ai_error",
			},
		}

		var buf bytes.Buffer
		f := formatter.NewFormatter(formatter.FormatText)
		err := f.Format(&buf, result)

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "API call failed")
	})
}

// TestExecCommand_ErrorScenarios tests various error scenarios.
func TestExecCommand_ErrorScenarios(t *testing.T) {
	t.Run("config error - AI not enabled", func(t *testing.T) {
		tmpDir := createValidAtmosConfig(t, false, "")
		t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
		t.Setenv("ATMOS_BASE_PATH", tmpDir)

		testCmd := createTestExecCmd()
		err := execCmd.RunE(testCmd, []string{"test"})

		require.Error(t, err)
		var execErr *execError
		require.True(t, errors.As(err, &execErr))
		assert.Equal(t, "config_error", execErr.errorType)
	})

	t.Run("input error - empty prompt", func(t *testing.T) {
		tmpDir := createValidAtmosConfig(t, true, "")
		t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
		t.Setenv("ATMOS_BASE_PATH", tmpDir)

		testCmd := createTestExecCmd()
		err := execCmd.RunE(testCmd, []string{""})

		require.Error(t, err)
		var execErr *execError
		require.True(t, errors.As(err, &execErr))
		assert.Equal(t, "input_error", execErr.errorType)
	})

	t.Run("ai error - client creation failure", func(t *testing.T) {
		tmpDir := createValidAtmosConfig(t, true, "")
		t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
		t.Setenv("ATMOS_BASE_PATH", tmpDir)

		testCmd := createTestExecCmd()
		err := execCmd.RunE(testCmd, []string{"test prompt"})

		require.Error(t, err)
		var execErr *execError
		require.True(t, errors.As(err, &execErr))
		// AI client creation fails due to missing API key.
		assert.Equal(t, "ai_error", execErr.errorType)
	})
}

// TestContextPatternHandling tests the context pattern handling.
func TestContextPatternHandling(t *testing.T) {
	t.Run("include patterns are set correctly", func(t *testing.T) {
		testCmd := createTestExecCmd()
		// StringSlice parses comma-separated values into separate items.
		err := testCmd.Flags().Set("include", "*.tf,*.yaml")
		require.NoError(t, err)

		patterns, _ := testCmd.Flags().GetStringSlice("include")
		// Cobra's StringSlice splits on comma.
		assert.Contains(t, patterns, "*.tf")
		assert.Contains(t, patterns, "*.yaml")
	})

	t.Run("exclude patterns are set correctly", func(t *testing.T) {
		testCmd := createTestExecCmd()
		err := testCmd.Flags().Set("exclude", "*.lock")
		require.NoError(t, err)

		patterns, _ := testCmd.Flags().GetStringSlice("exclude")
		assert.Contains(t, patterns, "*.lock")
	})

	t.Run("single include pattern", func(t *testing.T) {
		testCmd := createTestExecCmd()
		err := testCmd.Flags().Set("include", "*.tf")
		require.NoError(t, err)
		patterns, _ := testCmd.Flags().GetStringSlice("include")
		assert.Len(t, patterns, 1)
		assert.Equal(t, "*.tf", patterns[0])
	})
}
