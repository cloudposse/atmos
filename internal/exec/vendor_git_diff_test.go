package exec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildGitDiffArgs(t *testing.T) {
	tests := []struct {
		name         string
		opts         *GitDiffOptions
		colorize     bool
		expectedArgs []string
	}{
		{
			name: "basic diff with color",
			opts: &GitDiffOptions{
				FromRef: "v1.0.0",
				ToRef:   "v1.2.0",
				Context: 3,
				Unified: true,
			},
			colorize: true,
			expectedArgs: []string{
				"diff",
				"--color=always",
				"-U3",
				"--unified",
				"v1.0.0..v1.2.0",
			},
		},
		{
			name: "diff without color",
			opts: &GitDiffOptions{
				FromRef: "v1.0.0",
				ToRef:   "v1.2.0",
				Context: 3,
				Unified: true,
			},
			colorize: false,
			expectedArgs: []string{
				"diff",
				"--color=never",
				"-U3",
				"--unified",
				"v1.0.0..v1.2.0",
			},
		},
		{
			name: "diff with file filter",
			opts: &GitDiffOptions{
				FromRef:  "v1.0.0",
				ToRef:    "v1.2.0",
				Context:  5,
				Unified:  true,
				FilePath: "variables.tf",
			},
			colorize: false,
			expectedArgs: []string{
				"diff",
				"--color=never",
				"-U5",
				"--unified",
				"v1.0.0..v1.2.0",
				"--",
				"variables.tf",
			},
		},
		{
			name: "diff with zero context",
			opts: &GitDiffOptions{
				FromRef: "abc123",
				ToRef:   "def456",
				Context: 0,
				Unified: false,
			},
			colorize: true,
			expectedArgs: []string{
				"diff",
				"--color=always",
				"-U0",
				"abc123..def456",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildGitDiffArgs(tt.opts, tt.colorize)
			assert.Equal(t, tt.expectedArgs, args)
		})
	}
}

func TestShouldColorizeOutput(t *testing.T) {
	tests := []struct {
		name       string
		noColor    bool
		outputFile string
		termEnv    string
		expected   bool
		skipOnCI   bool
	}{
		{
			name:       "no-color flag set",
			noColor:    true,
			outputFile: "",
			termEnv:    "xterm-256color",
			expected:   false,
		},
		{
			name:       "output to file",
			noColor:    false,
			outputFile: "/tmp/output.txt",
			termEnv:    "xterm-256color",
			expected:   false,
		},
		{
			name:       "TERM is dumb",
			noColor:    false,
			outputFile: "",
			termEnv:    "dumb",
			expected:   false,
		},
		{
			name:       "TERM is empty",
			noColor:    false,
			outputFile: "",
			termEnv:    "",
			expected:   false,
		},
		{
			name:       "all conditions met for colorization",
			noColor:    false,
			outputFile: "",
			termEnv:    "xterm-256color",
			expected:   false, // Will be false in test environment (no TTY)
			skipOnCI:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnCI && os.Getenv("CI") != "" {
				t.Skip("Skipping TTY-dependent test in CI environment")
			}

			// Set TERM environment variable
			if tt.termEnv != "" {
				t.Setenv("TERM", tt.termEnv)
			}

			result := shouldColorizeOutput(tt.noColor, tt.outputFile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWriteOutput(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		outputFile string
		expectErr  bool
	}{
		{
			name:       "write to file",
			data:       []byte("test output"),
			outputFile: "output.txt",
			expectErr:  false,
		},
		{
			name:       "write to stdout",
			data:       []byte("test output"),
			outputFile: "",
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.outputFile != "" {
				// Create temp directory for file output
				tempDir := t.TempDir()
				tt.outputFile = tempDir + "/" + tt.outputFile
			}

			err := writeOutput(tt.data, tt.outputFile)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				if tt.outputFile != "" {
					// Verify file was written
					content, err := os.ReadFile(tt.outputFile)
					require.NoError(t, err)
					assert.Equal(t, tt.data, content)
				}
			}
		})
	}
}

func TestStripANSICodes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "text with ANSI codes",
			input:    "\x1b[31mred text\x1b[0m",
			expected: "31mred text0m",
		},
		{
			name:     "text without ANSI codes",
			input:    "plain text",
			expected: "plain text",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "multiple ANSI codes",
			input:    "\x1b[1m\x1b[31mbold red\x1b[0m\x1b[0m",
			expected: "1m31mbold red0m0m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSICodes([]byte(tt.input))
			assert.Equal(t, tt.expected, string(result))
		})
	}
}
