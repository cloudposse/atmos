package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewVersionCmd(t *testing.T) {
	cmd := newVersionCmd()

	// Test command properties
	assert.Equal(t, "version", cmd.Use)
	assert.Equal(t, "Print version information", cmd.Short)
	assert.NotNil(t, cmd.Run)
}

func TestVersionCommand_Output(t *testing.T) {
	// Save original values
	origVersion := Version
	origBuildTime := BuildTime
	origGitCommit := GitCommit
	defer func() {
		Version = origVersion
		BuildTime = origBuildTime
		GitCommit = origGitCommit
	}()

	tests := []struct {
		name      string
		version   string
		buildTime string
		gitCommit string
		expected  []string
	}{
		{
			name:      "default values",
			version:   "dev",
			buildTime: "unknown",
			gitCommit: "unknown",
			expected: []string{
				"gotcha version dev",
				"Build time: unknown",
				"Git commit: unknown",
			},
		},
		{
			name:      "custom version",
			version:   "v1.2.3",
			buildTime: "2025-01-01T00:00:00Z",
			gitCommit: "abc123def456",
			expected: []string{
				"gotcha version v1.2.3",
				"Build time: 2025-01-01T00:00:00Z",
				"Git commit: abc123def456",
			},
		},
		{
			name:      "empty values",
			version:   "",
			buildTime: "",
			gitCommit: "",
			expected: []string{
				"gotcha version",
				"Build time:",
				"Git commit:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test values
			Version = tt.version
			BuildTime = tt.buildTime
			GitCommit = tt.gitCommit

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Run command
			cmd := newVersionCmd()
			cmd.Run(cmd, []string{})

			// Restore stdout and read output
			w.Close()
			os.Stdout = oldStdout
			output, _ := io.ReadAll(r)
			outputStr := string(output)

			// Check expected strings
			for _, expected := range tt.expected {
				assert.Contains(t, outputStr, expected, "Output should contain expected string")
			}
		})
	}
}

func TestVersionCommand_FormattedOutput(t *testing.T) {
	// Set known values
	origVersion := Version
	origBuildTime := BuildTime
	origGitCommit := GitCommit
	defer func() {
		Version = origVersion
		BuildTime = origBuildTime
		GitCommit = origGitCommit
	}()

	Version = "v2.0.0"
	BuildTime = "2025-09-18T10:00:00Z"
	GitCommit = "1234567890abcdef"

	// Capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := newVersionCmd()
	cmd.Run(cmd, []string{})

	w.Close()
	os.Stdout = oldStdout
	output, _ := io.ReadAll(r)
	outputLines := strings.Split(strings.TrimSpace(string(output)), "\n")

	// Verify formatting
	assert.Len(t, outputLines, 3, "Should output exactly 3 lines")
	assert.Equal(t, "gotcha version v2.0.0", outputLines[0])
	assert.Equal(t, "  Build time: 2025-09-18T10:00:00Z", outputLines[1])
	assert.Equal(t, "  Git commit: 1234567890abcdef", outputLines[2])
}

func TestVersionCommand_Execute(t *testing.T) {
	// Capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Create and run command (don't use Execute which tries to parse args)
	cmd := newVersionCmd()
	cmd.Run(cmd, []string{})

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout
	output, _ := io.ReadAll(r)

	// Verify output
	assert.Contains(t, string(output), "gotcha version", "Output should contain version information")
}

func TestVersionVariables(t *testing.T) {
	// Test that version variables exist and have expected types
	assert.IsType(t, "", Version, "Version should be a string")
	assert.IsType(t, "", BuildTime, "BuildTime should be a string")
	assert.IsType(t, "", GitCommit, "GitCommit should be a string")

	// Test default values (when not overridden at build time)
	// These may be different in CI, so we just check they're not empty
	assert.NotEmpty(t, Version, "Version should not be empty")
}

func TestVersionCommand_WithLongCommitHash(t *testing.T) {
	// Test with a full-length git commit hash
	origGitCommit := GitCommit
	defer func() {
		GitCommit = origGitCommit
	}()

	GitCommit = "a" + strings.Repeat("0", 39) // 40-character hash

	// Capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := newVersionCmd()
	cmd.Run(cmd, []string{})

	w.Close()
	os.Stdout = oldStdout
	output, _ := io.ReadAll(r)

	// Verify the full commit hash is displayed
	assert.Contains(t, string(output), fmt.Sprintf("Git commit: %s", GitCommit))
}