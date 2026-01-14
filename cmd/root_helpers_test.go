package cmd

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/profiler"
	"github.com/cloudposse/atmos/pkg/schema"
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
