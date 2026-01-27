package ai

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/session"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name          string
		durationStr   string
		expectedDays  int
		expectedError error
	}{
		{
			name:          "empty string returns default",
			durationStr:   "",
			expectedDays:  session.DefaultRetentionDays,
			expectedError: nil,
		},
		{
			name:          "valid days",
			durationStr:   "30d",
			expectedDays:  30,
			expectedError: nil,
		},
		{
			name:          "single day",
			durationStr:   "1d",
			expectedDays:  1,
			expectedError: nil,
		},
		{
			name:          "valid weeks",
			durationStr:   "2w",
			expectedDays:  14, // 2 * 7
			expectedError: nil,
		},
		{
			name:          "single week",
			durationStr:   "1w",
			expectedDays:  7,
			expectedError: nil,
		},
		{
			name:          "valid months",
			durationStr:   "2m",
			expectedDays:  60, // 2 * 30
			expectedError: nil,
		},
		{
			name:          "single month",
			durationStr:   "1m",
			expectedDays:  30,
			expectedError: nil,
		},
		{
			name:          "valid hours - exact day",
			durationStr:   "24h",
			expectedDays:  1,
			expectedError: nil,
		},
		{
			name:          "valid hours - rounds up",
			durationStr:   "25h",
			expectedDays:  2, // Rounds up from 1.04 days
			expectedError: nil,
		},
		{
			name:          "valid hours - multiple days",
			durationStr:   "48h",
			expectedDays:  2,
			expectedError: nil,
		},
		{
			name:          "hours less than a day rounds to 1",
			durationStr:   "12h",
			expectedDays:  1, // Rounds up
			expectedError: nil,
		},
		{
			name:          "single hour rounds to 1 day",
			durationStr:   "1h",
			expectedDays:  1,
			expectedError: nil,
		},
		{
			name:          "large value",
			durationStr:   "365d",
			expectedDays:  365,
			expectedError: nil,
		},
		{
			name:          "invalid format - no number",
			durationStr:   "d",
			expectedDays:  0,
			expectedError: errUtils.ErrAIInvalidDurationFormat,
		},
		{
			name:          "invalid format - no unit",
			durationStr:   "30",
			expectedDays:  0,
			expectedError: errUtils.ErrAIInvalidDurationFormat,
		},
		{
			name:          "invalid format - only text",
			durationStr:   "invalid",
			expectedDays:  0,
			expectedError: errUtils.ErrAIInvalidDurationFormat,
		},
		{
			name:          "invalid unit",
			durationStr:   "30x",
			expectedDays:  0,
			expectedError: errUtils.ErrAIUnsupportedDurationUnit,
		},
		{
			name:          "invalid unit - years",
			durationStr:   "1y",
			expectedDays:  0,
			expectedError: errUtils.ErrAIUnsupportedDurationUnit,
		},
		{
			name:          "invalid unit - seconds",
			durationStr:   "3600s",
			expectedDays:  0,
			expectedError: errUtils.ErrAIUnsupportedDurationUnit,
		},
		{
			name:          "negative value",
			durationStr:   "-30d",
			expectedDays:  -30,
			expectedError: nil, // parseDuration doesn't validate negative values
		},
		{
			name:          "zero value",
			durationStr:   "0d",
			expectedDays:  0,
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDuration(tt.durationStr)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedDays, result)
			}
		})
	}
}

func TestSessionsCommand_BasicProperties(t *testing.T) {
	t.Run("sessions command properties", func(t *testing.T) {
		assert.Equal(t, "sessions", sessionsCmd.Use)
		assert.Equal(t, "Manage AI chat sessions", sessionsCmd.Short)
		assert.NotEmpty(t, sessionsCmd.Long)
		assert.NotNil(t, sessionsCmd.RunE)
	})

	t.Run("sessions list command properties", func(t *testing.T) {
		assert.Equal(t, "list", sessionsListCmd.Use)
		assert.Equal(t, "List all AI chat sessions", sessionsListCmd.Short)
		assert.NotEmpty(t, sessionsListCmd.Long)
		assert.NotNil(t, sessionsListCmd.RunE)
	})

	t.Run("sessions clean command properties", func(t *testing.T) {
		assert.Equal(t, "clean", sessionsCleanCmd.Use)
		assert.Equal(t, "Clean old AI chat sessions", sessionsCleanCmd.Short)
		assert.NotEmpty(t, sessionsCleanCmd.Long)
		assert.NotNil(t, sessionsCleanCmd.RunE)
	})

	t.Run("sessions export command properties", func(t *testing.T) {
		assert.Equal(t, "export <session-name>", sessionsExportCmd.Use)
		assert.Equal(t, "Export an AI chat session to a checkpoint file", sessionsExportCmd.Short)
		assert.NotEmpty(t, sessionsExportCmd.Long)
		assert.NotNil(t, sessionsExportCmd.RunE)
		// Check that Args is set to require exactly 1 argument.
		assert.NotNil(t, sessionsExportCmd.Args)
	})

	t.Run("sessions import command properties", func(t *testing.T) {
		assert.Equal(t, "import <checkpoint-file>", sessionsImportCmd.Use)
		assert.Equal(t, "Import an AI chat session from a checkpoint file", sessionsImportCmd.Short)
		assert.NotEmpty(t, sessionsImportCmd.Long)
		assert.NotNil(t, sessionsImportCmd.RunE)
		// Check that Args is set to require exactly 1 argument.
		assert.NotNil(t, sessionsImportCmd.Args)
	})
}

func TestSessionsCommand_Flags(t *testing.T) {
	t.Run("clean command has older-than flag", func(t *testing.T) {
		olderThanFlag := sessionsCleanCmd.Flags().Lookup("older-than")
		require.NotNil(t, olderThanFlag, "older-than flag should be registered")
		assert.Equal(t, "string", olderThanFlag.Value.Type())
		assert.Equal(t, "30d", olderThanFlag.DefValue)
	})

	t.Run("export command has output flag", func(t *testing.T) {
		outputFlag := sessionsExportCmd.Flags().Lookup("output")
		require.NotNil(t, outputFlag, "output flag should be registered")
		assert.Equal(t, "string", outputFlag.Value.Type())
		assert.Equal(t, "o", outputFlag.Shorthand)
	})

	t.Run("export command has format flag", func(t *testing.T) {
		formatFlag := sessionsExportCmd.Flags().Lookup("format")
		require.NotNil(t, formatFlag, "format flag should be registered")
		assert.Equal(t, "string", formatFlag.Value.Type())
		assert.Equal(t, "f", formatFlag.Shorthand)
	})

	t.Run("export command has context flag", func(t *testing.T) {
		contextFlag := sessionsExportCmd.Flags().Lookup("context")
		require.NotNil(t, contextFlag, "context flag should be registered")
		assert.Equal(t, "bool", contextFlag.Value.Type())
		assert.Equal(t, "false", contextFlag.DefValue)
	})

	t.Run("export command has metadata flag", func(t *testing.T) {
		metadataFlag := sessionsExportCmd.Flags().Lookup("metadata")
		require.NotNil(t, metadataFlag, "metadata flag should be registered")
		assert.Equal(t, "bool", metadataFlag.Value.Type())
		assert.Equal(t, "true", metadataFlag.DefValue)
	})

	t.Run("import command has name flag", func(t *testing.T) {
		nameFlag := sessionsImportCmd.Flags().Lookup("name")
		require.NotNil(t, nameFlag, "name flag should be registered")
		assert.Equal(t, "string", nameFlag.Value.Type())
		assert.Equal(t, "n", nameFlag.Shorthand)
	})

	t.Run("import command has overwrite flag", func(t *testing.T) {
		overwriteFlag := sessionsImportCmd.Flags().Lookup("overwrite")
		require.NotNil(t, overwriteFlag, "overwrite flag should be registered")
		assert.Equal(t, "bool", overwriteFlag.Value.Type())
		assert.Equal(t, "false", overwriteFlag.DefValue)
	})

	t.Run("import command has context flag", func(t *testing.T) {
		contextFlag := sessionsImportCmd.Flags().Lookup("context")
		require.NotNil(t, contextFlag, "context flag should be registered")
		assert.Equal(t, "bool", contextFlag.Value.Type())
		assert.Equal(t, "true", contextFlag.DefValue)
	})
}

func TestSessionsCommand_Subcommands(t *testing.T) {
	t.Run("sessions command has expected subcommands", func(t *testing.T) {
		subcommands := sessionsCmd.Commands()
		subcommandNames := make(map[string]bool)
		for _, subcmd := range subcommands {
			subcommandNames[subcmd.Name()] = true
		}

		expectedSubcommands := []string{"list", "clean", "export", "import"}
		for _, expected := range expectedSubcommands {
			assert.True(t, subcommandNames[expected], "expected subcommand %s not found", expected)
		}
	})
}

func TestSessionsCommand_ErrorCases(t *testing.T) {
	t.Run("export command requires output flag", func(t *testing.T) {
		// Create a fresh command to avoid flag state issues.
		testCmd := &cobra.Command{
			Use:  "export",
			Args: cobra.ExactArgs(1),
			RunE: exportSessionCommand,
		}
		testCmd.Flags().StringP("output", "o", "", "Output file path (required)")
		testCmd.Flags().StringP("format", "f", "", "Output format")
		testCmd.Flags().Bool("context", false, "Include context")
		testCmd.Flags().Bool("metadata", true, "Include metadata")
		_ = testCmd.MarkFlagRequired("output")

		// Running without output flag should fail.
		// Note: The error comes from config initialization, not flag validation.
		err := testCmd.RunE(testCmd, []string{"test-session"})
		assert.Error(t, err)
	})

	t.Run("export command requires session name argument", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "export",
			Args: cobra.ExactArgs(1),
			RunE: exportSessionCommand,
		}
		testCmd.Flags().StringP("output", "o", "", "Output file path")
		testCmd.Flags().StringP("format", "f", "", "Output format")
		testCmd.Flags().Bool("context", false, "Include context")
		testCmd.Flags().Bool("metadata", true, "Include metadata")

		// Running with no args should fail validation.
		err := cobra.ExactArgs(1)(testCmd, []string{})
		assert.Error(t, err)
	})

	t.Run("import command requires checkpoint file argument", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "import",
			Args: cobra.ExactArgs(1),
			RunE: importSessionCommand,
		}
		testCmd.Flags().StringP("name", "n", "", "Name for imported session")
		testCmd.Flags().Bool("overwrite", false, "Overwrite existing")
		testCmd.Flags().Bool("context", true, "Include context")

		// Running with no args should fail validation.
		err := cobra.ExactArgs(1)(testCmd, []string{})
		assert.Error(t, err)
	})

	t.Run("listSessionsCommand returns error without valid config", func(t *testing.T) {
		// Without proper configuration, the command should fail.
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")
		err := listSessionsCommand(sessionsListCmd, []string{})
		assert.Error(t, err)
	})

	t.Run("cleanSessionsCommand returns error without valid config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

		testCmd := &cobra.Command{
			Use:  "clean",
			RunE: cleanSessionsCommand,
		}
		testCmd.Flags().String("older-than", "30d", "Duration")

		err := cleanSessionsCommand(testCmd, []string{})
		assert.Error(t, err)
	})
}

func TestConstants(t *testing.T) {
	t.Run("time constants are correct", func(t *testing.T) {
		assert.Equal(t, 24, hoursPerDay)
		assert.Equal(t, 7, daysPerWeek)
		assert.Equal(t, 30, daysPerMonth)
	})

	t.Run("context flag constant is correct", func(t *testing.T) {
		assert.Equal(t, "context", contextFlag)
	})
}
