package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/profiler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
)

// TestGetInvalidCommandName tests extraction of command names from error messages.
func TestGetInvalidCommandName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard error format",
			input:    `unknown command "foobar" for "atmos"`,
			expected: "foobar",
		},
		{
			name:     "command with hyphens",
			input:    `unknown command "my-custom-cmd" for "atmos"`,
			expected: "my-custom-cmd",
		},
		{
			name:     "empty quotes",
			input:    `unknown command "" for "atmos"`,
			expected: "",
		},
		{
			name:     "no match",
			input:    "some other error message",
			expected: "",
		},
		{
			name:     "multiple quoted strings (should extract first)",
			input:    `unknown command "first" and "second"`,
			expected: "first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getInvalidCommandName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCleanupLogFile tests log file cleanup functionality.
func TestCleanupLogFile(t *testing.T) {
	tests := []struct {
		name       string
		setupFile  bool
		expectLogs bool
	}{
		{
			name:       "cleanup with open log file",
			setupFile:  true,
			expectLogs: false,
		},
		{
			name:       "cleanup with nil log file",
			setupFile:  false,
			expectLogs: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original log file handle.
			originalHandle := logFileHandle
			defer func() {
				logFileHandle = originalHandle
			}()

			if tt.setupFile {
				// Create a temporary log file.
				tmpFile, err := os.CreateTemp("", "atmos-test-*.log")
				assert.NoError(t, err)
				tmpPath := tmpFile.Name()
				defer os.Remove(tmpPath)

				logFileHandle = tmpFile
			} else {
				logFileHandle = nil
			}

			// Call cleanup - should not panic.
			assert.NotPanics(t, func() {
				cleanupLogFile()
			})

			// Verify logFileHandle is set to nil after cleanup.
			assert.Nil(t, logFileHandle)
		})
	}
}

// TestCleanup tests the public Cleanup function.
func TestCleanup(t *testing.T) {
	// Save original log file handle.
	originalHandle := logFileHandle
	defer func() {
		logFileHandle = originalHandle
	}()

	// Create a temporary log file.
	tmpFile, err := os.CreateTemp("", "atmos-test-*.log")
	assert.NoError(t, err)
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	logFileHandle = tmpFile

	// Call Cleanup - should not panic.
	assert.NotPanics(t, func() {
		Cleanup()
	})

	// Verify logFileHandle is set to nil after cleanup.
	assert.Nil(t, logFileHandle)
}

// TestSetupLogger_LogFileHandling tests log file handling with different outputs.
func TestSetupLogger_LogFileHandling(t *testing.T) {
	// Save original state.
	originalHandle := logFileHandle
	defer func() {
		logFileHandle = originalHandle
		log.SetOutput(os.Stderr)
	}()

	tests := []struct {
		name        string
		logFile     string
		expectFile  bool
		expectError bool
	}{
		{
			name:       "stderr output",
			logFile:    "/dev/stderr",
			expectFile: false,
		},
		{
			name:       "stdout output",
			logFile:    "/dev/stdout",
			expectFile: false,
		},
		{
			name:       "null output",
			logFile:    "/dev/null",
			expectFile: false,
		},
		{
			name:       "empty log file (no output set)",
			logFile:    "",
			expectFile: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset logFileHandle before each test.
			if logFileHandle != nil {
				logFileHandle.Close()
				logFileHandle = nil
			}

			cfg := &schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Info",
					File:  tt.logFile,
				},
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{},
				},
			}

			// Should not panic.
			assert.NotPanics(t, func() {
				SetupLogger(cfg)
			})

			if tt.expectFile {
				assert.NotNil(t, logFileHandle, "Expected log file handle to be set")
			}
		})
	}
}

// TestGetBaseProfilerConfig tests profiler configuration defaults.
func TestGetBaseProfilerConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   schema.AtmosConfiguration
		expected profiler.Config
	}{
		{
			name: "empty config uses defaults",
			config: schema.AtmosConfiguration{
				Profiler: profiler.Config{},
			},
			expected: profiler.DefaultConfig(),
		},
		{
			name: "partial config fills in defaults",
			config: schema.AtmosConfiguration{
				Profiler: profiler.Config{
					Enabled: true,
					Port:    0, // Should get default
				},
			},
			expected: profiler.Config{
				Enabled:     true,
				Host:        profiler.DefaultConfig().Host,
				Port:        profiler.DefaultConfig().Port,
				ProfileType: profiler.DefaultConfig().ProfileType,
			},
		},
		{
			name: "fully specified config preserved",
			config: schema.AtmosConfiguration{
				Profiler: profiler.Config{
					Enabled:     true,
					Host:        "custom-host",
					Port:        9999,
					ProfileType: profiler.ProfileTypeCPU,
				},
			},
			expected: profiler.Config{
				Enabled:     true,
				Host:        "custom-host",
				Port:        9999,
				ProfileType: profiler.ProfileTypeCPU,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getBaseProfilerConfig(&tt.config)
			assert.Equal(t, tt.expected.Enabled, result.Enabled)
			assert.Equal(t, tt.expected.Host, result.Host)
			assert.Equal(t, tt.expected.Port, result.Port)
			assert.Equal(t, tt.expected.ProfileType, result.ProfileType)
		})
	}
}

// TestApplyProfilerEnvironmentOverrides tests environment variable overrides for profiler.
func TestApplyProfilerEnvironmentOverrides(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
		initial     profiler.Config
		expected    profiler.Config
		expectError bool
	}{
		{
			name: "override host",
			atmosConfig: schema.AtmosConfiguration{
				Profiler: profiler.Config{
					Host: "new-host",
				},
			},
			initial: profiler.DefaultConfig(),
			expected: profiler.Config{
				Host:        "new-host",
				Port:        profiler.DefaultConfig().Port,
				ProfileType: profiler.DefaultConfig().ProfileType,
				Enabled:     false,
			},
		},
		{
			name: "override port",
			atmosConfig: schema.AtmosConfiguration{
				Profiler: profiler.Config{
					Port: 8080,
				},
			},
			initial: profiler.DefaultConfig(),
			expected: profiler.Config{
				Host:        profiler.DefaultConfig().Host,
				Port:        8080,
				ProfileType: profiler.DefaultConfig().ProfileType,
				Enabled:     false,
			},
		},
		{
			name: "file enables profiler automatically",
			atmosConfig: schema.AtmosConfiguration{
				Profiler: profiler.Config{
					File: "/tmp/profile.out",
				},
			},
			initial: profiler.DefaultConfig(),
			expected: profiler.Config{
				Host:        profiler.DefaultConfig().Host,
				Port:        profiler.DefaultConfig().Port,
				ProfileType: profiler.DefaultConfig().ProfileType,
				File:        "/tmp/profile.out",
				Enabled:     true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.initial
			err := applyProfilerEnvironmentOverrides(&config, &tt.atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.Host, config.Host)
				assert.Equal(t, tt.expected.Port, config.Port)
				assert.Equal(t, tt.expected.Enabled, config.Enabled)
				if tt.expected.File != "" {
					assert.Equal(t, tt.expected.File, config.File)
				}
			}
		})
	}
}

// TestApplyCLIFlagOverrides tests CLI flag overrides for profiler.
func TestApplyCLIFlagOverrides(t *testing.T) {
	tests := []struct {
		name        string
		setupCmd    func(*cobra.Command)
		initial     profiler.Config
		expected    profiler.Config
		expectError bool
	}{
		{
			name: "override enabled flag",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().Bool("profiler-enabled", false, "")
				cmd.Flags().Set("profiler-enabled", "true")
			},
			initial: profiler.DefaultConfig(),
			expected: profiler.Config{
				Enabled:     true,
				Host:        profiler.DefaultConfig().Host,
				Port:        profiler.DefaultConfig().Port,
				ProfileType: profiler.DefaultConfig().ProfileType,
			},
		},
		{
			name: "override port flag",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().Int("profiler-port", 0, "")
				cmd.Flags().Set("profiler-port", "9090")
			},
			initial: profiler.DefaultConfig(),
			expected: profiler.Config{
				Enabled:     false,
				Host:        profiler.DefaultConfig().Host,
				Port:        9090,
				ProfileType: profiler.DefaultConfig().ProfileType,
			},
		},
		{
			name: "no flags changed",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().Bool("profiler-enabled", false, "")
				cmd.Flags().Int("profiler-port", 0, "")
			},
			initial: profiler.DefaultConfig(),
			expected: profiler.Config{
				Enabled:     false,
				Host:        profiler.DefaultConfig().Host,
				Port:        profiler.DefaultConfig().Port,
				ProfileType: profiler.DefaultConfig().ProfileType,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			tt.setupCmd(cmd)

			config := tt.initial
			err := applyCLIFlagOverrides(&config, cmd)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.Enabled, config.Enabled)
				assert.Equal(t, tt.expected.Port, config.Port)
			}
		})
	}
}

// TestApplyProfileFileFlag tests profile file flag handling.
func TestApplyProfileFileFlag(t *testing.T) {
	tests := []struct {
		name            string
		setupCmd        func(*cobra.Command)
		expectedFile    string
		expectedEnabled bool
	}{
		{
			name: "file flag enables profiler",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().String("profile-file", "", "")
				cmd.Flags().Set("profile-file", "/tmp/cpu.prof")
			},
			expectedFile:    "/tmp/cpu.prof",
			expectedEnabled: true,
		},
		{
			name: "empty file flag does not enable profiler",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().String("profile-file", "", "")
				cmd.Flags().Set("profile-file", "")
			},
			expectedFile:    "",
			expectedEnabled: false,
		},
		{
			name: "flag not set",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().String("profile-file", "", "")
			},
			expectedFile:    "",
			expectedEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			tt.setupCmd(cmd)

			config := profiler.DefaultConfig()
			err := applyProfileFileFlag(&config, cmd)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFile, config.File)
			assert.Equal(t, tt.expectedEnabled, config.Enabled)
		})
	}
}

// TestApplyProfileTypeFlag tests profile type flag handling.
func TestApplyProfileTypeFlag(t *testing.T) {
	tests := []struct {
		name        string
		setupCmd    func(*cobra.Command)
		expected    profiler.ProfileType
		expectError bool
	}{
		{
			name: "valid cpu type",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().String("profile-type", "", "")
				cmd.Flags().Set("profile-type", "cpu")
			},
			expected: profiler.ProfileTypeCPU,
		},
		{
			name: "valid heap type",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().String("profile-type", "", "")
				cmd.Flags().Set("profile-type", "heap")
			},
			expected: profiler.ProfileTypeHeap,
		},
		{
			name: "invalid type",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().String("profile-type", "", "")
				cmd.Flags().Set("profile-type", "invalid")
			},
			expectError: true,
		},
		{
			name: "flag not set",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().String("profile-type", "", "")
			},
			expected: profiler.DefaultConfig().ProfileType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			tt.setupCmd(cmd)

			config := profiler.DefaultConfig()
			err := applyProfileTypeFlag(&config, cmd)

			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrParseFlag)
			} else {
				assert.NoError(t, err)
				if cmd.Flags().Changed("profile-type") {
					assert.Equal(t, tt.expected, config.ProfileType)
				}
			}
		})
	}
}

// TestHandleConfigInitError tests config initialization error handling.
func TestHandleConfigInitError(t *testing.T) {
	tests := []struct {
		name        string
		initErr     error
		isVersion   bool
		expectError bool
		expectNil   bool
	}{
		{
			name:      "version command with error returns nil",
			initErr:   errors.New("config error"),
			isVersion: true,
			expectNil: true,
		},
		{
			name:      "config not found returns nil",
			initErr:   fmt.Errorf("wrapped: %w", cfg.NotFound),
			isVersion: false,
			expectNil: true,
		},
		{
			name:        "invalid log level error preserved",
			initErr:     fmt.Errorf("%w\nSupported levels: Info, Debug", log.ErrInvalidLogLevel),
			isVersion:   false,
			expectError: true,
		},
		{
			name:        "other errors returned as-is",
			initErr:     errors.New("some other error"),
			isVersion:   false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore os.Args for version command detection.
			originalArgs := os.Args
			defer func() { os.Args = originalArgs }()

			if tt.isVersion {
				os.Args = []string{"atmos", "version"}
			} else {
				os.Args = []string{"atmos", "terraform", "plan"}
			}

			atmosConfig := &schema.AtmosConfiguration{}
			err := handleConfigInitError(tt.initErr, atmosConfig)

			switch {
			case tt.expectNil:
				assert.Nil(t, err)
			case tt.expectError:
				assert.Error(t, err)
			default:
				assert.NoError(t, err)
			}
		})
	}
}

// TestSetupLogger_InvalidLogLevel tests error handling for invalid log levels.
func TestSetupLogger_InvalidLogLevel(t *testing.T) {
	// Save original OsExit and restore after test.
	originalOsExit := errUtils.OsExit
	defer func() {
		errUtils.OsExit = originalOsExit
		log.SetOutput(os.Stderr)
	}()

	// Mock OsExit to panic so we can test the error path.
	type exitPanic struct {
		code int
	}
	var exitCode int
	errUtils.OsExit = func(code int) {
		exitCode = code
		panic(exitPanic{code: code})
	}

	cfg := &schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "InvalidLevel",
			File:  "",
		},
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{},
		},
	}

	// Should panic with exit code.
	assert.Panics(t, func() {
		SetupLogger(cfg)
	})

	// Verify it exits with non-zero code.
	assert.NotEqual(t, 0, exitCode)
}

// TestApplyBoolFlag tests boolean flag application helper.
func TestApplyBoolFlag(t *testing.T) {
	tests := []struct {
		name     string
		flagSet  bool
		flagVal  string
		expected bool
		called   bool
	}{
		{
			name:     "flag set to true",
			flagSet:  true,
			flagVal:  "true",
			expected: true,
			called:   true,
		},
		{
			name:     "flag set to false",
			flagSet:  true,
			flagVal:  "false",
			expected: false,
			called:   true,
		},
		{
			name:     "flag not set",
			flagSet:  false,
			expected: false,
			called:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().Bool("test-flag", false, "")
			if tt.flagSet {
				cmd.Flags().Set("test-flag", tt.flagVal)
			}

			var actualValue bool
			var setterCalled bool
			setter := func(val bool) {
				actualValue = val
				setterCalled = true
			}

			applyBoolFlag(cmd, "test-flag", setter)

			assert.Equal(t, tt.called, setterCalled)
			if tt.called {
				assert.Equal(t, tt.expected, actualValue)
			}
		})
	}
}

// TestApplyIntFlag tests integer flag application helper.
//
//nolint:dupl // Similar structure to other flag tests is intentional
func TestApplyIntFlag(t *testing.T) {
	tests := []struct {
		name     string
		flagSet  bool
		flagVal  string
		expected int
		called   bool
	}{
		{
			name:     "flag set to value",
			flagSet:  true,
			flagVal:  "42",
			expected: 42,
			called:   true,
		},
		{
			name:     "flag not set",
			flagSet:  false,
			expected: 0,
			called:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().Int("test-flag", 0, "")
			if tt.flagSet {
				cmd.Flags().Set("test-flag", tt.flagVal)
			}

			var actualValue int
			var setterCalled bool
			setter := func(val int) {
				actualValue = val
				setterCalled = true
			}

			applyIntFlag(cmd, "test-flag", setter)

			assert.Equal(t, tt.called, setterCalled)
			if tt.called {
				assert.Equal(t, tt.expected, actualValue)
			}
		})
	}
}

// TestApplyStringFlag tests string flag application helper.
//
//nolint:dupl // Similar structure to other flag tests is intentional
func TestApplyStringFlag(t *testing.T) {
	tests := []struct {
		name     string
		flagSet  bool
		flagVal  string
		expected string
		called   bool
	}{
		{
			name:     "flag set to value",
			flagSet:  true,
			flagVal:  "test-value",
			expected: "test-value",
			called:   true,
		},
		{
			name:     "flag not set",
			flagSet:  false,
			expected: "",
			called:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("test-flag", "", "")
			if tt.flagSet {
				cmd.Flags().Set("test-flag", tt.flagVal)
			}

			var actualValue string
			var setterCalled bool
			setter := func(val string) {
				actualValue = val
				setterCalled = true
			}

			applyStringFlag(cmd, "test-flag", setter)

			assert.Equal(t, tt.called, setterCalled)
			if tt.called {
				assert.Equal(t, tt.expected, actualValue)
			}
		})
	}
}

// TestSetupLogger_LogFileCreation tests actual log file creation.
func TestSetupLogger_LogFileCreation(t *testing.T) {
	// Save original state.
	originalHandle := logFileHandle
	defer func() {
		logFileHandle = originalHandle
		log.SetOutput(os.Stderr)
	}()

	// Create temporary directory for test log files.
	tmpDir := t.TempDir()

	logPath := fmt.Sprintf("%s/test.log", tmpDir)

	cfg := &schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
			File:  logPath,
		},
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{},
		},
	}

	SetupLogger(cfg)

	// Verify log file was created and handle is set.
	assert.NotNil(t, logFileHandle)
	assert.FileExists(t, logPath)

	// Write a log message.
	log.Info("test message")

	// Cleanup and close the file.
	cleanupLogFile()
	assert.Nil(t, logFileHandle)

	// Now read the file to verify content was written.
	content, err := os.ReadFile(logPath)
	assert.NoError(t, err)
	// The file should contain content (we wrote a log message).
	assert.NotEmpty(t, content)
}

// TestRootCmd_RunE tests the root command's RunE function.
func TestRootCmd_RunE(t *testing.T) {
	// This test verifies that the root command's RunE function
	// properly handles configuration and prints the ATMOS logo.

	// Use test fixtures.
	stacksPath := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	// The RunE function calls checkAtmosConfig() and ExecuteAtmosCmd().
	// We can't easily test the full execution without integration tests,
	// but we can verify it doesn't panic with valid config.

	// Note: This is a minimal test - the actual RunE behavior is tested
	// through integration tests in the tests/ directory.
	assert.NotNil(t, RootCmd.RunE, "RootCmd should have a RunE function")
}

// TestConvertToTermenvProfile tests terminal color profile conversion.
func TestConvertToTermenvProfile(t *testing.T) {
	tests := []struct {
		name     string
		input    terminal.ColorProfile
		expected termenv.Profile
	}{
		{
			name:     "ColorNone maps to Ascii",
			input:    terminal.ColorNone,
			expected: termenv.Ascii,
		},
		{
			name:     "Color16 maps to ANSI",
			input:    terminal.Color16,
			expected: termenv.ANSI,
		},
		{
			name:     "Color256 maps to ANSI256",
			input:    terminal.Color256,
			expected: termenv.ANSI256,
		},
		{
			name:     "ColorTrue maps to TrueColor",
			input:    terminal.ColorTrue,
			expected: termenv.TrueColor,
		},
		{
			name:     "Unknown profile defaults to Ascii",
			input:    terminal.ColorProfile(99),
			expected: termenv.Ascii,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToTermenvProfile(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFindExperimentalParent tests finding experimental parent commands.
// This function uses a two-pass approach:
// - First pass: Look for registry-based experimental commands (top-level like devcontainer, toolchain)
// - Second pass: Look for annotation-based experimental subcommands (terraform backend, etc.)
// This ensures that when running "devcontainer list", it returns "devcontainer" not "list".
func TestFindExperimentalParent(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *cobra.Command
		expected string
	}{
		{
			name: "nil command returns empty",
			setup: func() *cobra.Command {
				return nil
			},
			expected: "",
		},
		{
			name: "non-experimental command returns empty",
			setup: func() *cobra.Command {
				return &cobra.Command{Use: "regular"}
			},
			expected: "",
		},
		{
			name: "command with experimental annotation returns name",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{
					Use:         "experimental-cmd",
					Annotations: map[string]string{"experimental": "true"},
				}
				return cmd
			},
			expected: "experimental-cmd",
		},
		{
			name: "subcommand of experimental parent returns parent name",
			setup: func() *cobra.Command {
				parent := &cobra.Command{
					Use:         "parent",
					Annotations: map[string]string{"experimental": "true"},
				}
				child := &cobra.Command{Use: "child"}
				parent.AddCommand(child)
				return child
			},
			expected: "parent",
		},
		{
			name: "deeply nested subcommand returns nearest experimental parent",
			setup: func() *cobra.Command {
				grandparent := &cobra.Command{
					Use:         "grandparent",
					Annotations: map[string]string{"experimental": "true"},
				}
				parent := &cobra.Command{
					Use:         "parent",
					Annotations: map[string]string{"experimental": "true"},
				}
				child := &cobra.Command{Use: "child"}
				parent.AddCommand(child)
				grandparent.AddCommand(parent)
				return child
			},
			// The function finds the nearest experimental parent (first match going up the tree).
			expected: "parent",
		},
		{
			name: "mixed: non-experimental grandparent with experimental parent",
			setup: func() *cobra.Command {
				grandparent := &cobra.Command{Use: "grandparent"}
				parent := &cobra.Command{
					Use:         "parent",
					Annotations: map[string]string{"experimental": "true"},
				}
				child := &cobra.Command{Use: "child"}
				parent.AddCommand(child)
				grandparent.AddCommand(parent)
				return child
			},
			expected: "parent",
		},
		{
			name: "subcommand without annotation under experimental parent",
			setup: func() *cobra.Command {
				parent := &cobra.Command{
					Use:         "experimental-parent",
					Annotations: map[string]string{"experimental": "true"},
				}
				// Child has no annotation but parent does.
				child := &cobra.Command{Use: "subcommand"}
				parent.AddCommand(child)
				return child
			},
			expected: "experimental-parent",
		},
		{
			name: "command with non-true experimental annotation returns empty",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{
					Use:         "cmd",
					Annotations: map[string]string{"experimental": "false"},
				}
				return cmd
			},
			expected: "",
		},
		{
			name: "command with empty annotations map returns empty",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{
					Use:         "cmd",
					Annotations: map[string]string{},
				}
				return cmd
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setup()
			result := findExperimentalParent(cmd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseUseVersionFromArgsInternal tests --use-version flag parsing.
func TestParseUseVersionFromArgsInternal(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "no args returns empty",
			args:     []string{},
			expected: "",
		},
		{
			name:     "no use-version flag returns empty",
			args:     []string{"terraform", "plan"},
			expected: "",
		},
		{
			name:     "use-version=value format",
			args:     []string{"terraform", "--use-version=1.2.3", "plan"},
			expected: "1.2.3",
		},
		{
			name:     "use-version value format",
			args:     []string{"terraform", "--use-version", "1.2.3", "plan"},
			expected: "1.2.3",
		},
		{
			name:     "use-version at end without value returns empty",
			args:     []string{"terraform", "--use-version"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseUseVersionFromArgsInternal(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildFlagDescription tests flag description building.
func TestBuildFlagDescription(t *testing.T) {
	tests := []struct {
		name     string
		flag     *pflag.Flag
		contains string
	}{
		{
			name: "flag with no default",
			flag: &pflag.Flag{
				Name:     "test",
				Usage:    "Test flag",
				DefValue: "",
			},
			contains: "Test flag",
		},
		{
			name: "flag with string default",
			flag: &pflag.Flag{
				Name:     "format",
				Usage:    "Output format",
				DefValue: "json",
			},
			contains: "(default `json`)",
		},
		{
			name: "flag with false default",
			flag: &pflag.Flag{
				Name:     "verbose",
				Usage:    "Verbose output",
				DefValue: "false",
			},
			contains: "Verbose output",
		},
		{
			name: "flag with zero default",
			flag: &pflag.Flag{
				Name:     "count",
				Usage:    "Item count",
				DefValue: "0",
			},
			contains: "Item count",
		},
		{
			name: "flag with empty array default",
			flag: &pflag.Flag{
				Name:     "items",
				Usage:    "Item list",
				DefValue: "[]",
			},
			contains: "Item list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildFlagDescription(tt.flag)
			assert.Contains(t, result, tt.contains)
		})
	}
}

// TestRenderWrappedLines tests wrapped line rendering.
func TestRenderWrappedLines(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		indent   int
		expected string
	}{
		{
			name:     "empty lines",
			lines:    []string{},
			indent:   4,
			expected: "",
		},
		{
			name:     "single line",
			lines:    []string{"Hello world"},
			indent:   4,
			expected: "Hello world\n",
		},
		{
			name:     "multiple lines",
			lines:    []string{"Line one", "Line two"},
			indent:   2,
			expected: "Line one\n  Line two\n",
		},
	}

	descStyle := lipgloss.NewStyle()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderWrappedLines(&buf, tt.lines, tt.indent, &descStyle)
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

// TestSyncGlobalFlagsToViper tests syncing global flags to viper.
func TestSyncGlobalFlagsToViper(t *testing.T) {
	tests := []struct {
		name       string
		setupCmd   func(*cobra.Command)
		checkViper func(t *testing.T)
	}{
		{
			name: "profile flag synced",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().StringSlice("profile", []string{}, "")
				cmd.Flags().Set("profile", "dev")
			},
			checkViper: func(t *testing.T) {
				profiles := viper.GetStringSlice("profile")
				assert.Contains(t, profiles, "dev")
			},
		},
		{
			name: "identity flag synced",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().String("identity", "", "")
				cmd.Flags().Set("identity", "test-user")
			},
			checkViper: func(t *testing.T) {
				identity := viper.GetString("identity")
				assert.Equal(t, "test-user", identity)
			},
		},
		{
			name: "unchanged flags not synced",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().String("identity", "default", "")
				// Don't set/change the flag
			},
			checkViper: func(t *testing.T) {
				// Just verify it doesn't panic
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper for each test
			viper.Reset()

			cmd := &cobra.Command{Use: "test"}
			tt.setupCmd(cmd)

			syncGlobalFlagsToViper(cmd)
			tt.checkViper(t)
		})
	}
}

// TestConfigureEarlyColorProfile tests early color profile configuration.
func TestConfigureEarlyColorProfile(t *testing.T) {
	tests := []struct {
		name       string
		setupEnv   func(t *testing.T)
		setupFlags func(*cobra.Command)
	}{
		{
			name: "NO_COLOR env disables colors",
			setupEnv: func(t *testing.T) {
				t.Setenv("NO_COLOR", "1")
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("no-color", false, "")
				cmd.Flags().Bool("force-color", false, "")
			},
		},
		{
			name: "no-color flag disables colors",
			setupEnv: func(t *testing.T) {
				// NO_COLOR not set
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("no-color", false, "")
				cmd.Flags().Bool("force-color", false, "")
				cmd.Flags().Set("no-color", "true")
			},
		},
		{
			name: "force-color flag enables colors",
			setupEnv: func(t *testing.T) {
				// NO_COLOR not set
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("no-color", false, "")
				cmd.Flags().Bool("force-color", false, "")
				cmd.Flags().Set("force-color", "true")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv(t)

			cmd := &cobra.Command{Use: "test"}
			tt.setupFlags(cmd)

			// Should not panic.
			assert.NotPanics(t, func() {
				configureEarlyColorProfile(cmd)
			})
		})
	}
}

// TestFormatFlagNameParts tests flag name formatting for different flag types.
func TestFormatFlagNameParts(t *testing.T) {
	tests := []struct {
		name              string
		flagName          string
		shorthand         string
		flagType          string
		expectedName      string
		expectedType      string
		expectedTypeEmpty bool
	}{
		{
			name:              "bool flag with shorthand returns no type",
			flagName:          "verbose",
			shorthand:         "v",
			flagType:          "bool",
			expectedName:      "-v, --verbose",
			expectedTypeEmpty: true,
		},
		{
			name:              "bool flag without shorthand returns no type",
			flagName:          "help",
			shorthand:         "",
			flagType:          "bool",
			expectedName:      "    --help",
			expectedTypeEmpty: true,
		},
		{
			name:         "string flag with shorthand",
			flagName:     "output",
			shorthand:    "o",
			flagType:     "string",
			expectedName: "-o, --output",
			expectedType: "string",
		},
		{
			name:         "int flag without shorthand",
			flagName:     "count",
			shorthand:    "",
			flagType:     "int",
			expectedName: "    --count",
			expectedType: "int",
		},
		{
			name:         "stringSlice flag becomes strings",
			flagName:     "profiles",
			shorthand:    "p",
			flagType:     "stringSlice",
			expectedName: "-p, --profiles",
			expectedType: "strings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}

			// Add the flag based on type.
			switch tt.flagType {
			case "bool":
				if tt.shorthand != "" {
					cmd.Flags().BoolP(tt.flagName, tt.shorthand, false, "test")
				} else {
					cmd.Flags().Bool(tt.flagName, false, "test")
				}
			case "string":
				if tt.shorthand != "" {
					cmd.Flags().StringP(tt.flagName, tt.shorthand, "", "test")
				} else {
					cmd.Flags().String(tt.flagName, "", "test")
				}
			case "int":
				if tt.shorthand != "" {
					cmd.Flags().IntP(tt.flagName, tt.shorthand, 0, "test")
				} else {
					cmd.Flags().Int(tt.flagName, 0, "test")
				}
			case "stringSlice":
				if tt.shorthand != "" {
					cmd.Flags().StringSliceP(tt.flagName, tt.shorthand, nil, "test")
				} else {
					cmd.Flags().StringSlice(tt.flagName, nil, "test")
				}
			}

			flag := cmd.Flags().Lookup(tt.flagName)
			assert.NotNil(t, flag)

			name, flagType := formatFlagNameParts(flag)
			assert.Equal(t, tt.expectedName, name)
			if tt.expectedTypeEmpty {
				assert.Empty(t, flagType)
			} else {
				assert.Equal(t, tt.expectedType, flagType)
			}
		})
	}
}

// TestGetTerminalWidth tests terminal width detection.
func TestGetTerminalWidth(t *testing.T) {
	// getTerminalWidth should return a positive integer.
	// In test environment (non-TTY), it returns the default width.
	width := getTerminalWidth()
	assert.Greater(t, width, 0)
	assert.LessOrEqual(t, width, 120) // Max width is 120
}

// TestCalculateMaxFlagWidth tests flag width calculation.
func TestCalculateMaxFlagWidth(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().BoolP("verbose", "v", false, "verbose output")
	cmd.Flags().String("long-flag-name", "", "a flag with a long name")
	cmd.Flags().Bool("hidden", false, "hidden flag")
	cmd.Flags().Lookup("hidden").Hidden = true

	maxWidth := calculateMaxFlagWidth(cmd.Flags())
	// Should not include hidden flag.
	// Long flag: "    --long-flag-name string" is longest.
	assert.Greater(t, maxWidth, 0)
}

// TestRenderFlags tests flag rendering.
func TestRenderFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().BoolP("verbose", "v", false, "verbose output")
	cmd.Flags().String("format", "json", "output format")

	var buf bytes.Buffer
	flagStyle := lipgloss.NewStyle()
	argTypeStyle := lipgloss.NewStyle()
	descStyle := lipgloss.NewStyle()

	atmosConfig := &schema.AtmosConfiguration{}

	// Should not panic.
	assert.NotPanics(t, func() {
		renderFlags(&buf, cmd.Flags(), flagStyle, argTypeStyle, descStyle, 80, atmosConfig)
	})

	output := buf.String()
	assert.Contains(t, output, "verbose")
	assert.Contains(t, output, "format")
}

// TestRenderFlagsNilFlagSet tests renderFlags with nil flag set.
func TestRenderFlagsNilFlagSet(t *testing.T) {
	var buf bytes.Buffer
	flagStyle := lipgloss.NewStyle()
	argTypeStyle := lipgloss.NewStyle()
	descStyle := lipgloss.NewStyle()

	atmosConfig := &schema.AtmosConfiguration{}

	// Should not panic with nil flags.
	assert.NotPanics(t, func() {
		renderFlags(&buf, nil, flagStyle, argTypeStyle, descStyle, 80, atmosConfig)
	})

	// Output should be empty.
	assert.Empty(t, buf.String())
}

// TestExperimentalModeHandling tests the experimental command mode switch cases.
// This tests the different behaviors based on settings.experimental configuration.
func TestExperimentalModeHandling(t *testing.T) {
	tests := []struct {
		name             string
		experimentalMode string
		expectExit       bool
		expectedExitCode int
	}{
		{
			name:             "silence mode - no output or exit",
			experimentalMode: "silence",
			expectExit:       false,
		},
		{
			name:             "disable mode - command disabled with exit",
			experimentalMode: "disable",
			expectExit:       true,
			expectedExitCode: 1,
		},
		{
			name:             "warn mode - warning shown but no exit",
			experimentalMode: "warn",
			expectExit:       false,
		},
		{
			name:             "error mode - warning and exit",
			experimentalMode: "error",
			expectExit:       true,
			expectedExitCode: 1,
		},
		{
			name:             "empty mode defaults to warn - no exit",
			experimentalMode: "",
			expectExit:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use NewTestKit to isolate RootCmd state.
			_ = NewTestKit(t)

			// Save and restore os.Exit.
			originalOsExit := errUtils.OsExit
			defer func() {
				errUtils.OsExit = originalOsExit
			}()

			// Track if exit was called and with what code.
			var exitCalled bool
			var exitCode int
			errUtils.OsExit = func(code int) {
				exitCalled = true
				exitCode = code
				// Don't actually exit in tests - panic to stop execution.
				panic(fmt.Sprintf("os.Exit(%d) called", code))
			}

			// Create a test command marked as experimental.
			testCmd := &cobra.Command{
				Use:   "test-experimental",
				Short: "Test experimental command",
				Annotations: map[string]string{
					"experimental": "true",
				},
				Run: func(cmd *cobra.Command, args []string) {
					// Command executed successfully.
				},
			}

			// Add the test command to RootCmd.
			RootCmd.AddCommand(testCmd)
			defer func() {
				RootCmd.RemoveCommand(testCmd)
			}()

			// Set up environment for the test.
			stacksPath := "../tests/fixtures/scenarios/complete"
			t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
			t.Setenv("ATMOS_BASE_PATH", stacksPath)

			// Set the experimental mode via environment variable.
			// This will be read during config initialization.
			// The env var is ATMOS_EXPERIMENTAL (not ATMOS_SETTINGS_EXPERIMENTAL).
			if tt.experimentalMode != "" {
				t.Setenv("ATMOS_EXPERIMENTAL", tt.experimentalMode)
			}

			// Set args to run our test experimental command.
			RootCmd.SetArgs([]string{"test-experimental"})

			// Execute and check for panic (which we use to simulate exit).
			if tt.expectExit {
				assert.Panics(t, func() {
					_ = Execute()
				}, "Expected os.Exit to be called")
				assert.True(t, exitCalled, "Expected exit to be called")
				assert.Equal(t, tt.expectedExitCode, exitCode, "Expected exit code %d, got %d", tt.expectedExitCode, exitCode)
			} else {
				assert.NotPanics(t, func() {
					_ = Execute()
				}, "Expected no os.Exit call")
				assert.False(t, exitCalled, "Expected exit not to be called")
			}
		})
	}
}
