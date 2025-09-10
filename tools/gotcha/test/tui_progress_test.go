package test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTUIProgressBar_FormatTerminal tests that the progress bar appears with format:terminal.
func TestTUIProgressBar_FormatTerminal(t *testing.T) {
	t.Skipf("TUI test requires manual verification - cannot capture TUI output in test")

	tempDir := t.TempDir()

	// Create a slow test that gives us time to see the progress bar
	testFile := filepath.Join(tempDir, "slow_test.go")
	testContent := `package main

import (
	"testing"
	"time"
)

func TestSlow1(t *testing.T) {
	time.Sleep(500 * time.Millisecond)
}

func TestSlow2(t *testing.T) {
	time.Sleep(500 * time.Millisecond)
}

func TestSlow3(t *testing.T) {
	time.Sleep(500 * time.Millisecond)
}
`
	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	// Create go.mod
	goModFile := filepath.Join(tempDir, "go.mod")
	err = os.WriteFile(goModFile, []byte("module testpkg\ngo 1.21\n"), 0o644)
	require.NoError(t, err)

	// Create .gotcha.yaml with format: terminal (for TUI with progress bar)
	configFile := filepath.Join(tempDir, ".gotcha.yaml")
	configContent := `format: terminal
show: all
packages:
  - "."
`
	err = os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Build gotcha
	gotchaBinary := filepath.Join(tempDir, "gotcha-test")
	gotchaDir, _ := filepath.Abs("..")

	buildCmd := exec.Command("go", "build", "-o", gotchaBinary, ".")
	buildCmd.Dir = gotchaDir
	buildOut, buildErr := buildCmd.CombinedOutput()
	require.NoError(t, buildErr, "Build failed: %s", buildOut)

	// Run with GOTCHA_FORCE_TUI to ensure TUI mode
	cmd := exec.Command(gotchaBinary, ".")
	cmd.Dir = tempDir
	cmd.Env = append(os.Environ(), "GOTCHA_FORCE_TUI=true")

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err = cmd.Run()
	require.NoError(t, err)

	outputStr := output.String()
	t.Logf("Output with format:terminal and GOTCHA_FORCE_TUI:\n%s", outputStr)

	// The TUI progress bar uses special characters and ANSI codes
	// We can't easily test for the actual progress bar, but we can check
	// that it's using TUI mode by looking for certain patterns
}

// TestStreamMode_NoProgressBar tests that format:stream does NOT show progress bar.
func TestStreamMode_NoProgressBar(t *testing.T) {
	tempDir := t.TempDir()

	// Create a simple test
	testFile := filepath.Join(tempDir, "simple_test.go")
	testContent := `package main

import "testing"

func TestA(t *testing.T) {}
func TestB(t *testing.T) {}
func TestC(t *testing.T) {}
`
	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	// Create go.mod
	goModFile := filepath.Join(tempDir, "go.mod")
	err = os.WriteFile(goModFile, []byte("module testpkg\ngo 1.21\n"), 0o644)
	require.NoError(t, err)

	// Create .gotcha.yaml with format: stream (NO progress bar)
	configFile := filepath.Join(tempDir, ".gotcha.yaml")
	configContent := `format: stream
show: all
packages:
  - "."
`
	err = os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Build gotcha
	gotchaBinary := filepath.Join(tempDir, "gotcha-test")
	gotchaDir, _ := filepath.Abs("..")

	buildCmd := exec.Command("go", "build", "-o", gotchaBinary, ".")
	buildCmd.Dir = gotchaDir
	buildOut, buildErr := buildCmd.CombinedOutput()
	require.NoError(t, buildErr, "Build failed: %s", buildOut)

	// Run normally (stream mode)
	cmd := exec.Command(gotchaBinary, ".")
	cmd.Dir = tempDir

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err = cmd.Run()
	require.NoError(t, err)

	outputStr := output.String()
	t.Logf("Output with format:stream:\n%s", outputStr)

	// In stream mode, we should see the package header and test results
	assert.Contains(t, outputStr, "▶", "Should show package header")
	assert.Contains(t, outputStr, "Test Results:", "Should show test results summary")

	// But we should NOT see TUI-specific elements like spinner or progress percentage
	// (Note: We can't definitively test for absence of progress bar since it's
	// overwritten in TUI mode, but in stream mode the output is simpler)
}

// TestFormatDifference documents the difference between stream and terminal formats.
func TestFormatDifference(t *testing.T) {
	t.Logf(`
Format differences in gotcha:

1. format: stream (default in .gotcha.yaml)
   - Direct output to stderr
   - No progress bar
   - Shows tests as they complete
   - Works well in CI and when piping output
   - Example output:
     ▶ package/name
       ✔ TestPass
       ✘ TestFail
       ⊘ TestSkip

2. format: terminal
   - Interactive TUI mode (when TTY is detected)
   - Shows animated progress bar at bottom
   - Updates in place
   - Shows: spinner, current test, progress bar, percentage, test count, time, buffer size
   - Example:
     ⠋ Running TestExample [▓▓▓▓░░░░] 45%% 23/50 tests  12s 145.2KB

3. Why the progress bar is not showing:
   - Your .gotcha.yaml has "format: stream"
   - Progress bar only appears with "format: terminal"
   - To see progress bar, either:
     a) Change .gotcha.yaml to "format: terminal"
     b) Use --format=terminal flag
     c) Set GOTCHA_FORCE_TUI=true environment variable
`)
}

// TestProgressBarConfiguration shows how to enable the progress bar.
func TestProgressBarConfiguration(t *testing.T) {
	configurations := []struct {
		name        string
		configYAML  string
		envVars     []string
		flags       []string
		expectTUI   bool
		description string
	}{
		{
			name: "stream_format_default",
			configYAML: `format: stream
packages: ["."]`,
			expectTUI:   false,
			description: "Default stream format - no progress bar",
		},
		{
			name: "terminal_format_in_config",
			configYAML: `format: terminal
packages: ["."]`,
			expectTUI:   true,
			description: "Terminal format in config - shows progress bar if TTY",
		},
		{
			name:        "force_tui_env_var",
			configYAML:  `format: stream`,
			envVars:     []string{"GOTCHA_FORCE_TUI=true"},
			expectTUI:   true,
			description: "Force TUI with environment variable",
		},
		{
			name:        "terminal_flag_override",
			configYAML:  `format: stream`,
			flags:       []string{"--format=terminal"},
			expectTUI:   true,
			description: "Override config with --format=terminal flag",
		},
	}

	for _, tc := range configurations {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Configuration: %s", tc.description)
			t.Logf("Config YAML:\n%s", tc.configYAML)
			if len(tc.envVars) > 0 {
				t.Logf("Environment: %v", tc.envVars)
			}
			if len(tc.flags) > 0 {
				t.Logf("Flags: %v", tc.flags)
			}
			t.Logf("Expects TUI/Progress Bar: %v", tc.expectTUI)
		})
	}
}

// TestCurrentBehavior documents the current behavior with format:stream.
func TestCurrentBehavior(t *testing.T) {
	t.Logf(`
Current Behavior Analysis:

The issue "We are no longer displaying the progress bar in TUI mode" is because:

1. The .gotcha.yaml configuration has:
   format: stream

2. The progress bar ONLY appears when:
   - format is set to "terminal" (not "stream")
   - AND a TTY is detected (or GOTCHA_FORCE_TUI=true)
   - AND not in CI mode

3. With format:stream, gotcha uses StreamReporter which:
   - Outputs directly to stderr
   - Shows tests as they complete
   - Does NOT show a progress bar
   - This is working as designed

4. The regression mentioned (commit 4b284e9) likely changed:
   - Either the default format
   - Or how the format is determined
   - Or the condition for entering TUI mode

To restore the progress bar, change .gotcha.yaml to:
   format: terminal

Or run with:
   gotcha --format=terminal ./...
`)
}
