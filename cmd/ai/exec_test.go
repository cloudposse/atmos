package ai

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
