package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifyInsideGitRepo(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "git-repo-verify-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save current working directory
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

	// Restore original working directory
	if err := os.Chdir(currentDir); err != nil {
		t.Fatalf("Failed to restore working directory: %v", err)
	}
}
