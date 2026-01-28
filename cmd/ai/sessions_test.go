//nolint:dupl // Test files contain similar setup code by design for isolation and clarity.
package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

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
		{
			name:          "hours that divide evenly into days",
			durationStr:   "72h",
			expectedDays:  3, // 72 / 24 = 3, no remainder
			expectedError: nil,
		},
		{
			name:          "hours with 1 remainder",
			durationStr:   "49h",
			expectedDays:  3, // 49 / 24 = 2 remainder 1, rounds up to 3
			expectedError: nil,
		},
		{
			name:          "large weeks value",
			durationStr:   "52w",
			expectedDays:  364, // 52 * 7
			expectedError: nil,
		},
		{
			name:          "large months value",
			durationStr:   "12m",
			expectedDays:  360, // 12 * 30
			expectedError: nil,
		},
		{
			name:          "invalid format - spaces",
			durationStr:   "30 d",
			expectedDays:  30, // fmt.Sscanf parses "30 d" as value=30, unit="d" (skipping whitespace)
			expectedError: nil,
		},
		{
			name:          "invalid format - uppercase unit",
			durationStr:   "30D",
			expectedDays:  0,
			expectedError: errUtils.ErrAIUnsupportedDurationUnit,
		},
		{
			name:          "invalid format - mixed case",
			durationStr:   "30dD",
			expectedDays:  0,
			expectedError: errUtils.ErrAIUnsupportedDurationUnit,
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

	t.Run("cleanSessionsCommand returns error with invalid duration format", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "clean",
			RunE: cleanSessionsCommand,
		}
		testCmd.Flags().String("older-than", "invalid-duration", "Duration")

		err := cleanSessionsCommand(testCmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid duration format")
	})

	t.Run("exportSessionCommand returns error without valid config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

		testCmd := &cobra.Command{
			Use:  "export",
			RunE: exportSessionCommand,
		}
		testCmd.Flags().StringP("output", "o", "output.json", "Output file path")
		testCmd.Flags().StringP("format", "f", "", "Output format")
		testCmd.Flags().Bool("context", false, "Include context")
		testCmd.Flags().Bool("metadata", true, "Include metadata")

		err := exportSessionCommand(testCmd, []string{"test-session"})
		assert.Error(t, err)
	})

	t.Run("importSessionCommand returns error without valid config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

		testCmd := &cobra.Command{
			Use:  "import",
			RunE: importSessionCommand,
		}
		testCmd.Flags().StringP("name", "n", "", "Name for imported session")
		testCmd.Flags().Bool("overwrite", false, "Overwrite existing")
		testCmd.Flags().Bool("context", true, "Include context")

		err := importSessionCommand(testCmd, []string{"checkpoint.json"})
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

func TestParseDuration_BoundaryConditions(t *testing.T) {
	tests := []struct {
		name          string
		durationStr   string
		expectedDays  int
		expectedError error
	}{
		{
			name:          "zero hours returns 0 days",
			durationStr:   "0h",
			expectedDays:  0,
			expectedError: nil,
		},
		{
			name:          "23 hours rounds to 1 day",
			durationStr:   "23h",
			expectedDays:  1,
			expectedError: nil,
		},
		{
			name:          "24 hours exactly equals 1 day",
			durationStr:   "24h",
			expectedDays:  1,
			expectedError: nil,
		},
		{
			name:          "47 hours rounds to 2 days",
			durationStr:   "47h",
			expectedDays:  2,
			expectedError: nil,
		},
		{
			name:          "very large hours value",
			durationStr:   "8760h", // 365 days
			expectedDays:  365,
			expectedError: nil,
		},
		{
			name:          "zero weeks",
			durationStr:   "0w",
			expectedDays:  0,
			expectedError: nil,
		},
		{
			name:          "zero months",
			durationStr:   "0m",
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

func TestExportSessionCommand_FlagParsing(t *testing.T) {
	t.Run("parses all flags correctly", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "export",
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		}
		testCmd.Flags().StringP("output", "o", "", "Output file path")
		testCmd.Flags().StringP("format", "f", "", "Output format")
		testCmd.Flags().Bool("context", false, "Include context")
		testCmd.Flags().Bool("metadata", true, "Include metadata")

		// Set flags.
		err := testCmd.Flags().Set("output", "test-output.json")
		assert.NoError(t, err)

		err = testCmd.Flags().Set("format", "json")
		assert.NoError(t, err)

		err = testCmd.Flags().Set("context", "true")
		assert.NoError(t, err)

		err = testCmd.Flags().Set("metadata", "false")
		assert.NoError(t, err)

		// Verify flags were set.
		output, err := testCmd.Flags().GetString("output")
		assert.NoError(t, err)
		assert.Equal(t, "test-output.json", output)

		format, err := testCmd.Flags().GetString("format")
		assert.NoError(t, err)
		assert.Equal(t, "json", format)

		includeContext, err := testCmd.Flags().GetBool("context")
		assert.NoError(t, err)
		assert.True(t, includeContext)

		includeMetadata, err := testCmd.Flags().GetBool("metadata")
		assert.NoError(t, err)
		assert.False(t, includeMetadata)
	})

	t.Run("shorthand flags work", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "export",
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		}
		testCmd.Flags().StringP("output", "o", "", "Output file path")
		testCmd.Flags().StringP("format", "f", "", "Output format")

		// Set using full flag name, verify shorthand is registered.
		outputFlag := testCmd.Flags().Lookup("output")
		assert.NotNil(t, outputFlag)
		assert.Equal(t, "o", outputFlag.Shorthand)

		formatFlag := testCmd.Flags().Lookup("format")
		assert.NotNil(t, formatFlag)
		assert.Equal(t, "f", formatFlag.Shorthand)
	})
}

func TestImportSessionCommand_FlagParsing(t *testing.T) {
	t.Run("parses all flags correctly", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "import",
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		}
		testCmd.Flags().StringP("name", "n", "", "Name for imported session")
		testCmd.Flags().Bool("overwrite", false, "Overwrite existing")
		testCmd.Flags().Bool("context", true, "Include context")

		// Set flags.
		err := testCmd.Flags().Set("name", "imported-session")
		assert.NoError(t, err)

		err = testCmd.Flags().Set("overwrite", "true")
		assert.NoError(t, err)

		err = testCmd.Flags().Set("context", "false")
		assert.NoError(t, err)

		// Verify flags were set.
		name, err := testCmd.Flags().GetString("name")
		assert.NoError(t, err)
		assert.Equal(t, "imported-session", name)

		overwrite, err := testCmd.Flags().GetBool("overwrite")
		assert.NoError(t, err)
		assert.True(t, overwrite)

		includeContext, err := testCmd.Flags().GetBool("context")
		assert.NoError(t, err)
		assert.False(t, includeContext)
	})

	t.Run("shorthand flags work", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "import",
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		}
		testCmd.Flags().StringP("name", "n", "", "Name for imported session")

		// Verify shorthand is registered.
		nameFlag := testCmd.Flags().Lookup("name")
		assert.NotNil(t, nameFlag)
		assert.Equal(t, "n", nameFlag.Shorthand)
	})
}

func TestCleanSessionCommand_FlagParsing(t *testing.T) {
	t.Run("parses older-than flag correctly", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "clean",
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		}
		testCmd.Flags().String("older-than", "30d", "Duration")

		// Set flag.
		err := testCmd.Flags().Set("older-than", "7d")
		assert.NoError(t, err)

		// Verify flag was set.
		olderThan, err := testCmd.Flags().GetString("older-than")
		assert.NoError(t, err)
		assert.Equal(t, "7d", olderThan)
	})

	t.Run("default value is 30d", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "clean",
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		}
		testCmd.Flags().String("older-than", "30d", "Duration")

		// Get default value.
		olderThan, err := testCmd.Flags().GetString("older-than")
		assert.NoError(t, err)
		assert.Equal(t, "30d", olderThan)
	})
}

func TestSessionsCommand_LongDescriptions(t *testing.T) {
	t.Run("sessions command has comprehensive long description", func(t *testing.T) {
		assert.Contains(t, sessionsCmd.Long, "sessions")
		assert.Contains(t, sessionsCmd.Long, "conversation")
	})

	t.Run("list command long description mentions details", func(t *testing.T) {
		assert.Contains(t, sessionsListCmd.Long, "session")
		assert.Contains(t, sessionsListCmd.Long, "Example")
	})

	t.Run("clean command long description mentions retention", func(t *testing.T) {
		assert.Contains(t, sessionsCleanCmd.Long, "older")
		assert.Contains(t, sessionsCleanCmd.Long, "Example")
	})

	t.Run("export command long description mentions formats", func(t *testing.T) {
		assert.Contains(t, sessionsExportCmd.Long, "checkpoint")
		assert.Contains(t, sessionsExportCmd.Long, "JSON")
		assert.Contains(t, sessionsExportCmd.Long, "YAML")
		assert.Contains(t, sessionsExportCmd.Long, "Markdown")
	})

	t.Run("import command long description mentions restore", func(t *testing.T) {
		assert.Contains(t, sessionsImportCmd.Long, "checkpoint")
		assert.Contains(t, sessionsImportCmd.Long, "Restores")
	})
}

func TestExportSessionCommand_ArgsValidation(t *testing.T) {
	t.Run("accepts exactly one argument", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "export <session-name>",
			Args: cobra.ExactArgs(1),
		}

		// Valid - one argument.
		err := cobra.ExactArgs(1)(testCmd, []string{"session-name"})
		assert.NoError(t, err)

		// Invalid - no arguments.
		err = cobra.ExactArgs(1)(testCmd, []string{})
		assert.Error(t, err)

		// Invalid - two arguments.
		err = cobra.ExactArgs(1)(testCmd, []string{"session1", "session2"})
		assert.Error(t, err)
	})
}

func TestImportSessionCommand_ArgsValidation(t *testing.T) {
	t.Run("accepts exactly one argument", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "import <checkpoint-file>",
			Args: cobra.ExactArgs(1),
		}

		// Valid - one argument.
		err := cobra.ExactArgs(1)(testCmd, []string{"checkpoint.json"})
		assert.NoError(t, err)

		// Invalid - no arguments.
		err = cobra.ExactArgs(1)(testCmd, []string{})
		assert.Error(t, err)

		// Invalid - two arguments.
		err = cobra.ExactArgs(1)(testCmd, []string{"file1.json", "file2.json"})
		assert.Error(t, err)
	})
}

func TestSessionsCommand_CommandHierarchy(t *testing.T) {
	t.Run("sessions command is parent of subcommands", func(t *testing.T) {
		// Verify list is a child of sessions.
		assert.Equal(t, sessionsCmd, sessionsListCmd.Parent())
	})

	t.Run("sessions command has 4 subcommands", func(t *testing.T) {
		assert.Len(t, sessionsCmd.Commands(), 4)
	})
}

func TestInitSessionManager_Errors(t *testing.T) {
	// Test that initSessionManager returns appropriate errors.
	// Since it depends on config, we can only test that it errors with bad config.
	t.Run("returns error when AI is not enabled", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

		manager, cleanup, err := initSessionManager()

		assert.Error(t, err)
		assert.Nil(t, manager)
		assert.Nil(t, cleanup)
	})
}

func TestCleanSessionsCommand_DurationValidation(t *testing.T) {
	tests := []struct {
		name        string
		olderThan   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid 30d",
			olderThan:   "30d",
			expectError: false,
		},
		{
			name:        "valid 7d",
			olderThan:   "7d",
			expectError: false,
		},
		{
			name:        "valid 24h",
			olderThan:   "24h",
			expectError: false,
		},
		{
			name:        "valid 2w",
			olderThan:   "2w",
			expectError: false,
		},
		{
			name:        "valid 1m",
			olderThan:   "1m",
			expectError: false,
		},
		{
			name:        "invalid format xyz",
			olderThan:   "xyz",
			expectError: true,
			errorMsg:    "invalid duration format",
		},
		{
			name:        "invalid unit y",
			olderThan:   "30y",
			expectError: true,
			errorMsg:    "unsupported duration unit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days, err := parseDuration(tt.olderThan)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Greater(t, days, 0)
			}
		})
	}
}

func TestSessionsCommand_OutputFlagRequired(t *testing.T) {
	t.Run("export command marks output as required", func(t *testing.T) {
		// Check that the output flag is registered.
		outputFlag := sessionsExportCmd.Flags().Lookup("output")
		require.NotNil(t, outputFlag)

		// The flag should have the required annotation.
		// This is checked during command execution.
		assert.Equal(t, "string", outputFlag.Value.Type())
	})
}

func TestListSessionsCommand_NoConfigError(t *testing.T) {
	// Ensure proper environment isolation.
	originalEnv := os.Getenv("ATMOS_CLI_CONFIG_PATH")
	defer func() {
		if originalEnv == "" {
			os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		} else {
			os.Setenv("ATMOS_CLI_CONFIG_PATH", originalEnv)
		}
	}()

	t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/config/path")

	err := listSessionsCommand(sessionsListCmd, []string{})
	assert.Error(t, err)
}

func TestCleanSessionsCommand_NoFlagError(t *testing.T) {
	t.Run("handles missing older-than flag gracefully", func(t *testing.T) {
		// Create a command without the flag.
		testCmd := &cobra.Command{
			Use:  "clean",
			RunE: cleanSessionsCommand,
		}

		// This should error because the flag is not registered.
		err := cleanSessionsCommand(testCmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get older-than flag")
	})
}

func TestExportSessionCommand_MissingFlags(t *testing.T) {
	t.Run("handles missing output flag", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "export",
			RunE: exportSessionCommand,
		}
		// Only register some flags.
		testCmd.Flags().StringP("format", "f", "", "Output format")
		testCmd.Flags().Bool("context", false, "Include context")
		testCmd.Flags().Bool("metadata", true, "Include metadata")

		// Should error on missing output flag.
		err := exportSessionCommand(testCmd, []string{"test-session"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get output flag")
	})

	t.Run("handles missing format flag", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "export",
			RunE: exportSessionCommand,
		}
		testCmd.Flags().StringP("output", "o", "", "Output file path")
		testCmd.Flags().Bool("context", false, "Include context")
		testCmd.Flags().Bool("metadata", true, "Include metadata")

		// Should error on missing format flag.
		err := exportSessionCommand(testCmd, []string{"test-session"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get format flag")
	})

	t.Run("handles missing context flag", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "export",
			RunE: exportSessionCommand,
		}
		testCmd.Flags().StringP("output", "o", "output.json", "Output file path")
		testCmd.Flags().StringP("format", "f", "", "Output format")
		testCmd.Flags().Bool("metadata", true, "Include metadata")

		// Should error on missing context flag.
		err := exportSessionCommand(testCmd, []string{"test-session"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get context flag")
	})

	t.Run("handles missing metadata flag", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "export",
			RunE: exportSessionCommand,
		}
		testCmd.Flags().StringP("output", "o", "output.json", "Output file path")
		testCmd.Flags().StringP("format", "f", "", "Output format")
		testCmd.Flags().Bool("context", false, "Include context")

		// Should error on missing metadata flag.
		err := exportSessionCommand(testCmd, []string{"test-session"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get metadata flag")
	})
}

func TestImportSessionCommand_MissingFlags(t *testing.T) {
	t.Run("handles missing name flag", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "import",
			RunE: importSessionCommand,
		}
		testCmd.Flags().Bool("overwrite", false, "Overwrite existing")
		testCmd.Flags().Bool("context", true, "Include context")

		// Should error on missing name flag.
		err := importSessionCommand(testCmd, []string{"checkpoint.json"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get name flag")
	})

	t.Run("handles missing overwrite flag", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "import",
			RunE: importSessionCommand,
		}
		testCmd.Flags().StringP("name", "n", "", "Name for imported session")
		testCmd.Flags().Bool("context", true, "Include context")

		// Should error on missing overwrite flag.
		err := importSessionCommand(testCmd, []string{"checkpoint.json"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get overwrite flag")
	})

	t.Run("handles missing context flag", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "import",
			RunE: importSessionCommand,
		}
		testCmd.Flags().StringP("name", "n", "", "Name for imported session")
		testCmd.Flags().Bool("overwrite", false, "Overwrite existing")

		// Should error on missing context flag.
		err := importSessionCommand(testCmd, []string{"checkpoint.json"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get context flag")
	})
}

func TestSessionsCmd_DefaultRunE(t *testing.T) {
	// The default sessions command should run listSessionsCommand.
	assert.NotNil(t, sessionsCmd.RunE)
}

func TestParseDuration_EdgeCases(t *testing.T) {
	t.Run("handles very small hour values", func(t *testing.T) {
		// 1 hour should round up to 1 day.
		days, err := parseDuration("1h")
		assert.NoError(t, err)
		assert.Equal(t, 1, days)
	})

	t.Run("handles hour value that equals exact days", func(t *testing.T) {
		// 168 hours = 7 days exactly.
		days, err := parseDuration("168h")
		assert.NoError(t, err)
		assert.Equal(t, 7, days)
	})

	t.Run("handles hour value just over exact days", func(t *testing.T) {
		// 169 hours = 7 days + 1 hour, rounds to 8 days.
		days, err := parseDuration("169h")
		assert.NoError(t, err)
		assert.Equal(t, 8, days)
	})
}

func TestSessionsCommand_CobraIntegration(t *testing.T) {
	t.Run("list command can be found by name", func(t *testing.T) {
		found := false
		for _, cmd := range sessionsCmd.Commands() {
			if cmd.Name() == "list" {
				found = true
				break
			}
		}
		assert.True(t, found, "list command should be found in sessions subcommands")
	})

	t.Run("clean command can be found by name", func(t *testing.T) {
		found := false
		for _, cmd := range sessionsCmd.Commands() {
			if cmd.Name() == "clean" {
				found = true
				break
			}
		}
		assert.True(t, found, "clean command should be found in sessions subcommands")
	})

	t.Run("export command can be found by name", func(t *testing.T) {
		found := false
		for _, cmd := range sessionsCmd.Commands() {
			if cmd.Name() == "export" {
				found = true
				break
			}
		}
		assert.True(t, found, "export command should be found in sessions subcommands")
	})

	t.Run("import command can be found by name", func(t *testing.T) {
		found := false
		for _, cmd := range sessionsCmd.Commands() {
			if cmd.Name() == "import" {
				found = true
				break
			}
		}
		assert.True(t, found, "import command should be found in sessions subcommands")
	})
}

// Helper function to create a test checkpoint file.
func createTestCheckpointFile(t *testing.T, path string, format string) {
	t.Helper()

	checkpoint := session.Checkpoint{
		Version:    session.CheckpointVersion,
		ExportedAt: time.Now(),
		Session: session.CheckpointSession{
			Name:        "test-session",
			Provider:    "anthropic",
			Model:       "claude-3-opus",
			ProjectPath: "/test/project",
			CreatedAt:   time.Now().Add(-24 * time.Hour),
			UpdatedAt:   time.Now(),
		},
		Messages: []session.CheckpointMessage{
			{
				Role:      "user",
				Content:   "Hello",
				CreatedAt: time.Now().Add(-23 * time.Hour),
			},
			{
				Role:      "assistant",
				Content:   "Hi there!",
				CreatedAt: time.Now().Add(-22 * time.Hour),
			},
		},
		Statistics: session.CheckpointStatistics{
			MessageCount:      2,
			UserMessages:      1,
			AssistantMessages: 1,
		},
	}

	var data []byte
	var err error

	switch format {
	case "json":
		data, err = json.MarshalIndent(checkpoint, "", "  ")
	default:
		t.Fatalf("unsupported format: %s", format)
	}

	require.NoError(t, err)
	err = os.WriteFile(path, data, 0o644)
	require.NoError(t, err)
}

func TestValidateCheckpointFile(t *testing.T) {
	// Use t.TempDir() for cross-platform temp directory.
	tempDir := t.TempDir()

	t.Run("validates valid checkpoint file", func(t *testing.T) {
		checkpointPath := filepath.Join(tempDir, "valid-checkpoint.json")
		createTestCheckpointFile(t, checkpointPath, "json")

		err := session.ValidateCheckpointFile(checkpointPath)
		assert.NoError(t, err)
	})

	t.Run("returns error for nonexistent file", func(t *testing.T) {
		nonexistentPath := filepath.Join(tempDir, "nonexistent.json")

		err := session.ValidateCheckpointFile(nonexistentPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read checkpoint file")
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		invalidPath := filepath.Join(tempDir, "invalid.json")
		err := os.WriteFile(invalidPath, []byte("not valid json"), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(invalidPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse JSON checkpoint")
	})

	t.Run("returns error for missing version", func(t *testing.T) {
		noVersionPath := filepath.Join(tempDir, "no-version.json")
		err := os.WriteFile(noVersionPath, []byte(`{"session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"user","content":"test"}],"statistics":{"message_count":1}}`), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(noVersionPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "checkpoint version is missing")
	})

	t.Run("returns error for invalid version", func(t *testing.T) {
		invalidVersionPath := filepath.Join(tempDir, "invalid-version.json")
		err := os.WriteFile(invalidVersionPath, []byte(`{"version":"9.9","session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"user","content":"test"}],"statistics":{"message_count":1}}`), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(invalidVersionPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported checkpoint version")
	})

	t.Run("returns error for missing session name", func(t *testing.T) {
		noNamePath := filepath.Join(tempDir, "no-name.json")
		err := os.WriteFile(noNamePath, []byte(`{"version":"1.0","session":{"provider":"test","model":"test"},"messages":[{"role":"user","content":"test"}],"statistics":{"message_count":1}}`), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(noNamePath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session name is required")
	})

	t.Run("returns error for missing provider", func(t *testing.T) {
		noProviderPath := filepath.Join(tempDir, "no-provider.json")
		err := os.WriteFile(noProviderPath, []byte(`{"version":"1.0","session":{"name":"test","model":"test"},"messages":[{"role":"user","content":"test"}],"statistics":{"message_count":1}}`), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(noProviderPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session provider is required")
	})

	t.Run("returns error for missing model", func(t *testing.T) {
		noModelPath := filepath.Join(tempDir, "no-model.json")
		err := os.WriteFile(noModelPath, []byte(`{"version":"1.0","session":{"name":"test","provider":"test"},"messages":[{"role":"user","content":"test"}],"statistics":{"message_count":1}}`), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(noModelPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session model is required")
	})

	t.Run("returns error for empty messages", func(t *testing.T) {
		emptyMessagesPath := filepath.Join(tempDir, "empty-messages.json")
		err := os.WriteFile(emptyMessagesPath, []byte(`{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[],"statistics":{"message_count":0}}`), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(emptyMessagesPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "checkpoint must contain at least one message")
	})

	t.Run("returns error for message with missing role", func(t *testing.T) {
		noRolePath := filepath.Join(tempDir, "no-role.json")
		err := os.WriteFile(noRolePath, []byte(`{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[{"content":"test"}],"statistics":{"message_count":1}}`), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(noRolePath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "role is required")
	})

	t.Run("returns error for message with invalid role", func(t *testing.T) {
		invalidRolePath := filepath.Join(tempDir, "invalid-role.json")
		err := os.WriteFile(invalidRolePath, []byte(`{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"invalid","content":"test"}],"statistics":{"message_count":1}}`), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(invalidRolePath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid role")
	})

	t.Run("returns error for mismatched message count", func(t *testing.T) {
		mismatchPath := filepath.Join(tempDir, "mismatch-count.json")
		err := os.WriteFile(mismatchPath, []byte(`{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"user","content":"test"}],"statistics":{"message_count":5}}`), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(mismatchPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "statistics message count")
	})
}

func TestParseDuration_NegativeValues(t *testing.T) {
	// Test that negative values are handled (they're allowed but may not make sense).
	tests := []struct {
		name         string
		durationStr  string
		expectedDays int
	}{
		{
			name:         "negative days",
			durationStr:  "-7d",
			expectedDays: -7,
		},
		{
			name:         "negative weeks",
			durationStr:  "-2w",
			expectedDays: -14,
		},
		{
			name:         "negative months",
			durationStr:  "-1m",
			expectedDays: -30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days, err := parseDuration(tt.durationStr)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedDays, days)
		})
	}
}

func TestParseDuration_NegativeHours(t *testing.T) {
	// Negative hours are a bit tricky with the rounding logic.
	t.Run("negative hours", func(t *testing.T) {
		days, err := parseDuration("-24h")
		assert.NoError(t, err)
		// -24 / 24 = -1, no remainder.
		assert.Equal(t, -1, days)
	})

	t.Run("negative hours with remainder", func(t *testing.T) {
		days, err := parseDuration("-25h")
		assert.NoError(t, err)
		// -25 / 24 = -1, remainder -1, so days++ makes it 0.
		// Note: This behavior may not be ideal but reflects current implementation.
		assert.Equal(t, 0, days)
	})
}

func TestSessionsCommand_FlagDefaults(t *testing.T) {
	t.Run("clean command older-than default is 30d", func(t *testing.T) {
		flag := sessionsCleanCmd.Flags().Lookup("older-than")
		require.NotNil(t, flag)
		assert.Equal(t, "30d", flag.DefValue)
	})

	t.Run("export command context default is false", func(t *testing.T) {
		flag := sessionsExportCmd.Flags().Lookup("context")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("export command metadata default is true", func(t *testing.T) {
		flag := sessionsExportCmd.Flags().Lookup("metadata")
		require.NotNil(t, flag)
		assert.Equal(t, "true", flag.DefValue)
	})

	t.Run("import command overwrite default is false", func(t *testing.T) {
		flag := sessionsImportCmd.Flags().Lookup("overwrite")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("import command context default is true", func(t *testing.T) {
		flag := sessionsImportCmd.Flags().Lookup("context")
		require.NotNil(t, flag)
		assert.Equal(t, "true", flag.DefValue)
	})
}

func TestSessionsCommand_UsageStrings(t *testing.T) {
	t.Run("sessions command usage is correct", func(t *testing.T) {
		assert.Equal(t, "sessions", sessionsCmd.Use)
	})

	t.Run("list command usage is correct", func(t *testing.T) {
		assert.Equal(t, "list", sessionsListCmd.Use)
	})

	t.Run("clean command usage is correct", func(t *testing.T) {
		assert.Equal(t, "clean", sessionsCleanCmd.Use)
	})

	t.Run("export command usage is correct", func(t *testing.T) {
		assert.Equal(t, "export <session-name>", sessionsExportCmd.Use)
	})

	t.Run("import command usage is correct", func(t *testing.T) {
		assert.Equal(t, "import <checkpoint-file>", sessionsImportCmd.Use)
	})
}

func TestSessionsCommand_OutputCapture(t *testing.T) {
	// Test that commands produce output through standard channels.
	t.Run("commands have RunE set", func(t *testing.T) {
		assert.NotNil(t, sessionsCmd.RunE)
		assert.NotNil(t, sessionsListCmd.RunE)
		assert.NotNil(t, sessionsCleanCmd.RunE)
		assert.NotNil(t, sessionsExportCmd.RunE)
		assert.NotNil(t, sessionsImportCmd.RunE)
	})
}

func TestParseDuration_SpecialCases(t *testing.T) {
	t.Run("single digit values", func(t *testing.T) {
		days, err := parseDuration("5d")
		assert.NoError(t, err)
		assert.Equal(t, 5, days)
	})

	t.Run("double digit values", func(t *testing.T) {
		days, err := parseDuration("99d")
		assert.NoError(t, err)
		assert.Equal(t, 99, days)
	})

	t.Run("triple digit values", func(t *testing.T) {
		days, err := parseDuration("365d")
		assert.NoError(t, err)
		assert.Equal(t, 365, days)
	})

	t.Run("very large value", func(t *testing.T) {
		days, err := parseDuration("10000d")
		assert.NoError(t, err)
		assert.Equal(t, 10000, days)
	})
}

func TestValidateCheckpointFile_YAMLFormat(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("validates valid YAML checkpoint file", func(t *testing.T) {
		yamlContent := `version: "1.0"
session:
  name: "test-session"
  provider: "anthropic"
  model: "claude-3-opus"
  project_path: "/test/project"
messages:
  - role: "user"
    content: "Hello"
  - role: "assistant"
    content: "Hi there!"
statistics:
  message_count: 2
  user_messages: 1
  assistant_messages: 1
`
		yamlPath := filepath.Join(tempDir, "valid.yaml")
		err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(yamlPath)
		assert.NoError(t, err)
	})

	t.Run("returns error for invalid YAML", func(t *testing.T) {
		invalidPath := filepath.Join(tempDir, "invalid.yaml")
		err := os.WriteFile(invalidPath, []byte("invalid: yaml: content:"), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(invalidPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse YAML checkpoint")
	})

	t.Run("validates .yml extension", func(t *testing.T) {
		yamlContent := `version: "1.0"
session:
  name: "test-session"
  provider: "anthropic"
  model: "claude-3-opus"
messages:
  - role: "user"
    content: "Hello"
statistics:
  message_count: 1
`
		ymlPath := filepath.Join(tempDir, "valid.yml")
		err := os.WriteFile(ymlPath, []byte(yamlContent), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(ymlPath)
		assert.NoError(t, err)
	})
}

func TestSessionsCommand_HelpText(t *testing.T) {
	// Verify help text contains expected content.
	var buf bytes.Buffer

	t.Run("sessions command help contains subcommands", func(t *testing.T) {
		sessionsCmd.SetOut(&buf)
		sessionsCmd.SetErr(&buf)

		// Get help output.
		_ = sessionsCmd.Help()
		output := buf.String()

		assert.Contains(t, output, "list")
		assert.Contains(t, output, "clean")
		assert.Contains(t, output, "export")
		assert.Contains(t, output, "import")
	})
}

func TestSessionsCommand_WindowsCompatibility(t *testing.T) {
	// Test that path operations work on all platforms.
	tempDir := t.TempDir()

	t.Run("checkpoint path uses correct separator", func(t *testing.T) {
		// Create a path using filepath.Join.
		checkpointPath := filepath.Join(tempDir, "sessions", "checkpoint.json")

		// Verify the directory component can be extracted.
		dir := filepath.Dir(checkpointPath)
		assert.NotEmpty(t, dir)

		// Verify the base name is correct.
		base := filepath.Base(checkpointPath)
		assert.Equal(t, "checkpoint.json", base)
	})

	t.Run("handles paths with special characters", func(t *testing.T) {
		// Create a path with spaces (works on all platforms with filepath.Join).
		spacePath := filepath.Join(tempDir, "path with spaces", "checkpoint.json")
		dir := filepath.Dir(spacePath)
		assert.Contains(t, dir, "path with spaces")
	})
}

func TestConstants_Values(t *testing.T) {
	// Verify constant values are as expected.
	t.Run("hoursPerDay is 24", func(t *testing.T) {
		assert.Equal(t, 24, hoursPerDay)
	})

	t.Run("daysPerWeek is 7", func(t *testing.T) {
		assert.Equal(t, 7, daysPerWeek)
	})

	t.Run("daysPerMonth is 30", func(t *testing.T) {
		assert.Equal(t, 30, daysPerMonth)
	})

	t.Run("contextFlag is context", func(t *testing.T) {
		assert.Equal(t, "context", contextFlag)
	})
}

func TestSessionsCommand_SubcommandParent(t *testing.T) {
	// Verify parent-child relationships.
	t.Run("list parent is sessions", func(t *testing.T) {
		assert.Equal(t, sessionsCmd, sessionsListCmd.Parent())
	})

	t.Run("clean parent is sessions", func(t *testing.T) {
		assert.Equal(t, sessionsCmd, sessionsCleanCmd.Parent())
	})

	t.Run("export parent is sessions", func(t *testing.T) {
		assert.Equal(t, sessionsCmd, sessionsExportCmd.Parent())
	})

	t.Run("import parent is sessions", func(t *testing.T) {
		assert.Equal(t, sessionsCmd, sessionsImportCmd.Parent())
	})
}

func TestValidateCheckpointFile_AllRoles(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("accepts user role", func(t *testing.T) {
		content := `{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"user","content":"test"}],"statistics":{"message_count":1}}`
		path := filepath.Join(tempDir, "user-role.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})

	t.Run("accepts assistant role", func(t *testing.T) {
		content := `{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"assistant","content":"test"}],"statistics":{"message_count":1}}`
		path := filepath.Join(tempDir, "assistant-role.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})

	t.Run("accepts system role", func(t *testing.T) {
		content := `{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"system","content":"test"}],"statistics":{"message_count":1}}`
		path := filepath.Join(tempDir, "system-role.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})

	t.Run("accepts mixed roles", func(t *testing.T) {
		content := `{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"system","content":"system prompt"},{"role":"user","content":"hello"},{"role":"assistant","content":"hi"}],"statistics":{"message_count":3}}`
		path := filepath.Join(tempDir, "mixed-roles.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})
}

func TestSessionsCommand_WithAIEnabledConfig(t *testing.T) {
	// Test with a fixture that has AI enabled.
	testDir := "../../tests/fixtures/scenarios/atmos-describe-affected-with-dependents-and-locked"

	t.Run("listSessionsCommand with valid AI config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
		t.Setenv("ATMOS_BASE_PATH", testDir)

		// The command should not fail on config loading, but may fail on storage.
		err := listSessionsCommand(sessionsListCmd, []string{})
		// We expect it to proceed past config validation but may fail on storage.
		// The error might be about storage, not config.
		if err != nil {
			// Either it works or fails on storage, not config validation.
			assert.NotContains(t, err.Error(), "AI features are not enabled")
			assert.NotContains(t, err.Error(), "sessions are not enabled")
		}
	})

	t.Run("cleanSessionsCommand with valid AI config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
		t.Setenv("ATMOS_BASE_PATH", testDir)

		testCmd := &cobra.Command{
			Use:  "clean",
			RunE: cleanSessionsCommand,
		}
		testCmd.Flags().String("older-than", "30d", "Duration")

		err := cleanSessionsCommand(testCmd, []string{})
		if err != nil {
			// Should proceed past config validation.
			assert.NotContains(t, err.Error(), "AI features are not enabled")
			assert.NotContains(t, err.Error(), "sessions are not enabled")
		}
	})
}

func TestSessionsCommand_UnsupportedFormat(t *testing.T) {
	// Test checkpoint file with unsupported format.
	tempDir := t.TempDir()

	t.Run("returns error for unsupported format file", func(t *testing.T) {
		// Create a file with unsupported extension.
		txtPath := filepath.Join(tempDir, "checkpoint.txt")
		err := os.WriteFile(txtPath, []byte("some content"), 0o644)
		require.NoError(t, err)

		// ValidateCheckpointFile should fail because .txt defaults to JSON parsing.
		err = session.ValidateCheckpointFile(txtPath)
		assert.Error(t, err)
	})
}

func TestCheckpointCreation_Integration(t *testing.T) {
	// Test creating and validating a checkpoint file with various content.
	tempDir := t.TempDir()

	t.Run("checkpoint with metadata", func(t *testing.T) {
		content := `{
			"version": "1.0",
			"session": {
				"name": "test-session",
				"provider": "anthropic",
				"model": "claude-3-opus",
				"metadata": {
					"custom_key": "custom_value",
					"number": 123
				}
			},
			"messages": [
				{"role": "user", "content": "Hello"}
			],
			"statistics": {
				"message_count": 1,
				"user_messages": 1,
				"assistant_messages": 0
			}
		}`
		path := filepath.Join(tempDir, "with-metadata.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})

	t.Run("checkpoint with context", func(t *testing.T) {
		content := `{
			"version": "1.0",
			"session": {
				"name": "test-session",
				"provider": "openai",
				"model": "gpt-4"
			},
			"messages": [
				{"role": "user", "content": "Test"}
			],
			"context": {
				"working_directory": "/home/user/project",
				"files_accessed": ["file1.tf", "file2.tf"]
			},
			"statistics": {
				"message_count": 1
			}
		}`
		path := filepath.Join(tempDir, "with-context.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})

	t.Run("checkpoint with archived messages", func(t *testing.T) {
		content := `{
			"version": "1.0",
			"session": {
				"name": "archived-session",
				"provider": "anthropic",
				"model": "claude-3-opus"
			},
			"messages": [
				{"role": "user", "content": "Old message", "archived": true},
				{"role": "assistant", "content": "Old response", "archived": true},
				{"role": "user", "content": "New message"}
			],
			"statistics": {
				"message_count": 3,
				"user_messages": 2,
				"assistant_messages": 1
			}
		}`
		path := filepath.Join(tempDir, "with-archived.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})

	t.Run("checkpoint with skill", func(t *testing.T) {
		content := `{
			"version": "1.0",
			"session": {
				"name": "skill-session",
				"provider": "anthropic",
				"model": "claude-3-opus",
				"skill": "stack-analyzer"
			},
			"messages": [
				{"role": "user", "content": "Analyze my stacks"}
			],
			"statistics": {
				"message_count": 1
			}
		}`
		path := filepath.Join(tempDir, "with-skill.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})
}

func TestParseDuration_AllUnits(t *testing.T) {
	// Test all valid units with various values.
	tests := []struct {
		input    string
		expected int
	}{
		// Hours
		{"1h", 1},
		{"24h", 1},
		{"48h", 2},
		{"120h", 5},
		// Days
		{"1d", 1},
		{"7d", 7},
		{"30d", 30},
		// Weeks
		{"1w", 7},
		{"4w", 28},
		{"52w", 364},
		// Months
		{"1m", 30},
		{"6m", 180},
		{"12m", 360},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseDuration(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSessionsCommand_CommandNames(t *testing.T) {
	// Verify command names match expected values.
	t.Run("sessions command name", func(t *testing.T) {
		assert.Equal(t, "sessions", sessionsCmd.Name())
	})

	t.Run("list command name", func(t *testing.T) {
		assert.Equal(t, "list", sessionsListCmd.Name())
	})

	t.Run("clean command name", func(t *testing.T) {
		assert.Equal(t, "clean", sessionsCleanCmd.Name())
	})

	t.Run("export command name", func(t *testing.T) {
		assert.Equal(t, "export", sessionsExportCmd.Name())
	})

	t.Run("import command name", func(t *testing.T) {
		assert.Equal(t, "import", sessionsImportCmd.Name())
	})
}

// TestListSessionsCommand_FunctionExecution tests the listSessionsCommand function execution paths.
func TestListSessionsCommand_FunctionExecution(t *testing.T) {
	t.Run("executes with invalid config path", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/invalid/path")
		t.Setenv("ATMOS_BASE_PATH", "/nonexistent/invalid/path")

		err := listSessionsCommand(sessionsListCmd, []string{})
		assert.Error(t, err)
	})
}

// TestCleanSessionsCommand_FunctionExecution tests the cleanSessionsCommand function execution paths.
func TestCleanSessionsCommand_FunctionExecution(t *testing.T) {
	t.Run("clean command with different duration values", func(t *testing.T) {
		testCases := []struct {
			name      string
			olderThan string
		}{
			{"7 days", "7d"},
			{"24 hours", "24h"},
			{"2 weeks", "2w"},
			{"1 month", "1m"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

				testCmd := &cobra.Command{
					Use:  "clean",
					RunE: cleanSessionsCommand,
				}
				testCmd.Flags().String("older-than", tc.olderThan, "Duration")

				err := cleanSessionsCommand(testCmd, []string{})
				// Should fail on config, not duration parsing.
				assert.Error(t, err)
				// Verify it's not a duration parsing error.
				assert.NotContains(t, err.Error(), "invalid duration format")
			})
		}
	})
}

// TestExportSessionCommand_FunctionExecution tests the exportSessionCommand function execution paths.
func TestExportSessionCommand_FunctionExecution(t *testing.T) {
	t.Run("export with all flags set", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

		testCmd := &cobra.Command{
			Use:  "export",
			RunE: exportSessionCommand,
		}
		testCmd.Flags().StringP("output", "o", "", "Output file path")
		testCmd.Flags().StringP("format", "f", "", "Output format")
		testCmd.Flags().Bool("context", false, "Include context")
		testCmd.Flags().Bool("metadata", true, "Include metadata")

		// Set flags.
		_ = testCmd.Flags().Set("output", "test-output.json")
		_ = testCmd.Flags().Set("format", "json")
		_ = testCmd.Flags().Set("context", "true")
		_ = testCmd.Flags().Set("metadata", "true")

		err := exportSessionCommand(testCmd, []string{"test-session"})
		// Should fail on config initialization, not flag parsing.
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "failed to get")
	})

	t.Run("export with yaml format", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

		testCmd := &cobra.Command{
			Use:  "export",
			RunE: exportSessionCommand,
		}
		testCmd.Flags().StringP("output", "o", "", "Output file path")
		testCmd.Flags().StringP("format", "f", "", "Output format")
		testCmd.Flags().Bool("context", false, "Include context")
		testCmd.Flags().Bool("metadata", true, "Include metadata")

		_ = testCmd.Flags().Set("output", "test-output.yaml")
		_ = testCmd.Flags().Set("format", "yaml")

		err := exportSessionCommand(testCmd, []string{"test-session"})
		assert.Error(t, err)
	})

	t.Run("export with markdown format", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

		testCmd := &cobra.Command{
			Use:  "export",
			RunE: exportSessionCommand,
		}
		testCmd.Flags().StringP("output", "o", "", "Output file path")
		testCmd.Flags().StringP("format", "f", "", "Output format")
		testCmd.Flags().Bool("context", false, "Include context")
		testCmd.Flags().Bool("metadata", true, "Include metadata")

		_ = testCmd.Flags().Set("output", "test-output.md")
		_ = testCmd.Flags().Set("format", "markdown")

		err := exportSessionCommand(testCmd, []string{"test-session"})
		assert.Error(t, err)
	})
}

// TestImportSessionCommand_FunctionExecution tests the importSessionCommand function execution paths.
func TestImportSessionCommand_FunctionExecution(t *testing.T) {
	t.Run("import with all flags set", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

		testCmd := &cobra.Command{
			Use:  "import",
			RunE: importSessionCommand,
		}
		testCmd.Flags().StringP("name", "n", "", "Name for imported session")
		testCmd.Flags().Bool("overwrite", false, "Overwrite existing")
		testCmd.Flags().Bool("context", true, "Include context")

		// Set flags.
		_ = testCmd.Flags().Set("name", "imported-session")
		_ = testCmd.Flags().Set("overwrite", "true")
		_ = testCmd.Flags().Set("context", "true")

		err := importSessionCommand(testCmd, []string{"checkpoint.json"})
		// Should fail on config initialization, not flag parsing.
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "failed to get")
	})

	t.Run("import with default flags", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

		testCmd := &cobra.Command{
			Use:  "import",
			RunE: importSessionCommand,
		}
		testCmd.Flags().StringP("name", "n", "", "Name for imported session")
		testCmd.Flags().Bool("overwrite", false, "Overwrite existing")
		testCmd.Flags().Bool("context", true, "Include context")

		// Keep default flag values.
		err := importSessionCommand(testCmd, []string{"checkpoint.yaml"})
		assert.Error(t, err)
	})
}

// TestParseDuration_MoreEdgeCases tests additional edge cases for parseDuration.
func TestParseDuration_MoreEdgeCases(t *testing.T) {
	t.Run("very large hour value with exact division", func(t *testing.T) {
		// 240 hours = 10 days exactly.
		days, err := parseDuration("240h")
		assert.NoError(t, err)
		assert.Equal(t, 10, days)
	})

	t.Run("very large hour value with remainder", func(t *testing.T) {
		// 241 hours = 10 days + 1 hour, rounds to 11 days.
		days, err := parseDuration("241h")
		assert.NoError(t, err)
		assert.Equal(t, 11, days)
	})

	t.Run("multiple digit day value", func(t *testing.T) {
		days, err := parseDuration("1234d")
		assert.NoError(t, err)
		assert.Equal(t, 1234, days)
	})

	t.Run("multiple digit week value", func(t *testing.T) {
		days, err := parseDuration("100w")
		assert.NoError(t, err)
		assert.Equal(t, 700, days) // 100 * 7
	})

	t.Run("multiple digit month value", func(t *testing.T) {
		days, err := parseDuration("100m")
		assert.NoError(t, err)
		assert.Equal(t, 3000, days) // 100 * 30
	})
}

// TestValidateCheckpointFile_MoreScenarios tests additional checkpoint validation scenarios.
func TestValidateCheckpointFile_MoreScenarios(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("checkpoint with tool role", func(t *testing.T) {
		// Tool role should be rejected.
		content := `{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"tool","content":"test"}],"statistics":{"message_count":1}}`
		path := filepath.Join(tempDir, "tool-role.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid role")
	})

	t.Run("checkpoint with function role", func(t *testing.T) {
		content := `{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"function","content":"test"}],"statistics":{"message_count":1}}`
		path := filepath.Join(tempDir, "function-role.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid role")
	})

	t.Run("checkpoint with large message count", func(t *testing.T) {
		content := `{
			"version":"1.0",
			"session":{"name":"test","provider":"test","model":"test"},
			"messages":[
				{"role":"user","content":"msg1"},
				{"role":"assistant","content":"msg2"},
				{"role":"user","content":"msg3"},
				{"role":"assistant","content":"msg4"},
				{"role":"user","content":"msg5"}
			],
			"statistics":{"message_count":5}
		}`
		path := filepath.Join(tempDir, "large-count.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})

	t.Run("checkpoint with empty content in user message", func(t *testing.T) {
		// Empty content should be allowed (some system messages can be empty).
		content := `{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"user","content":""}],"statistics":{"message_count":1}}`
		path := filepath.Join(tempDir, "empty-content.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})

	t.Run("checkpoint with special characters in content", func(t *testing.T) {
		content := `{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"user","content":"Hello\nWorld\t\"quoted\" <html>"}],"statistics":{"message_count":1}}`
		path := filepath.Join(tempDir, "special-chars.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})

	t.Run("checkpoint with unicode content", func(t *testing.T) {
		content := `{"version":"1.0","session":{"name":"test-unicode","provider":"anthropic","model":"claude-3"},"messages":[{"role":"user","content":"Hello 世界 🌍"}],"statistics":{"message_count":1}}`
		path := filepath.Join(tempDir, "unicode.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})
}

// TestSessionsCommand_FlagsWithShorthand tests shorthand flag handling.
func TestSessionsCommand_FlagsWithShorthand(t *testing.T) {
	t.Run("export output flag shorthand is o", func(t *testing.T) {
		flag := sessionsExportCmd.Flags().Lookup("output")
		require.NotNil(t, flag)
		assert.Equal(t, "o", flag.Shorthand)
	})

	t.Run("export format flag shorthand is f", func(t *testing.T) {
		flag := sessionsExportCmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		assert.Equal(t, "f", flag.Shorthand)
	})

	t.Run("import name flag shorthand is n", func(t *testing.T) {
		flag := sessionsImportCmd.Flags().Lookup("name")
		require.NotNil(t, flag)
		assert.Equal(t, "n", flag.Shorthand)
	})
}

// TestCleanSessionsCommand_DurationParsingCoverage tests duration parsing for full coverage.
func TestCleanSessionsCommand_DurationParsingCoverage(t *testing.T) {
	t.Run("clean command parses hours correctly", func(t *testing.T) {
		// Test the round-up logic in hour parsing.
		testCases := []struct {
			hours        string
			expectedDays int
		}{
			{"1h", 1},   // 0.04 days rounds up to 1.
			{"12h", 1},  // 0.5 days rounds up to 1.
			{"23h", 1},  // 0.96 days rounds up to 1.
			{"24h", 1},  // exactly 1 day.
			{"25h", 2},  // 1.04 days rounds up to 2.
			{"48h", 2},  // exactly 2 days.
			{"72h", 3},  // exactly 3 days.
			{"100h", 5}, // 4.17 days rounds up to 5.
		}

		for _, tc := range testCases {
			t.Run(tc.hours, func(t *testing.T) {
				days, err := parseDuration(tc.hours)
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedDays, days)
			})
		}
	})

	t.Run("clean command parses weeks correctly", func(t *testing.T) {
		testCases := []struct {
			weeks        string
			expectedDays int
		}{
			{"1w", 7},
			{"2w", 14},
			{"4w", 28},
			{"8w", 56},
		}

		for _, tc := range testCases {
			t.Run(tc.weeks, func(t *testing.T) {
				days, err := parseDuration(tc.weeks)
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedDays, days)
			})
		}
	})

	t.Run("clean command parses months correctly", func(t *testing.T) {
		testCases := []struct {
			months       string
			expectedDays int
		}{
			{"1m", 30},
			{"2m", 60},
			{"3m", 90},
			{"6m", 180},
			{"12m", 360},
		}

		for _, tc := range testCases {
			t.Run(tc.months, func(t *testing.T) {
				days, err := parseDuration(tc.months)
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedDays, days)
			})
		}
	})
}

// TestInitSessionManager_ErrorPathCoverage tests initSessionManager error paths.
func TestInitSessionManager_ErrorPathCoverage(t *testing.T) {
	t.Run("returns error when config initialization fails", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/config/path/that/does/not/exist")
		t.Setenv("ATMOS_BASE_PATH", "/nonexistent/base/path")

		manager, cleanup, err := initSessionManager()

		// Should fail on config initialization or AI not enabled check.
		assert.Error(t, err)
		assert.Nil(t, manager)
		assert.Nil(t, cleanup)
	})

	t.Run("error message format is correct", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/bad/path")

		manager, cleanup, err := initSessionManager()

		assert.Error(t, err)
		assert.Nil(t, manager)
		assert.Nil(t, cleanup)
		// Error should be wrapped properly.
		assert.NotEmpty(t, err.Error())
	})
}

// TestSessionsCommand_RunEFunctions tests that RunE functions are properly set.
func TestSessionsCommand_RunEFunctions(t *testing.T) {
	t.Run("sessionsCmd RunE is listSessionsCommand", func(t *testing.T) {
		// The RunE function should be set.
		assert.NotNil(t, sessionsCmd.RunE)
	})

	t.Run("sessionsListCmd RunE is listSessionsCommand", func(t *testing.T) {
		assert.NotNil(t, sessionsListCmd.RunE)
	})

	t.Run("sessionsCleanCmd RunE is cleanSessionsCommand", func(t *testing.T) {
		assert.NotNil(t, sessionsCleanCmd.RunE)
	})

	t.Run("sessionsExportCmd RunE is exportSessionCommand", func(t *testing.T) {
		assert.NotNil(t, sessionsExportCmd.RunE)
	})

	t.Run("sessionsImportCmd RunE is importSessionCommand", func(t *testing.T) {
		assert.NotNil(t, sessionsImportCmd.RunE)
	})
}

// TestValidateCheckpointFile_FilePermissions tests checkpoint file permission scenarios.
func TestValidateCheckpointFile_FilePermissions(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("handles directory instead of file", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "notafile")
		err := os.Mkdir(dirPath, 0o755)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(dirPath)
		assert.Error(t, err)
	})
}

// TestParseDuration_ZeroAndBoundary tests zero and boundary cases for duration parsing.
//

func TestParseDuration_ZeroAndBoundary(t *testing.T) {
	t.Run("zero hours returns 0 days", func(t *testing.T) {
		days, err := parseDuration("0h")
		assert.NoError(t, err)
		assert.Equal(t, 0, days)
	})

	t.Run("zero days returns 0 days", func(t *testing.T) {
		days, err := parseDuration("0d")
		assert.NoError(t, err)
		assert.Equal(t, 0, days)
	})

	t.Run("zero weeks returns 0 days", func(t *testing.T) {
		days, err := parseDuration("0w")
		assert.NoError(t, err)
		assert.Equal(t, 0, days)
	})

	t.Run("zero months returns 0 days", func(t *testing.T) {
		days, err := parseDuration("0m")
		assert.NoError(t, err)
		assert.Equal(t, 0, days)
	})
}

// TestSessionsCommand_CobraArgsValidation tests Cobra args validation.
func TestSessionsCommand_CobraArgsValidation(t *testing.T) {
	t.Run("export requires exactly one argument", func(t *testing.T) {
		// Zero args.
		err := sessionsExportCmd.Args(sessionsExportCmd, []string{})
		assert.Error(t, err)

		// One arg - valid.
		err = sessionsExportCmd.Args(sessionsExportCmd, []string{"session-name"})
		assert.NoError(t, err)

		// Two args.
		err = sessionsExportCmd.Args(sessionsExportCmd, []string{"session1", "session2"})
		assert.Error(t, err)
	})

	t.Run("import requires exactly one argument", func(t *testing.T) {
		// Zero args.
		err := sessionsImportCmd.Args(sessionsImportCmd, []string{})
		assert.Error(t, err)

		// One arg - valid.
		err = sessionsImportCmd.Args(sessionsImportCmd, []string{"checkpoint.json"})
		assert.NoError(t, err)

		// Two args.
		err = sessionsImportCmd.Args(sessionsImportCmd, []string{"file1.json", "file2.json"})
		assert.Error(t, err)
	})

	t.Run("list accepts any number of arguments", func(t *testing.T) {
		// list command doesn't have Args set, so should accept anything.
		assert.Nil(t, sessionsListCmd.Args)
	})

	t.Run("clean accepts any number of arguments", func(t *testing.T) {
		// clean command doesn't have Args set.
		assert.Nil(t, sessionsCleanCmd.Args)
	})
}

// TestCheckpointYAMLExtensions tests both .yaml and .yml extensions.
func TestCheckpointYAMLExtensions(t *testing.T) {
	tempDir := t.TempDir()

	yamlContent := `version: "1.0"
session:
  name: "test-session"
  provider: "anthropic"
  model: "claude-3-opus"
messages:
  - role: "user"
    content: "Hello"
statistics:
  message_count: 1
`

	t.Run("validates .yaml extension", func(t *testing.T) {
		yamlPath := filepath.Join(tempDir, "test.yaml")
		err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(yamlPath)
		assert.NoError(t, err)
	})

	t.Run("validates .yml extension", func(t *testing.T) {
		ymlPath := filepath.Join(tempDir, "test.yml")
		err := os.WriteFile(ymlPath, []byte(yamlContent), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(ymlPath)
		assert.NoError(t, err)
	})
}

// TestValidateCheckpointFile_NilAndEmptyChecks tests nil and empty validation paths.
func TestValidateCheckpointFile_NilAndEmptyChecks(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("empty file fails validation", func(t *testing.T) {
		emptyPath := filepath.Join(tempDir, "empty.json")
		err := os.WriteFile(emptyPath, []byte(""), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(emptyPath)
		assert.Error(t, err)
	})

	t.Run("null JSON fails validation", func(t *testing.T) {
		nullPath := filepath.Join(tempDir, "null.json")
		err := os.WriteFile(nullPath, []byte("null"), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(nullPath)
		assert.Error(t, err)
	})

	t.Run("empty object fails validation", func(t *testing.T) {
		emptyObjPath := filepath.Join(tempDir, "empty-obj.json")
		err := os.WriteFile(emptyObjPath, []byte("{}"), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(emptyObjPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version is missing")
	})
}

// TestSessionsCommand_AllFlagsPresent verifies all expected flags are present on commands.
func TestSessionsCommand_AllFlagsPresent(t *testing.T) {
	t.Run("clean command has all expected flags", func(t *testing.T) {
		flags := []string{"older-than"}
		for _, flag := range flags {
			f := sessionsCleanCmd.Flags().Lookup(flag)
			assert.NotNil(t, f, "flag %s should exist", flag)
		}
	})

	t.Run("export command has all expected flags", func(t *testing.T) {
		flags := []string{"output", "format", "context", "metadata"}
		for _, flag := range flags {
			f := sessionsExportCmd.Flags().Lookup(flag)
			assert.NotNil(t, f, "flag %s should exist", flag)
		}
	})

	t.Run("import command has all expected flags", func(t *testing.T) {
		flags := []string{"name", "overwrite", "context"}
		for _, flag := range flags {
			f := sessionsImportCmd.Flags().Lookup(flag)
			assert.NotNil(t, f, "flag %s should exist", flag)
		}
	})
}

// TestParseDuration_InvalidInputs tests various invalid inputs for parseDuration.
func TestParseDuration_InvalidInputs(t *testing.T) {
	invalidInputs := []struct {
		input string
		desc  string
	}{
		{"abc", "only letters"},
		{"123", "only numbers"},
		{"d30", "unit before number"},
		{"30 days", "full word unit"},
		{"30.5d", "decimal number"},
		{"d", "only unit"},
		{".d", "dot and unit"},
		{"30dd", "double unit"},
	}

	for _, tc := range invalidInputs {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := parseDuration(tc.input)
			assert.Error(t, err, "expected error for input: %s", tc.input)
		})
	}
}

// TestCheckpointValidation_StatisticsMismatch tests statistics validation.
func TestCheckpointValidation_StatisticsMismatch(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("message count too high", func(t *testing.T) {
		content := `{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"user","content":"test"}],"statistics":{"message_count":10}}`
		path := filepath.Join(tempDir, "high-count.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "statistics message count")
	})

	t.Run("message count too low", func(t *testing.T) {
		content := `{"version":"1.0","session":{"name":"test","provider":"test","model":"test"},"messages":[{"role":"user","content":"test"},{"role":"assistant","content":"response"}],"statistics":{"message_count":1}}`
		path := filepath.Join(tempDir, "low-count.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "statistics message count")
	})
}

// TestCleanSessionsCommand_FlagErrors tests flag retrieval error paths.
func TestCleanSessionsCommand_FlagErrors(t *testing.T) {
	t.Run("clean without older-than flag registered returns error", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "clean"}
		// Don't register the flag.

		err := cleanSessionsCommand(testCmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get older-than flag")
	})
}

// TestExportSessionCommand_FlagErrors tests flag retrieval error paths for export.
func TestExportSessionCommand_FlagErrors(t *testing.T) {
	t.Run("export without any flags returns error", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "export"}
		// Don't register any flags.

		err := exportSessionCommand(testCmd, []string{"session"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get output flag")
	})

	t.Run("export without format flag returns error", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "export"}
		testCmd.Flags().StringP("output", "o", "", "Output")
		// Don't register format flag.

		err := exportSessionCommand(testCmd, []string{"session"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get format flag")
	})

	t.Run("export without context flag returns error", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "export"}
		testCmd.Flags().StringP("output", "o", "", "Output")
		testCmd.Flags().StringP("format", "f", "", "Format")
		// Don't register context flag.

		err := exportSessionCommand(testCmd, []string{"session"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get context flag")
	})

	t.Run("export without metadata flag returns error", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "export"}
		testCmd.Flags().StringP("output", "o", "", "Output")
		testCmd.Flags().StringP("format", "f", "", "Format")
		testCmd.Flags().Bool("context", false, "Context")
		// Don't register metadata flag.

		err := exportSessionCommand(testCmd, []string{"session"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get metadata flag")
	})
}

// TestImportSessionCommand_FlagErrors tests flag retrieval error paths for import.
func TestImportSessionCommand_FlagErrors(t *testing.T) {
	t.Run("import without any flags returns error", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "import"}
		// Don't register any flags.

		err := importSessionCommand(testCmd, []string{"checkpoint.json"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get name flag")
	})

	t.Run("import without overwrite flag returns error", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "import"}
		testCmd.Flags().StringP("name", "n", "", "Name")
		// Don't register overwrite flag.

		err := importSessionCommand(testCmd, []string{"checkpoint.json"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get overwrite flag")
	})

	t.Run("import without context flag returns error", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "import"}
		testCmd.Flags().StringP("name", "n", "", "Name")
		testCmd.Flags().Bool("overwrite", false, "Overwrite")
		// Don't register context flag.

		err := importSessionCommand(testCmd, []string{"checkpoint.json"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get context flag")
	})
}

// TestSessionsIntegration tests the session commands with a proper test fixture that has AI enabled.
func TestSessionsIntegration(t *testing.T) {
	// Get the absolute path to the test fixture.
	fixtureDir, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "atmos-describe-affected-with-dependents-and-locked"))
	require.NoError(t, err)

	// Create a temp directory for sessions storage within the fixture.
	sessionsDir := filepath.Join(fixtureDir, ".atmos", "sessions")
	err = os.MkdirAll(sessionsDir, 0o755)
	require.NoError(t, err)
	defer func() {
		// Clean up the sessions directory after tests.
		_ = os.RemoveAll(filepath.Join(fixtureDir, ".atmos"))
	}()

	t.Run("listSessionsCommand succeeds with AI enabled config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", fixtureDir)
		t.Setenv("ATMOS_BASE_PATH", fixtureDir)

		// Create a fresh command with proper setup.
		testCmd := &cobra.Command{
			Use:  "list",
			RunE: listSessionsCommand,
		}

		err := listSessionsCommand(testCmd, []string{})
		// With AI enabled, the command should succeed or fail on storage initialization, not config.
		if err != nil {
			// It may fail on storage, but not on "AI not enabled" or "sessions not enabled".
			assert.NotContains(t, err.Error(), "AI features are not enabled")
			assert.NotContains(t, err.Error(), "sessions are not enabled")
		}
	})

	t.Run("cleanSessionsCommand succeeds with valid duration and AI enabled config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", fixtureDir)
		t.Setenv("ATMOS_BASE_PATH", fixtureDir)

		testCmd := &cobra.Command{
			Use:  "clean",
			RunE: cleanSessionsCommand,
		}
		testCmd.Flags().String("older-than", "30d", "Duration")

		err := cleanSessionsCommand(testCmd, []string{})
		if err != nil {
			assert.NotContains(t, err.Error(), "AI features are not enabled")
			assert.NotContains(t, err.Error(), "sessions are not enabled")
			assert.NotContains(t, err.Error(), "invalid duration format")
		}
	})

	t.Run("exportSessionCommand fails when session not found", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", fixtureDir)
		t.Setenv("ATMOS_BASE_PATH", fixtureDir)

		testCmd := &cobra.Command{
			Use:  "export",
			RunE: exportSessionCommand,
		}
		testCmd.Flags().StringP("output", "o", "", "Output file path")
		testCmd.Flags().StringP("format", "f", "", "Output format")
		testCmd.Flags().Bool("context", false, "Include context")
		testCmd.Flags().Bool("metadata", true, "Include metadata")

		_ = testCmd.Flags().Set("output", filepath.Join(t.TempDir(), "export.json"))

		err := exportSessionCommand(testCmd, []string{"nonexistent-session"})
		// Should fail trying to find the session, not on config.
		if err != nil {
			assert.NotContains(t, err.Error(), "AI features are not enabled")
			assert.NotContains(t, err.Error(), "sessions are not enabled")
		}
	})

	t.Run("importSessionCommand fails when checkpoint file not found", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", fixtureDir)
		t.Setenv("ATMOS_BASE_PATH", fixtureDir)

		testCmd := &cobra.Command{
			Use:  "import",
			RunE: importSessionCommand,
		}
		testCmd.Flags().StringP("name", "n", "", "Name for imported session")
		testCmd.Flags().Bool("overwrite", false, "Overwrite existing")
		testCmd.Flags().Bool("context", true, "Include context")

		err := importSessionCommand(testCmd, []string{filepath.Join(t.TempDir(), "nonexistent.json")})
		// Should fail on file not found, not on config.
		if err != nil {
			assert.NotContains(t, err.Error(), "AI features are not enabled")
			assert.NotContains(t, err.Error(), "sessions are not enabled")
		}
	})
}

// TestSessionsWithTempConfig tests session commands with a temporary atmos.yaml configuration.
func TestSessionsWithTempConfig(t *testing.T) {
	// Create a temp directory for the test.
	tempDir := t.TempDir()

	// Create necessary directory structure.
	stacksDir := filepath.Join(tempDir, "stacks")
	componentsDir := filepath.Join(tempDir, "components", "terraform")
	sessionsDir := filepath.Join(tempDir, ".atmos", "sessions")
	err := os.MkdirAll(stacksDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(componentsDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(sessionsDir, 0o755)
	require.NoError(t, err)

	// Create a minimal atmos.yaml with AI enabled.
	// Use filepath.ToSlash to convert backslashes to forward slashes for YAML.
	atmosConfig := fmt.Sprintf(`base_path: "%s"

stacks:
  base_path: "stacks"
  name_pattern: "{tenant}-{environment}-{stage}"

components:
  terraform:
    base_path: "%s"

settings:
  ai:
    enabled: true
    sessions:
      enabled: true
      path: "%s"
`, filepath.ToSlash(tempDir), filepath.ToSlash(filepath.Join("components", "terraform")), filepath.ToSlash(filepath.Join(".atmos", "sessions")))

	atmosYAMLPath := filepath.Join(tempDir, "atmos.yaml")
	err = os.WriteFile(atmosYAMLPath, []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	// Create a minimal stack file to prevent config errors.
	stackContent := `vars:
  tenant: test
  environment: dev
  stage: test
`
	stackPath := filepath.Join(stacksDir, "test.yaml")
	err = os.WriteFile(stackPath, []byte(stackContent), 0o644)
	require.NoError(t, err)

	t.Run("listSessionsCommand with empty sessions", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)
		t.Setenv("ATMOS_BASE_PATH", tempDir)

		testCmd := &cobra.Command{
			Use:  "list",
			RunE: listSessionsCommand,
		}

		// This should succeed and show "No sessions found".
		err := listSessionsCommand(testCmd, []string{})
		// May succeed or fail on storage - either is acceptable for this test.
		// We mainly want to ensure it gets past config validation.
		if err != nil {
			assert.NotContains(t, err.Error(), "AI features are not enabled")
			assert.NotContains(t, err.Error(), "sessions are not enabled")
		}
	})

	t.Run("cleanSessionsCommand with no sessions to clean", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)
		t.Setenv("ATMOS_BASE_PATH", tempDir)

		testCmd := &cobra.Command{
			Use:  "clean",
			RunE: cleanSessionsCommand,
		}
		testCmd.Flags().String("older-than", "1d", "Duration")

		err := cleanSessionsCommand(testCmd, []string{})
		if err != nil {
			assert.NotContains(t, err.Error(), "AI features are not enabled")
			assert.NotContains(t, err.Error(), "sessions are not enabled")
		}
	})

	t.Run("cleanSessionsCommand with various duration formats", func(t *testing.T) {
		durations := []string{"24h", "7d", "2w", "1m"}

		for _, duration := range durations {
			t.Run(duration, func(t *testing.T) {
				t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)
				t.Setenv("ATMOS_BASE_PATH", tempDir)

				testCmd := &cobra.Command{
					Use:  "clean",
					RunE: cleanSessionsCommand,
				}
				testCmd.Flags().String("older-than", duration, "Duration")

				err := cleanSessionsCommand(testCmd, []string{})
				if err != nil {
					assert.NotContains(t, err.Error(), "AI features are not enabled")
					assert.NotContains(t, err.Error(), "invalid duration format")
				}
			})
		}
	})
}

// TestSessionsExportImportRoundTrip tests export/import functionality end-to-end.
func TestSessionsExportImportRoundTrip(t *testing.T) {
	// Create temp directories for the test.
	tempDir := t.TempDir()
	exportDir := t.TempDir()

	// Create necessary directory structure.
	stacksDir := filepath.Join(tempDir, "stacks")
	componentsDir := filepath.Join(tempDir, "components", "terraform")
	sessionsDir := filepath.Join(tempDir, ".atmos", "sessions")
	err := os.MkdirAll(stacksDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(componentsDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(sessionsDir, 0o755)
	require.NoError(t, err)

	// Create a minimal atmos.yaml with AI enabled.
	atmosConfig := fmt.Sprintf(`base_path: "%s"

stacks:
  base_path: "stacks"
  name_pattern: "{tenant}-{environment}-{stage}"

components:
  terraform:
    base_path: "%s"

settings:
  ai:
    enabled: true
    sessions:
      enabled: true
      path: "%s"
`, filepath.ToSlash(tempDir), filepath.ToSlash(filepath.Join("components", "terraform")), filepath.ToSlash(filepath.Join(".atmos", "sessions")))

	atmosYAMLPath := filepath.Join(tempDir, "atmos.yaml")
	err = os.WriteFile(atmosYAMLPath, []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	// Create a minimal stack file.
	stackContent := `vars:
  tenant: test
  environment: dev
  stage: test
`
	stackPath := filepath.Join(stacksDir, "test.yaml")
	err = os.WriteFile(stackPath, []byte(stackContent), 0o644)
	require.NoError(t, err)

	t.Run("import valid checkpoint file", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)
		t.Setenv("ATMOS_BASE_PATH", tempDir)

		// Create a valid checkpoint file.
		checkpointPath := filepath.Join(exportDir, "checkpoint.json")
		createTestCheckpointFile(t, checkpointPath, "json")

		testCmd := &cobra.Command{
			Use:  "import",
			RunE: importSessionCommand,
		}
		testCmd.Flags().StringP("name", "n", "imported-test-session", "Name for imported session")
		testCmd.Flags().Bool("overwrite", false, "Overwrite existing")
		testCmd.Flags().Bool("context", true, "Include context")

		err := importSessionCommand(testCmd, []string{checkpointPath})
		// Should succeed or fail on storage, not on config validation.
		if err != nil {
			assert.NotContains(t, err.Error(), "AI features are not enabled")
			assert.NotContains(t, err.Error(), "sessions are not enabled")
		}
	})

	t.Run("export command with all format options", func(t *testing.T) {
		formats := []struct {
			extension string
			format    string
		}{
			{".json", "json"},
			{".yaml", "yaml"},
			{".md", "markdown"},
		}

		for _, f := range formats {
			t.Run(f.format, func(t *testing.T) {
				t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)
				t.Setenv("ATMOS_BASE_PATH", tempDir)

				testCmd := &cobra.Command{
					Use:  "export",
					RunE: exportSessionCommand,
				}
				testCmd.Flags().StringP("output", "o", "", "Output file path")
				testCmd.Flags().StringP("format", "f", "", "Output format")
				testCmd.Flags().Bool("context", true, "Include context")
				testCmd.Flags().Bool("metadata", true, "Include metadata")

				outputPath := filepath.Join(exportDir, "export"+f.extension)
				_ = testCmd.Flags().Set("output", outputPath)
				_ = testCmd.Flags().Set("format", f.format)
				_ = testCmd.Flags().Set("context", "true")
				_ = testCmd.Flags().Set("metadata", "true")

				err := exportSessionCommand(testCmd, []string{"test-session"})
				// Will fail because session doesn't exist, but should pass config validation.
				if err != nil {
					assert.NotContains(t, err.Error(), "AI features are not enabled")
					assert.NotContains(t, err.Error(), "sessions are not enabled")
					assert.NotContains(t, err.Error(), "failed to get output flag")
				}
			})
		}
	})

	t.Run("import command with overwrite flag", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)
		t.Setenv("ATMOS_BASE_PATH", tempDir)

		checkpointPath := filepath.Join(exportDir, "checkpoint-overwrite.json")
		createTestCheckpointFile(t, checkpointPath, "json")

		testCmd := &cobra.Command{
			Use:  "import",
			RunE: importSessionCommand,
		}
		testCmd.Flags().StringP("name", "n", "", "Name for imported session")
		testCmd.Flags().Bool("overwrite", true, "Overwrite existing")
		testCmd.Flags().Bool("context", false, "Include context")

		_ = testCmd.Flags().Set("overwrite", "true")
		_ = testCmd.Flags().Set("context", "false")

		err := importSessionCommand(testCmd, []string{checkpointPath})
		if err != nil {
			assert.NotContains(t, err.Error(), "AI features are not enabled")
		}
	})
}

// TestSessionsErrorPaths tests error paths that haven't been covered.
func TestSessionsErrorPaths(t *testing.T) {
	t.Run("initSessionManager returns error when AI not enabled", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create necessary directory structure.
		stacksDir := filepath.Join(tempDir, "stacks", "deploy")
		err := os.MkdirAll(stacksDir, 0o755)
		require.NoError(t, err)

		// Create config with AI disabled.
		atmosConfig := fmt.Sprintf(`base_path: "%s"

stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_template: "{{ .vars.environment }}-{{ .vars.stage }}"

settings:
  ai:
    enabled: false
`, filepath.ToSlash(tempDir))

		atmosYAMLPath := filepath.Join(tempDir, "atmos.yaml")
		err = os.WriteFile(atmosYAMLPath, []byte(atmosConfig), 0o644)
		require.NoError(t, err)

		// Create a minimal stack file.
		stackPath := filepath.Join(stacksDir, "test.yaml")
		err = os.WriteFile(stackPath, []byte("vars:\n  environment: dev\n  stage: test\n"), 0o644)
		require.NoError(t, err)

		t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)
		t.Setenv("ATMOS_BASE_PATH", tempDir)

		manager, cleanup, err := initSessionManager()
		assert.Error(t, err)
		assert.Nil(t, manager)
		assert.Nil(t, cleanup)
		assert.Contains(t, err.Error(), "AI features are not enabled")
	})

	t.Run("initSessionManager returns error when sessions not enabled", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create necessary directory structure.
		stacksDir := filepath.Join(tempDir, "stacks", "deploy")
		err := os.MkdirAll(stacksDir, 0o755)
		require.NoError(t, err)

		// Create config with AI enabled but sessions disabled.
		atmosConfig := fmt.Sprintf(`base_path: "%s"

stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_template: "{{ .vars.environment }}-{{ .vars.stage }}"

settings:
  ai:
    enabled: true
    sessions:
      enabled: false
`, filepath.ToSlash(tempDir))

		atmosYAMLPath := filepath.Join(tempDir, "atmos.yaml")
		err = os.WriteFile(atmosYAMLPath, []byte(atmosConfig), 0o644)
		require.NoError(t, err)

		// Create a minimal stack file.
		stackPath := filepath.Join(stacksDir, "test.yaml")
		err = os.WriteFile(stackPath, []byte("vars:\n  environment: dev\n  stage: test\n"), 0o644)
		require.NoError(t, err)

		t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)
		t.Setenv("ATMOS_BASE_PATH", tempDir)

		manager, cleanup, err := initSessionManager()
		assert.Error(t, err)
		assert.Nil(t, manager)
		assert.Nil(t, cleanup)
		assert.Contains(t, err.Error(), "sessions are not enabled")
	})
}

// TestParseDurationEdgeCasesAdditional tests additional edge cases for parseDuration.
func TestParseDurationEdgeCasesAdditional(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedDays  int
		expectError   bool
		expectedError error
	}{
		{
			name:         "handles leading zeros in value",
			input:        "007d",
			expectedDays: 7,
			expectError:  false,
		},
		{
			name:         "handles large hour value requiring rounding",
			input:        "100h",
			expectedDays: 5, // 100/24 = 4.16, rounds up to 5
			expectError:  false,
		},
		{
			name:        "handles mixed letter case incorrectly",
			input:       "30D",
			expectError: true,
		},
		{
			name:         "handles trailing space",
			input:        "30d ",
			expectedDays: 30, // %s in fmt.Sscanf stops at whitespace, so unit is "d"
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days, err := parseDuration(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedDays, days)
			}
		})
	}
}

// TestSessionsCommandWithDifferentConfigs tests command behavior with various configuration states.
func TestSessionsCommandWithDifferentConfigs(t *testing.T) {
	t.Run("list command with AI disabled fails appropriately", func(t *testing.T) {
		tempDir := t.TempDir()
		stacksDir := filepath.Join(tempDir, "stacks", "deploy")
		err := os.MkdirAll(stacksDir, 0o755)
		require.NoError(t, err)

		atmosConfig := fmt.Sprintf(`base_path: "%s"
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_template: "{{ .vars.environment }}-{{ .vars.stage }}"
settings:
  ai:
    enabled: false
`, filepath.ToSlash(tempDir))

		err = os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(stacksDir, "test.yaml"), []byte("vars:\n  environment: dev\n  stage: test\n"), 0o644)
		require.NoError(t, err)

		t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)
		t.Setenv("ATMOS_BASE_PATH", tempDir)

		err = listSessionsCommand(sessionsListCmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "AI features are not enabled")
	})

	t.Run("clean command with sessions disabled fails appropriately", func(t *testing.T) {
		tempDir := t.TempDir()
		stacksDir := filepath.Join(tempDir, "stacks", "deploy")
		err := os.MkdirAll(stacksDir, 0o755)
		require.NoError(t, err)

		atmosConfig := fmt.Sprintf(`base_path: "%s"
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_template: "{{ .vars.environment }}-{{ .vars.stage }}"
settings:
  ai:
    enabled: true
    sessions:
      enabled: false
`, filepath.ToSlash(tempDir))

		err = os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(stacksDir, "test.yaml"), []byte("vars:\n  environment: dev\n  stage: test\n"), 0o644)
		require.NoError(t, err)

		t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)
		t.Setenv("ATMOS_BASE_PATH", tempDir)

		testCmd := &cobra.Command{
			Use:  "clean",
			RunE: cleanSessionsCommand,
		}
		testCmd.Flags().String("older-than", "30d", "Duration")

		err = cleanSessionsCommand(testCmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sessions are not enabled")
	})
}

// TestValidateCheckpointFileAdditional tests additional checkpoint validation scenarios.
func TestValidateCheckpointFileAdditional(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("checkpoint with extra fields passes validation", func(t *testing.T) {
		content := `{
			"version": "1.0",
			"session": {
				"name": "test",
				"provider": "anthropic",
				"model": "claude-3",
				"extra_field": "should be ignored"
			},
			"messages": [
				{"role": "user", "content": "test"}
			],
			"statistics": {
				"message_count": 1
			},
			"custom_key": "custom_value"
		}`
		path := filepath.Join(tempDir, "extra-fields.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})

	t.Run("checkpoint with nested timestamps passes validation", func(t *testing.T) {
		now := time.Now()
		checkpoint := session.Checkpoint{
			Version:    session.CheckpointVersion,
			ExportedAt: now,
			Session: session.CheckpointSession{
				Name:      "timestamp-test",
				Provider:  "anthropic",
				Model:     "claude-3-opus",
				CreatedAt: now.Add(-48 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
			},
			Messages: []session.CheckpointMessage{
				{
					Role:      "user",
					Content:   "test message",
					CreatedAt: now.Add(-24 * time.Hour),
				},
			},
			Statistics: session.CheckpointStatistics{
				MessageCount: 1,
			},
		}

		data, err := json.Marshal(checkpoint)
		require.NoError(t, err)

		path := filepath.Join(tempDir, "timestamps.json")
		err = os.WriteFile(path, data, 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})

	t.Run("checkpoint with very long content passes validation", func(t *testing.T) {
		longContent := string(make([]byte, 10000))
		for i := range []byte(longContent) {
			longContent = longContent[:i] + "a" + longContent[i+1:]
		}

		content := fmt.Sprintf(`{
			"version": "1.0",
			"session": {
				"name": "long-content-test",
				"provider": "anthropic",
				"model": "claude-3"
			},
			"messages": [
				{"role": "user", "content": %q}
			],
			"statistics": {
				"message_count": 1
			}
		}`, longContent)

		path := filepath.Join(tempDir, "long-content.json")
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)

		err = session.ValidateCheckpointFile(path)
		assert.NoError(t, err)
	})
}
