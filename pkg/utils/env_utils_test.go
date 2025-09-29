package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrependToPath(t *testing.T) {
	tests := []struct {
		name        string
		currentPath string
		newDir      string
		expected    string
	}{
		{
			name:        "empty path",
			currentPath: "",
			newDir:      "/test/bin",
			expected:    "/test/bin",
		},
		{
			name:        "existing path",
			currentPath: "/usr/bin:/bin",
			newDir:      "/test/bin",
			expected:    "/test/bin" + string(os.PathListSeparator) + "/usr/bin:/bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PrependToPath(tt.currentPath, tt.newDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPathFromEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		env      []string
		expected string
	}{
		{
			name:     "no PATH variable",
			env:      []string{"HOME=/home/user", "USER=testuser"},
			expected: "",
		},
		{
			name:     "PATH exists",
			env:      []string{"HOME=/home/user", "PATH=/usr/bin:/bin", "USER=testuser"},
			expected: "/usr/bin:/bin",
		},
		{
			name:     "empty PATH",
			env:      []string{"PATH=", "USER=testuser"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPathFromEnvironment(tt.env)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpdateEnvironmentPath(t *testing.T) {
	tests := []struct {
		name     string
		env      []string
		newDir   string
		expected []string
	}{
		{
			name:   "add PATH to empty environment",
			env:    []string{"HOME=/home/user"},
			newDir: "/test/bin",
			expected: []string{
				"HOME=/home/user",
				"PATH=/test/bin",
			},
		},
		{
			name:   "update existing PATH",
			env:    []string{"HOME=/home/user", "PATH=/usr/bin:/bin"},
			newDir: "/test/bin",
			expected: []string{
				"HOME=/home/user",
				"PATH=/test/bin" + string(os.PathListSeparator) + "/usr/bin:/bin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UpdateEnvironmentPath(tt.env, tt.newDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnsureBinaryInPath(t *testing.T) {
	tests := []struct {
		name       string
		env        []string
		binaryPath string
		expected   func([]string) bool // Function to validate result
	}{
		{
			name:       "binary directory not in PATH",
			env:        []string{"PATH=/usr/bin:/bin"},
			binaryPath: "/test/bin/atmos",
			expected: func(result []string) bool {
				path := GetPathFromEnvironment(result)
				return strings.HasPrefix(path, "/test/bin"+string(os.PathListSeparator))
			},
		},
		{
			name:       "binary directory already in PATH",
			env:        []string{"PATH=/test/bin:/usr/bin:/bin"},
			binaryPath: "/test/bin/atmos",
			expected: func(result []string) bool {
				path := GetPathFromEnvironment(result)
				return path == "/test/bin:/usr/bin:/bin"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnsureBinaryInPath(tt.env, tt.binaryPath)
			assert.True(t, tt.expected(result))
		})
	}
}

func TestEnvironmentPathIntegration(t *testing.T) {
	// Integration test: simulate the full AtmosRunner workflow
	originalEnv := []string{
		"HOME=/home/user",
		"PATH=/usr/bin:/bin",
		"USER=testuser",
	}

	// Simulate test binary in temp directory
	testBinaryPath := filepath.Join(os.TempDir(), "atmos-test-12345", "atmos")

	// Update environment with test binary
	updatedEnv := EnsureBinaryInPath(originalEnv, testBinaryPath)

	// Verify test binary directory is first in PATH
	updatedPath := GetPathFromEnvironment(updatedEnv)
	expectedPrefix := filepath.Join(os.TempDir(), "atmos-test-12345") + string(os.PathListSeparator)
	assert.True(t, strings.HasPrefix(updatedPath, expectedPrefix),
		"PATH should start with test binary directory: %s", updatedPath)

	// Verify original PATH components are preserved
	assert.Contains(t, updatedPath, "/usr/bin:/bin",
		"Original PATH should be preserved: %s", updatedPath)
}
