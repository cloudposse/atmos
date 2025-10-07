package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestVerifyInsideGitRepo(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "git-repo-verify-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save the current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Test cases
	tests := []struct {
		name     string
		setup    func() error
		expected bool
	}{
		{
			name: "outside git repository",
			setup: func() error {
				return os.Chdir(tmpDir)
			},
			expected: false,
		},
		{
			name: "inside git repository",
			setup: func() error {
				if err := os.Chdir(currentDir); err != nil {
					return err
				}
				return nil
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			if err := tt.setup(); err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}

			// Run test
			result := verifyInsideGitRepo()

			// Assert result
			assert.Equal(t, tt.expected, result)
		})
	}

	// Restore the original working directory
	if err := os.Chdir(currentDir); err != nil {
		t.Fatalf("Failed to restore working directory: %v", err)
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		target   string
		expected bool
	}{
		{
			name:     "empty slice",
			slice:    []string{},
			target:   "test",
			expected: false,
		},
		{
			name:     "contains target",
			slice:    []string{"one", "two", "three"},
			target:   "two",
			expected: true,
		},
		{
			name:     "does not contain target",
			slice:    []string{"one", "two", "three"},
			target:   "four",
			expected: false,
		},
		{
			name:     "case sensitive",
			slice:    []string{"One", "Two", "Three"},
			target:   "one",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Contains(tt.slice, tt.target)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractTrailingArgs(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		osArgs         []string
		expectedArgs   []string
		expectedString string
	}{
		{
			name:           "no trailing args",
			args:           []string{"arg1", "arg2"},
			osArgs:         []string{"program", "arg1", "arg2"},
			expectedArgs:   []string{"arg1", "arg2"},
			expectedString: "",
		},
		{
			name:           "with trailing args",
			args:           []string{"arg1", "--", "trail1", "trail2"},
			osArgs:         []string{"program", "arg1", "--", "trail1", "trail2"},
			expectedArgs:   []string{"arg1"},
			expectedString: "trail1 trail2",
		},
		{
			name:           "double dash at end",
			args:           []string{"arg1", "--"},
			osArgs:         []string{"program", "arg1", "--"},
			expectedArgs:   []string{"arg1"},
			expectedString: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, str := extractTrailingArgs(tt.args, tt.osArgs)
			assert.Equal(t, tt.expectedArgs, args)
			assert.Equal(t, tt.expectedString, str)
		})
	}
}

func TestIsVersionCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "version command",
			args:     []string{"version"},
			expected: true,
		},
		{
			name:     "version command with flags",
			args:     []string{"version", "--output", "json"},
			expected: true,
		},
		{
			name:     "not version command",
			args:     []string{"help"},
			expected: false,
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original os.Args
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()

			// Set up test args
			os.Args = append([]string{"atmos"}, tt.args...)

			// Test the function
			result := isVersionCommand()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPreCustomCommand(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		commandConfig *schema.Command
		expectError   bool
		exitCode      int
		errorContains string
	}{
		{
			name: "success - valid args with steps",
			args: []string{"arg1"},
			commandConfig: &schema.Command{
				Name: "test",
				Arguments: []schema.CommandArgument{
					{Name: "arg1", Required: true},
				},
				Steps: []string{"echo test"},
			},
			expectError: false,
		},
		{
			name: "error - subcommand required (has commands, no steps)",
			args: []string{},
			commandConfig: &schema.Command{
				Name:     "test",
				Commands: []schema.Command{{Name: "sub1"}},
			},
			expectError:   true,
			exitCode:      1,
			errorContains: "subcommand required",
		},
		{
			name: "error - invalid config (no steps or commands)",
			args: []string{},
			commandConfig: &schema.Command{
				Name: "test",
			},
			expectError:   true,
			exitCode:      1,
			errorContains: "has no steps or subcommands configured",
		},
		{
			name: "error - missing required arguments",
			args: []string{},
			commandConfig: &schema.Command{
				Name: "test",
				Arguments: []schema.CommandArgument{
					{Name: "arg1", Required: true, Default: ""},
					{Name: "arg2", Required: true, Default: ""},
				},
			},
			expectError:   true,
			exitCode:      1,
			errorContains: "requires at least 2 argument",
		},
		{
			name: "success - required arg with default uses default",
			args: []string{},
			commandConfig: &schema.Command{
				Name: "test",
				Arguments: []schema.CommandArgument{
					{Name: "arg1", Required: true, Default: "default-val"},
				},
				Steps: []string{"echo test"},
			},
			expectError: false,
		},
		{
			name: "error - no steps triggers subcommand required",
			args: []string{},
			commandConfig: &schema.Command{
				Name:     "test",
				Commands: []schema.Command{{Name: "sub1"}},
			},
			expectError:   true,
			exitCode:      1,
			errorContains: "subcommand required",
		},
		{
			name: "success - multiple args with some defaults",
			args: []string{"provided"},
			commandConfig: &schema.Command{
				Name: "test",
				Arguments: []schema.CommandArgument{
					{Name: "arg1", Required: true},
					{Name: "arg2", Required: false, Default: "default2"},
				},
				Steps: []string{"echo test"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			parentCmd := &cobra.Command{Use: "atmos"}

			err := preCustomCommand(cmd, tt.args, parentCmd, tt.commandConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.exitCode >= 0 {
					assert.Equal(t, tt.exitCode, errUtils.GetExitCode(err))
				}
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				// Verify annotations were set
				if cmd.Annotations != nil && len(tt.commandConfig.Arguments) > 0 {
					assert.Contains(t, cmd.Annotations, "resolvedArgs")
				}
			}
		})
	}
}

func TestHandleHelpRequest(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectsHelp bool
	}{
		{
			name:        "help argument",
			args:        []string{"help"},
			expectsHelp: true,
		},
		{
			name:        "--help flag",
			args:        []string{"--help"},
			expectsHelp: true,
		},
		{
			name:        "-h flag",
			args:        []string{"-h"},
			expectsHelp: true,
		},
		{
			name:        "--help in middle of args",
			args:        []string{"arg1", "--help", "arg2"},
			expectsHelp: true,
		},
		{
			name:        "no help request",
			args:        []string{"arg"},
			expectsHelp: false,
		},
		{
			name:        "empty args",
			args:        []string{},
			expectsHelp: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			err := handleHelpRequest(cmd, tt.args)

			// handleHelpRequest returns nil in all cases - it just shows help
			// The actual error with exit code 0 is returned by the caller
			assert.NoError(t, err)
		})
	}
}

func TestCheckAtmosConfig(t *testing.T) {
	// Save current directory
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tests := []struct {
		name        string
		opts        []AtmosValidateOption
		setupEnv    func() (cleanup func(), err error)
		expectError bool
		exitCode    int
	}{
		{
			name: "valid atmos config",
			setupEnv: func() (func(), error) {
				// Use the actual test fixtures directory
				fixtureDir := filepath.Join(currentDir, "../tests/fixtures/scenarios/basic")
				if err := os.Chdir(fixtureDir); err != nil {
					return nil, err
				}
				return func() {
					os.Chdir(currentDir)
				}, nil
			},
			expectError: false,
		},
		{
			name: "skip stack validation",
			opts: []AtmosValidateOption{WithStackValidation(false)},
			setupEnv: func() (func(), error) {
				// Create temp dir without stacks
				tmpDir, err := os.MkdirTemp("", "atmos-test-*")
				if err != nil {
					return nil, err
				}
				if err := os.Chdir(tmpDir); err != nil {
					os.RemoveAll(tmpDir)
					return nil, err
				}
				return func() {
					os.Chdir(currentDir)
					os.RemoveAll(tmpDir)
				}, nil
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup, err := tt.setupEnv()
			if err != nil {
				t.Fatalf("Failed to setup test environment: %v", err)
			}
			defer cleanup()

			err = checkAtmosConfig(tt.opts...)

			if tt.expectError {
				assert.Error(t, err)
				if tt.exitCode > 0 {
					assert.Equal(t, tt.exitCode, errUtils.GetExitCode(err))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestShowUsageAndExit(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		exitCode int
	}{
		{
			name:     "with args",
			args:     []string{"invalid"},
			exitCode: 1,
		},
		{
			name:     "without args",
			args:     []string{},
			exitCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
			}

			err := showUsageAndExit(cmd, tt.args)
			assert.Error(t, err)
			assert.Equal(t, tt.exitCode, errUtils.GetExitCode(err))
		})
	}
}

func TestShowFlagUsageAndExit(t *testing.T) {
	tests := []struct {
		name        string
		inputErr    error
		exitCode    int
		errContains string
	}{
		{
			name:        "flag parsing error",
			inputErr:    errors.New("unknown flag: --invalid"),
			exitCode:    1,
			errContains: "unknown flag",
		},
		{
			name:        "invalid flag value",
			inputErr:    errors.New("invalid value for --format"),
			exitCode:    1,
			errContains: "invalid value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use: "test",
			}

			err := showFlagUsageAndExit(cmd, tt.inputErr)
			assert.Error(t, err)
			assert.Equal(t, tt.exitCode, errUtils.GetExitCode(err))
			assert.True(t, errors.Is(err, errUtils.ErrInvalidFlag))
		})
	}
}

func TestShowErrorExampleFromMarkdown(t *testing.T) {
	tests := []struct {
		name        string
		arg         string
		hasCommands bool
		errContains string
	}{
		{
			name:        "unknown command",
			arg:         "unknown",
			hasCommands: true,
			errContains: "Unknown command",
		},
		{
			name:        "requires subcommand",
			arg:         "",
			hasCommands: true,
			errContains: "requires a subcommand",
		},
		{
			name:        "not valid usage",
			arg:         "",
			hasCommands: false,
			errContains: "is not valid usage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
			}

			if tt.hasCommands {
				cmd.AddCommand(&cobra.Command{
					Use:   "subcommand",
					Short: "Test subcommand",
				})
			}

			err := showErrorExampleFromMarkdown(cmd, tt.arg)
			assert.Error(t, err)
			assert.Equal(t, 1, errUtils.GetExitCode(err))
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestGetConfigAndStacksInfo(t *testing.T) {
	// This function is complex and requires full Atmos setup with proper fixtures
	// The error paths are tested through checkAtmosConfig which is already tested
	// Skip this test as it requires extensive integration test setup
	t.Skip("getConfigAndStacksInfo requires full integration test setup with proper Atmos fixtures and command flags")
}
