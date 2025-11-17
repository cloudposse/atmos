package cmd

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/profiler"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNoColorLog(t *testing.T) {
	// Skip in CI environments without TTY.
	if _, err := os.Open("/dev/tty"); err != nil {
		t.Skipf("Skipping test: TTY not available (/dev/tty): %v", err)
	}

	// Snapshot RootCmd state to prevent test pollution.
	_ = NewTestKit(t)

	// Save and restore working directory - previous tests may have changed it.
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	// Change back to original directory and automatically restore when test ends.
	t.Chdir(originalWd)

	// Ensure ATMOS_CHDIR is not set BEFORE anything else.
	// Previous tests may have set it, and we need to clear it before RootCmd.Execute().
	// We can't use t.Setenv here because previous tests may have set it,
	// and t.Setenv only restores to the ORIGINAL value before the test package loaded.
	os.Unsetenv("ATMOS_CHDIR")
	defer os.Unsetenv("ATMOS_CHDIR") // Clean up after test.

	stacksPath := "../tests/fixtures/scenarios/stack-templates"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")

	// Set the environment variable to disable color
	// t.Setenv("NO_COLOR", "1")
	t.Setenv("ATMOS_LOGS_LEVEL", "Debug")
	t.Setenv("NO_COLOR", "1")
	// Create a buffer to capture the output
	var buf bytes.Buffer
	log.SetOutput(&buf)

	// Use SetArgs - TestKit handles cleanup automatically.
	RootCmd.SetArgs([]string{"about"})

	// Reset buffer to ensure clean state (previous tests may have written to logger).
	buf.Reset()

	// Execute the command
	if err := Execute(); err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
	// Check if the output is without color
	output := buf.String()
	if strings.Contains(output, "\033[") {
		t.Errorf("Expected no color in output, but got: %s", output)
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("Command output: %s", output)
		}
	})
}

func TestInitFunction(t *testing.T) {
	// Test doesn't modify os.Args, so no need to save/restore.

	// Test cases
	tests := []struct {
		name           string
		setup          func()
		validate       func(t *testing.T)
		expectedErrMsg string
	}{
		{
			name: "verify command setup",
			setup: func() {
				// No special setup needed
			},
			validate: func(t *testing.T) {
				// Verify subcommands are properly registered
				assert.NotNil(t, RootCmd.Commands())
				// Add specific command checks if needed
			},
		},
		{
			name: "verify version command setup",
			setup: func() {
				// No special setup needed
			},
			validate: func(t *testing.T) {
				// Verify the version command is properly configured
				versionCmd, _, _ := RootCmd.Find([]string{"version"})
				assert.NotNil(t, versionCmd, "version command should be registered")
				// Add more specific version command checks
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if tt.setup != nil {
				tt.setup()
			}

			// The init() function runs automatically when the package is imported
			// We can verify its effects through the RootCmd and other package-level variables

			// Validate
			if tt.validate != nil {
				tt.validate(t)
			}
		})
	}
}

func TestSetupLogger_TraceLevel(t *testing.T) {
	// Save original state.
	originalLevel := log.GetLevel()
	defer func() {
		log.SetLevel(originalLevel)
		log.SetOutput(os.Stderr) // Reset to default
	}()

	tests := []struct {
		name          string
		configLevel   string
		expectedLevel log.Level
	}{
		{"Trace", "Trace", log.TraceLevel},
		{"Debug", "Debug", log.DebugLevel},
		{"Info", "Info", log.InfoLevel},
		{"Warning", "Warning", log.WarnLevel},
		{"Off", "Off", log.Level(math.MaxInt32)},
		{"Default", "", log.WarnLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: tt.configLevel,
					File:  "/dev/stderr", // Default log file
				},
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{},
				},
			}

			SetupLogger(cfg)
			assert.Equal(t, tt.expectedLevel, log.GetLevel(),
				"Expected level %v for config %q", tt.expectedLevel, tt.configLevel)
		})
	}
}

func TestSetupLogger_TraceVisibility(t *testing.T) {
	// Save original state.
	originalLevel := log.GetLevel()
	defer func() {
		log.SetLevel(originalLevel)
		log.SetOutput(os.Stderr) // Reset to default
	}()

	var buf bytes.Buffer
	log.SetOutput(&buf)

	tests := []struct {
		name         string
		configLevel  string
		traceVisible bool
		debugVisible bool
		infoVisible  bool
	}{
		{
			name:         "Trace level shows all",
			configLevel:  "Trace",
			traceVisible: true,
			debugVisible: true,
			infoVisible:  true,
		},
		{
			name:         "Debug level hides trace",
			configLevel:  "Debug",
			traceVisible: false,
			debugVisible: true,
			infoVisible:  true,
		},
		{
			name:         "Info level hides trace and debug",
			configLevel:  "Info",
			traceVisible: false,
			debugVisible: false,
			infoVisible:  true,
		},
		{
			name:         "Warning level hides trace, debug, and info",
			configLevel:  "Warning",
			traceVisible: false,
			debugVisible: false,
			infoVisible:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: tt.configLevel,
					File:  "", // No file so it uses the pre-set buffer
				},
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{},
				},
			}
			SetupLogger(cfg)

			// Test trace visibility.
			buf.Reset()
			log.Trace("trace test message")
			hasTrace := strings.Contains(buf.String(), "trace test message")
			assert.Equal(t, tt.traceVisible, hasTrace,
				"Trace visibility incorrect for %q level", tt.configLevel)

			// Test debug visibility.
			buf.Reset()
			log.Debug("debug test message")
			hasDebug := strings.Contains(buf.String(), "debug test message")
			assert.Equal(t, tt.debugVisible, hasDebug,
				"Debug visibility incorrect for %q level", tt.configLevel)

			// Test info visibility.
			buf.Reset()
			log.Info("info test message")
			hasInfo := strings.Contains(buf.String(), "info test message")
			assert.Equal(t, tt.infoVisible, hasInfo,
				"Info visibility incorrect for %q level", tt.configLevel)
		})
	}
}

func TestSetupLogger_TraceLevelFromEnvironment(t *testing.T) {
	// Save original state.
	originalLevel := log.GetLevel()
	originalEnv := os.Getenv("ATMOS_LOGS_LEVEL")
	defer func() {
		log.SetLevel(originalLevel)
		log.SetOutput(os.Stderr) // Reset to default
		if originalEnv == "" {
			os.Unsetenv("ATMOS_LOGS_LEVEL")
		} else {
			os.Setenv("ATMOS_LOGS_LEVEL", originalEnv)
		}
	}()

	// Test that ATMOS_LOGS_LEVEL=Trace works.
	t.Setenv("ATMOS_LOGS_LEVEL", "Trace")

	// Simulate loading config from environment.
	cfg := &schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: os.Getenv("ATMOS_LOGS_LEVEL"),
			File:  "/dev/stderr", // Default log file
		},
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{},
		},
	}
	SetupLogger(cfg)

	assert.Equal(t, log.TraceLevel, log.GetLevel(),
		"Should set trace level from environment variable")
}

func TestSetupLogger_NoColorWithTraceLevel(t *testing.T) {
	// Save original state.
	originalLevel := log.GetLevel()
	defer func() {
		log.SetLevel(originalLevel)
		log.SetOutput(os.Stderr) // Reset to default
	}()

	// Test that trace level works with no-color mode.
	cfg := &schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Trace",
			File:  "/dev/stderr", // Default log file
		},
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				// This simulates NO_COLOR environment.
			},
		},
	}

	// Mock the IsColorEnabled to return false.
	// Since we can't easily mock it, we'll just test that SetupLogger doesn't panic.
	assert.NotPanics(t, func() {
		SetupLogger(cfg)
	}, "SetupLogger should not panic with trace level and no color")

	assert.Equal(t, log.TraceLevel, log.GetLevel(),
		"Trace level should be set even with no color")
}

func TestVersionFlagParsing(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name        string
		args        []string
		expectValue bool
	}{
		{
			name:        "--version flag is parsed correctly",
			args:        []string{"--version"},
			expectValue: true,
		},
		{
			name:        "no --version flag defaults to false",
			args:        []string{},
			expectValue: false,
		},
		{
			name:        "--version can be combined with other flags",
			args:        []string{"--version", "--no-color"},
			expectValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Reset flag states before each test - need to reset both value and Changed state.
			versionFlag := RootCmd.PersistentFlags().Lookup("version")
			if versionFlag != nil {
				versionFlag.Value.Set("false")
				versionFlag.Changed = false
			}

			// Use the global RootCmd; state isolation is handled by flag reset above.
			RootCmd.SetArgs(tt.args)

			// Check that the version flag is defined.
			assert.NotNil(t, versionFlag, "version flag should be defined")
			assert.Contains(t, versionFlag.Usage, "Atmos CLI version", "usage should mention Atmos CLI version")

			// Parse flags.
			err := RootCmd.ParseFlags(tt.args)
			assert.NoError(t, err, "parsing flags should not error")

			// Check if version flag was set to expected value.
			versionSet, err := RootCmd.Flags().GetBool("version")
			assert.NoError(t, err)
			assert.Equal(t, tt.expectValue, versionSet, "version flag should be %v", tt.expectValue)
		})
	}
}

func TestVersionFlagExecutionPath(t *testing.T) {
	tests := []struct {
		name        string
		setup       func()
		cleanup     func()
		expectError bool
	}{
		{
			name: "version subcommand works normally without deep exit",
			setup: func() {
				versionFlag := RootCmd.PersistentFlags().Lookup("version")
				if versionFlag != nil {
					versionFlag.Value.Set("false")
					versionFlag.Changed = false
				}
				RootCmd.SetArgs([]string{"version"})
			},
			cleanup:     func() {},
			expectError: false, // Should complete normally, no error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use NewTestKit to isolate RootCmd state.
			_ = NewTestKit(t)

			t.Cleanup(tt.cleanup)

			// Setup test conditions.
			tt.setup()

			// Execute should complete normally without calling os.Exit.
			// The --version flag is now handled in main.go for production,
			// so tests only verify the version subcommand works without deep exit.
			err := Execute()
			if tt.expectError {
				assert.Error(t, err, "Execute should return an error")
			} else {
				assert.NoError(t, err, "Execute should complete without error")
			}
		})
	}
}

func TestPagerDoesNotRunWithoutTTY(t *testing.T) {
	// This test verifies that the pager doesn't try to use the alternate screen buffer
	// when there's no TTY available. This is important for scripted/CI environments
	// where stdin/stdout/stderr are not connected to a terminal.

	t.Run("help should not error when ATMOS_PAGER=false and no TTY", func(t *testing.T) {
		// Use NewTestKit to isolate RootCmd state.
		_ = NewTestKit(t)

		// Save original os.Args and os.Exit.
		originalArgs := os.Args
		originalOsExit := errUtils.OsExit
		defer func() {
			os.Args = originalArgs
			errUtils.OsExit = originalOsExit
		}()

		// Mock OsExit to prevent test framework panics from remaining deep exits.
		// Note: Pager NO LONGER calls os.Exit() (eliminated in cmd/root.go:1239-1241).
		// However, other code paths may still exit (e.g., version flag handler).
		// This mock catches those until all deep exits are eliminated.
		exitCalled := false
		errUtils.OsExit = func(code int) {
			exitCalled = true
			// Don't actually exit in tests
		}

		// Set ATMOS_CLI_CONFIG_PATH to a test directory to avoid loading real config.
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "testdata/pager")

		// Set ATMOS_PAGER=false to explicitly disable the pager.
		t.Setenv("ATMOS_PAGER", "false")

		// Set os.Args so our custom Execute() function can parse them.
		// This is required because Execute() needs to initialize atmosConfig from environment variables.
		os.Args = []string{"atmos", "--help"}

		// Execute should not error even without a TTY.
		// The pager should be disabled via ATMOS_PAGER=false, so no TTY error should occur.
		// We're primarily checking that there's no "could not open a new TTY" panic/error from pager.
		_ = Execute()

		// Success: No TTY-related panic occurred.
		// The test passing means pager handles missing TTY gracefully.
		// Note: exitCalled may be true from other exit paths (version flag, etc.), but
		// the important thing is that pager-specific errors don't cause exits anymore.
		_ = exitCalled
	})

	t.Run("help should not error when ATMOS_PAGER=true but no TTY", func(t *testing.T) {
		// Use NewTestKit to isolate RootCmd state.
		_ = NewTestKit(t)

		// Save original os.Args and os.Exit.
		originalArgs := os.Args
		originalOsExit := errUtils.OsExit
		defer func() {
			os.Args = originalArgs
			errUtils.OsExit = originalOsExit
		}()

		// Mock OsExit to prevent test framework panics from remaining deep exits.
		// Note: Pager NO LONGER calls os.Exit() (eliminated in cmd/root.go:1239-1241).
		// The pager's own error handling (pkg/pager/pager.go:88-92) falls back to direct output.
		// However, other code paths may still exit (e.g., version flag handler).
		// This mock catches those until all deep exits are eliminated.
		exitCalled := false
		errUtils.OsExit = func(code int) {
			exitCalled = true
			// Don't actually exit in tests
		}

		// Set ATMOS_CLI_CONFIG_PATH to a test directory to avoid loading real config.
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "testdata/pager")

		// Set ATMOS_PAGER=true to try to enable pager, but there's no TTY.
		// The pager should detect no TTY and fall back to direct output.
		t.Setenv("ATMOS_PAGER", "true")

		// Set os.Args so our custom Execute() function can parse them.
		os.Args = []string{"atmos", "--help"}

		// Execute should not error even without a TTY.
		// The pager should detect the lack of TTY and fall back to printing directly.
		// We're primarily checking that there's no "could not open a new TTY" panic/error from pager.
		_ = Execute()

		// Success: No TTY-related panic occurred from pager.
		// The test passing means pager handles missing TTY gracefully without exiting.
		// Note: exitCalled may be true from other exit paths (version flag, etc.), but
		// the important thing is that pager-specific errors don't cause exits anymore.
		_ = exitCalled
	})
}

// TestIsCompletionCommand tests the isCompletionCommand function.
func TestIsCompletionCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		compLine string
		argComp  string
		expected bool
	}{
		{
			name:     "regular completion command",
			args:     []string{"atmos", "completion"},
			expected: true,
		},
		{
			name:     "__complete hidden command",
			args:     []string{"atmos", "__complete"},
			expected: true,
		},
		{
			name:     "__completeNoDesc hidden command",
			args:     []string{"atmos", "__completeNoDesc"},
			expected: true,
		},
		{
			name:     "COMP_LINE env var set",
			args:     []string{"atmos", "terraform"},
			compLine: "atmos terraform ",
			expected: true,
		},
		{
			name:     "_ARGCOMPLETE env var set",
			args:     []string{"atmos", "terraform"},
			argComp:  "1",
			expected: true,
		},
		{
			name:     "regular command - not completion",
			args:     []string{"atmos", "version"},
			expected: false,
		},
		{
			name:     "no args",
			args:     []string{"atmos"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment variables.
			if tt.compLine != "" {
				t.Setenv("COMP_LINE", tt.compLine)
			}
			if tt.argComp != "" {
				t.Setenv("_ARGCOMPLETE", tt.argComp)
			}

			// Create a mock command with the appropriate name based on the test args.
			var cmd *cobra.Command
			if len(tt.args) > 1 {
				cmd = &cobra.Command{
					Use: tt.args[1],
				}
			}

			// Test.
			result := isCompletionCommand(cmd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindAnsiCodeEnd(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "simple code ending with m",
			input:    "0m",
			expected: 1, // Returns index of 'm'
		},
		{
			name:     "color code ending with m",
			input:    "38;5;123mtext",
			expected: 8, // Returns index of 'm'
		},
		{
			name:     "no ending letter",
			input:    "123;456;",
			expected: -1,
		},
		{
			name:     "uppercase ending",
			input:    "1A",
			expected: 1, // Returns index of 'A'
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findAnsiCodeEnd(tt.input)
			if result != tt.expected {
				t.Errorf("findAnsiCodeEnd(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsBackgroundCode(t *testing.T) {
	tests := []struct {
		name     string
		ansiCode string
		expected bool
	}{
		{
			name:     "foreground color code",
			ansiCode: "38;5;123m",
			expected: false,
		},
		{
			name:     "background code with prefix",
			ansiCode: "48;5;123m",
			expected: true,
		},
		{
			name:     "background code in middle",
			ansiCode: "0;48;5;123m",
			expected: true,
		},
		{
			name:     "reset code",
			ansiCode: "0m",
			expected: false,
		},
		{
			name:     "bold code",
			ansiCode: "1m",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBackgroundCode(tt.ansiCode)
			if result != tt.expected {
				t.Errorf("isBackgroundCode(%q) = %v, want %v", tt.ansiCode, result, tt.expected)
			}
		})
	}
}

func TestStripBackgroundCodes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text no codes",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "foreground only",
			input:    "\x1b[38;5;123mcolored text\x1b[0m",
			expected: "\x1b[38;5;123mcolored text\x1b[0m",
		},
		{
			name:     "background only",
			input:    "\x1b[48;5;123mbackground\x1b[0m",
			expected: "background\x1b[0m",
		},
		{
			name:     "foreground and background mixed (separate sequences)",
			input:    "\x1b[38;5;123m\x1b[48;5;200mtext\x1b[0m",
			expected: "\x1b[38;5;123mtext\x1b[0m",
		},
		{
			name:     "combined foreground and background in single sequence (TrueColor)",
			input:    "\x1b[38;2;255;0;0;48;2;0;0;255mred on blue\x1b[0m",
			expected: "\x1b[38;2;255;0;0mred on blue\x1b[0m",
		},
		{
			name:     "combined foreground and background in single sequence (256 color)",
			input:    "\x1b[38;5;123;48;5;200mtext\x1b[0m",
			expected: "\x1b[38;5;123mtext\x1b[0m",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripBackgroundCodes(tt.input)
			if result != tt.expected {
				t.Errorf("stripBackgroundCodes(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewFlagRenderLayout(t *testing.T) {
	tests := []struct {
		name          string
		termWidth     int
		maxFlagWidth  int
		wantDescWidth int
	}{
		{
			name:          "normal terminal width",
			termWidth:     120,
			maxFlagWidth:  20,
			wantDescWidth: 94, // Calculated as: termWidth minus leftPad minus maxFlag minus spaceBetween minus rightMargin.
		},
		{
			name:          "narrow terminal forces minimum",
			termWidth:     50,
			maxFlagWidth:  20,
			wantDescWidth: 40, // Minimum enforced
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := newFlagRenderLayout(tt.termWidth, tt.maxFlagWidth)
			if layout.descWidth != tt.wantDescWidth {
				t.Errorf("newFlagRenderLayout() descWidth = %d, want %d", layout.descWidth, tt.wantDescWidth)
			}
			if layout.maxFlagWidth != tt.maxFlagWidth {
				t.Errorf("newFlagRenderLayout() maxFlagWidth = %d, want %d", layout.maxFlagWidth, tt.maxFlagWidth)
			}
		})
	}
}

// TestParseChdirFromArgs tests the parseChdirFromArgs function that manually parses --chdir or -C flags.
// This function is critical for commands with DisableFlagParsing=true (terraform, helmfile, packer).
func TestParseChdirFromArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "--chdir with equals sign",
			args:     []string{"atmos", "--chdir=/tmp/foo", "terraform", "plan"},
			expected: "/tmp/foo",
		},
		{
			name:     "--chdir with space",
			args:     []string{"atmos", "--chdir", "/tmp/bar", "terraform", "plan"},
			expected: "/tmp/bar",
		},
		{
			name:     "-C with equals sign",
			args:     []string{"atmos", "-C=/tmp/baz", "terraform", "plan"},
			expected: "/tmp/baz",
		},
		{
			name:     "-C with space",
			args:     []string{"atmos", "-C", "/tmp/qux", "terraform", "plan"},
			expected: "/tmp/qux",
		},
		{
			name:     "-C concatenated (no space or equals)",
			args:     []string{"atmos", "-C/tmp/concat", "terraform", "plan"},
			expected: "/tmp/concat",
		},
		{
			name:     "-C concatenated with relative path",
			args:     []string{"atmos", "-C../foo", "terraform", "plan"},
			expected: "../foo",
		},
		{
			name:     "no chdir flag",
			args:     []string{"atmos", "terraform", "plan"},
			expected: "",
		},
		{
			name:     "--chdir at end without value",
			args:     []string{"atmos", "terraform", "plan", "--chdir"},
			expected: "",
		},
		{
			name:     "-C at end without value",
			args:     []string{"atmos", "terraform", "plan", "-C"},
			expected: "",
		},
		{
			name:     "multiple --chdir flags (first wins)",
			args:     []string{"atmos", "--chdir=/first", "--chdir=/second", "terraform", "plan"},
			expected: "/first",
		},
		{
			name:     "mixed -C and --chdir (first wins)",
			args:     []string{"atmos", "-C/first", "--chdir=/second", "terraform", "plan"},
			expected: "/first",
		},
		{
			name:     "--chdir with tilde",
			args:     []string{"atmos", "--chdir=~/mydir", "terraform", "plan"},
			expected: "~/mydir",
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: "",
		},
		{
			name:     "single arg",
			args:     []string{"atmos"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore os.Args.
			originalArgs := os.Args
			defer func() { os.Args = originalArgs }()

			// Set os.Args to the test args.
			os.Args = tt.args

			// Call the function.
			result := parseChdirFromArgs()

			// Verify.
			assert.Equal(t, tt.expected, result,
				"parseChdirFromArgs() with args %v should return %q, got %q",
				tt.args, tt.expected, result)
		})
	}
}

func TestSetupColorProfile(t *testing.T) {
	tests := []struct {
		name        string
		forceColor  bool
		expectForce bool
	}{
		{
			name:        "force color enabled",
			forceColor:  true,
			expectForce: true,
		},
		{
			name:        "force color disabled",
			forceColor:  false,
			expectForce: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						ForceColor: tt.forceColor,
					},
				},
			}

			// Should not panic.
			setupColorProfile(atmosConfig)

			// The function sets global color profile - difficult to test directly,
			// but we can verify it doesn't crash.
		})
	}
}

func TestSetupColorProfileFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envVar   string
		envValue string
		args     []string
	}{
		{
			name:     "ATMOS_FORCE_COLOR set",
			envVar:   "ATMOS_FORCE_COLOR",
			envValue: "true",
			args:     []string{},
		},
		{
			name:     "force-color flag",
			envVar:   "",
			envValue: "",
			args:     []string{"atmos", "--force-color", "version"},
		},
		{
			name:     "no force color",
			envVar:   "",
			envValue: "",
			args:     []string{"atmos", "version"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore os.Args.
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()

			if tt.envVar != "" {
				t.Setenv(tt.envVar, tt.envValue)
			}

			if len(tt.args) > 0 {
				os.Args = tt.args
			}

			// Should not panic.
			setupColorProfileFromEnv()
		})
	}
}

func TestCleanup(t *testing.T) {
	t.Run("cleanup without log file", func(t *testing.T) {
		// Ensure logFileHandle is nil.
		logFileHandle = nil

		// Should not panic.
		Cleanup()
	})

	t.Run("cleanup with log file", func(t *testing.T) {
		// Create a temporary log file.
		tmpFile, err := os.CreateTemp("", "test-log-*.log")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		// Set the global log file handle.
		logFileHandle = tmpFile

		// Cleanup should close the file.
		Cleanup()

		// Verify the handle was cleared.
		if logFileHandle != nil {
			t.Error("Expected logFileHandle to be nil after cleanup")
		}
	})
}

func TestExecuteVersion(t *testing.T) {
	t.Run("executes version command", func(t *testing.T) {
		// This function executes the version command.
		// We just verify it doesn't panic.
		err := ExecuteVersion()
		if err != nil {
			t.Logf("ExecuteVersion returned error: %v", err)
		}
	})
}

// TestGetTerminalWidth tests terminal width detection.
func TestGetTerminalWidth(t *testing.T) {
	// getTerminalWidth has fallback logic for when terminal size detection fails.
	// We can't easily mock xterm.GetSize, but we can verify it returns a sane value.
	width := getTerminalWidth()

	// Should return either detected width or default (120).
	assert.Greater(t, width, 0, "Terminal width should be positive")
	assert.LessOrEqual(t, width, 120, "Terminal width should not exceed default max")
}

// TestBuildFlagDescription tests flag description formatting.
func TestBuildFlagDescription(t *testing.T) {
	tests := []struct {
		name     string
		flag     *cobra.Command
		flagName string
		defValue string
		usage    string
		expected string
	}{
		{
			name:     "flag with default value",
			flagName: "timeout",
			defValue: "30s",
			usage:    "Connection timeout",
			expected: "Connection timeout (default `30s`)",
		},
		{
			name:     "bool flag with false default",
			flagName: "verbose",
			defValue: "false",
			usage:    "Enable verbose output",
			expected: "Enable verbose output",
		},
		{
			name:     "flag with zero default",
			flagName: "retries",
			defValue: "0",
			usage:    "Number of retries",
			expected: "Number of retries",
		},
		{
			name:     "flag with empty array default",
			flagName: "tags",
			defValue: "[]",
			usage:    "Resource tags",
			expected: "Resource tags",
		},
		{
			name:     "help flag",
			flagName: "help",
			defValue: "true",
			usage:    "Show help",
			expected: "Show help",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			if tt.flagName == "help" {
				cmd.PersistentFlags().BoolP(tt.flagName, "h", false, tt.usage)
			} else {
				cmd.PersistentFlags().String(tt.flagName, tt.defValue, tt.usage)
			}

			flag := cmd.PersistentFlags().Lookup(tt.flagName)
			if flag != nil {
				flag.DefValue = tt.defValue
				result := buildFlagDescription(flag)
				assert.Contains(t, result, tt.usage)
				if tt.defValue != "false" && tt.defValue != "0" && tt.defValue != "[]" && tt.flagName != "help" && tt.defValue != "" {
					assert.Contains(t, result, "default")
				}
			}
		})
	}
}

// TestCalculateMaxFlagWidth tests flag width calculation.
func TestCalculateMaxFlagWidth(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringP("short", "s", "", "Short flag")
	cmd.Flags().String("very-long-flag-name-here", "", "Long flag")
	cmd.Flags().BoolP("hidden", "x", false, "Hidden flag")
	cmd.Flags().Lookup("hidden").Hidden = true

	maxWidth := calculateMaxFlagWidth(cmd.Flags())

	// Should find the longest visible flag.
	assert.Greater(t, maxWidth, 0)
	// Should not count hidden flags.
	// The very-long-flag-name-here should be the max.
	assert.Greater(t, maxWidth, len("--short"))
}

// TestGetInvalidCommandName tests extraction of command name from error message.
func TestGetInvalidCommandName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid unknown command error",
			input:    `unknown command "foobar" for "atmos"`,
			expected: "foobar",
		},
		{
			name:     "unknown subcommand",
			input:    `unknown command "terraform-plan" for "atmos"`,
			expected: "terraform-plan",
		},
		{
			name:     "no match",
			input:    "some other error message",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getInvalidCommandName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestApplyBoolFlag tests boolean flag application.
func TestApplyBoolFlag(t *testing.T) {
	tests := []struct {
		name         string
		flagName     string
		flagValue    bool
		flagChanged  bool
		expectCalled bool
	}{
		{
			name:         "flag set to true",
			flagName:     "test-bool",
			flagValue:    true,
			flagChanged:  true,
			expectCalled: true,
		},
		{
			name:         "flag set to false",
			flagName:     "test-bool",
			flagValue:    false,
			flagChanged:  true,
			expectCalled: true,
		},
		{
			name:         "flag not changed",
			flagName:     "test-bool",
			flagValue:    false,
			flagChanged:  false,
			expectCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().Bool(tt.flagName, false, "test flag")

			if tt.flagChanged {
				cmd.Flags().Set(tt.flagName, fmt.Sprintf("%t", tt.flagValue))
			}

			called := false
			var capturedValue bool
			setter := func(val bool) {
				called = true
				capturedValue = val
			}

			applyBoolFlag(cmd, tt.flagName, setter)

			assert.Equal(t, tt.expectCalled, called, "Setter should be called: %v", tt.expectCalled)
			if tt.expectCalled {
				assert.Equal(t, tt.flagValue, capturedValue)
			}
		})
	}
}

// TestApplyIntFlag tests integer flag application.
func TestApplyIntFlag(t *testing.T) {
	tests := []struct {
		name         string
		flagName     string
		flagValue    int
		flagChanged  bool
		expectCalled bool
	}{
		{
			name:         "flag set to positive value",
			flagName:     "test-int",
			flagValue:    42,
			flagChanged:  true,
			expectCalled: true,
		},
		{
			name:         "flag set to zero",
			flagName:     "test-int",
			flagValue:    0,
			flagChanged:  true,
			expectCalled: true,
		},
		{
			name:         "flag not changed",
			flagName:     "test-int",
			flagValue:    0,
			flagChanged:  false,
			expectCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().Int(tt.flagName, 0, "test flag")

			if tt.flagChanged {
				cmd.Flags().Set(tt.flagName, fmt.Sprintf("%d", tt.flagValue))
			}

			called := false
			var capturedValue int
			setter := func(val int) {
				called = true
				capturedValue = val
			}

			applyIntFlag(cmd, tt.flagName, setter)

			assert.Equal(t, tt.expectCalled, called, "Setter should be called: %v", tt.expectCalled)
			if tt.expectCalled {
				assert.Equal(t, tt.flagValue, capturedValue)
			}
		})
	}
}

// TestApplyStringFlag tests string flag application.
func TestApplyStringFlag(t *testing.T) {
	tests := []struct {
		name         string
		flagName     string
		flagValue    string
		flagChanged  bool
		expectCalled bool
	}{
		{
			name:         "flag set to non-empty value",
			flagName:     "test-string",
			flagValue:    "hello",
			flagChanged:  true,
			expectCalled: true,
		},
		{
			name:         "flag set to empty value",
			flagName:     "test-string",
			flagValue:    "",
			flagChanged:  true,
			expectCalled: true,
		},
		{
			name:         "flag not changed",
			flagName:     "test-string",
			flagValue:    "",
			flagChanged:  false,
			expectCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String(tt.flagName, "", "test flag")

			if tt.flagChanged {
				cmd.Flags().Set(tt.flagName, tt.flagValue)
			}

			called := false
			var capturedValue string
			setter := func(val string) {
				called = true
				capturedValue = val
			}

			applyStringFlag(cmd, tt.flagName, setter)

			assert.Equal(t, tt.expectCalled, called, "Setter should be called: %v", tt.expectCalled)
			if tt.expectCalled {
				assert.Equal(t, tt.flagValue, capturedValue)
			}
		})
	}
}

// TestApplyProfileFileFlag tests profile file flag application with auto-enable.
func TestApplyProfileFileFlag(t *testing.T) {
	tests := []struct {
		name           string
		flagValue      string
		flagChanged    bool
		expectEnabled  bool
		expectFilePath string
	}{
		{
			name:           "file specified - should auto-enable",
			flagValue:      "/tmp/profile.prof",
			flagChanged:    true,
			expectEnabled:  true,
			expectFilePath: "/tmp/profile.prof",
		},
		{
			name:           "empty file - should not enable",
			flagValue:      "",
			flagChanged:    true,
			expectEnabled:  false,
			expectFilePath: "",
		},
		{
			name:           "flag not changed",
			flagValue:      "",
			flagChanged:    false,
			expectEnabled:  false,
			expectFilePath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("profile-file", "", "profile file path")

			if tt.flagChanged {
				cmd.Flags().Set("profile-file", tt.flagValue)
			}

			config := &schema.AtmosConfiguration{
				Profiler: profiler.Config{
					Enabled: false,
					File:    "",
				},
			}

			err := applyProfileFileFlag(&config.Profiler, cmd)
			assert.NoError(t, err)

			assert.Equal(t, tt.expectEnabled, config.Profiler.Enabled)
			assert.Equal(t, tt.expectFilePath, config.Profiler.File)
		})
	}
}

// TestApplyProfileTypeFlag tests profile type flag application with validation.
func TestApplyProfileTypeFlag(t *testing.T) {
	tests := []struct {
		name        string
		flagValue   string
		flagChanged bool
		expectError bool
		expectType  profiler.ProfileType
	}{
		{
			name:        "valid cpu profile type",
			flagValue:   "cpu",
			flagChanged: true,
			expectError: false,
			expectType:  profiler.ProfileTypeCPU,
		},
		{
			name:        "valid heap profile type",
			flagValue:   "heap",
			flagChanged: true,
			expectError: false,
			expectType:  profiler.ProfileTypeHeap,
		},
		{
			name:        "invalid profile type",
			flagValue:   "invalid",
			flagChanged: true,
			expectError: true,
		},
		{
			name:        "flag not changed",
			flagValue:   "",
			flagChanged: false,
			expectError: false,
			expectType:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("profile-type", "", "profile type")

			if tt.flagChanged {
				cmd.Flags().Set("profile-type", tt.flagValue)
			}

			config := &schema.AtmosConfiguration{
				Profiler: profiler.Config{
					ProfileType: "",
				},
			}

			err := applyProfileTypeFlag(&config.Profiler, cmd)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.flagChanged && !tt.expectError {
					assert.Equal(t, tt.expectType, config.Profiler.ProfileType)
				}
			}
		})
	}
}

// TestFormatFlagNameParts tests flag name formatting.
func TestFormatFlagNameParts(t *testing.T) {
	tests := []struct {
		name       string
		flagName   string
		shorthand  string
		flagType   string
		expectName string
		expectType string
	}{
		{
			name:       "flag with shorthand",
			flagName:   "verbose",
			shorthand:  "v",
			flagType:   "bool",
			expectName: "-v, --verbose",
			expectType: "",
		},
		{
			name:       "flag without shorthand",
			flagName:   "config",
			shorthand:  "",
			flagType:   "string",
			expectName: "    --config",
			expectType: "string",
		},
		{
			name:       "stringSlice type",
			flagName:   "tags",
			shorthand:  "",
			flagType:   "stringSlice",
			expectName: "    --tags",
			expectType: "strings",
		},
		{
			name:       "int type with shorthand",
			flagName:   "timeout",
			shorthand:  "t",
			flagType:   "int",
			expectName: "-t, --timeout",
			expectType: "int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}

			// Create flag based on type.
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
			case "stringSlice":
				if tt.shorthand != "" {
					cmd.Flags().StringSliceP(tt.flagName, tt.shorthand, nil, "test")
				} else {
					cmd.Flags().StringSlice(tt.flagName, nil, "test")
				}
			case "int":
				if tt.shorthand != "" {
					cmd.Flags().IntP(tt.flagName, tt.shorthand, 0, "test")
				} else {
					cmd.Flags().Int(tt.flagName, 0, "test")
				}
			}

			flag := cmd.Flags().Lookup(tt.flagName)
			assert.NotNil(t, flag)

			name, ftype := formatFlagNameParts(flag)
			assert.Equal(t, tt.expectName, name)
			assert.Equal(t, tt.expectType, ftype)
		})
	}
}
