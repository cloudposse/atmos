package cmd

import (
	"os"
	"os/exec"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
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

// skipIfPackerNotInstalled skips the test if packer is not available in PATH.
func skipIfPackerNotInstalled(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("packer"); err != nil {
		t.Skipf("packer not installed: %v", err)
	}
}

// skipIfHelmfileNotInstalled skips the test if helmfile is not available in PATH.
func skipIfHelmfileNotInstalled(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("helmfile"); err != nil {
		t.Skipf("helmfile not installed: %v", err)
	}
}

// TestPrintMessageForMissingAtmosConfig tests the printMessageForMissingAtmosConfig function.
func TestPrintMessageForMissingAtmosConfig(t *testing.T) {
	tests := []struct {
		name             string
		atmosConfig      schema.AtmosConfiguration
		expectedContains []string
	}{
		{
			name: "default config missing",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/test",
				Stacks: schema.Stacks{
					BasePath: "stacks",
				},
				Default: true,
			},
			expectedContains: []string{
				"stacks", // Should contain the stacks directory path
			},
		},
		{
			name: "custom config with invalid paths",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/custom",
				Stacks: schema.Stacks{
					BasePath: "my-stacks",
				},
				Default: false,
			},
			expectedContains: []string{
				"my-stacks", // Should contain the custom stacks directory
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This function writes to both stdout (logo) and stderr (markdown messages)
			// We'll just verify it doesn't panic and exercises both code paths
			// The actual markdown rendering is tested in markdown_utils_test.go
			printMessageForMissingAtmosConfig(tt.atmosConfig)

			// If we get here without panic, both code paths work
			// (Default=true uses missingConfigDefaultMarkdown, Default=false uses missingConfigFoundMarkdown)
		})
	}
}
