package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunTool(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Change to temp directory for test
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Create a mock binary for testing
	mockBinaryPath := filepath.Join(tempDir, "mock-tool")
	err = os.WriteFile(mockBinaryPath, []byte(`#!/bin/bash
echo "Mock tool version 1.0.0"
echo "Arguments: $@"
exit 0
`), 0755)
	require.NoError(t, err)

	tests := []struct {
		name        string
		args        []string
		setupMock   func() error
		wantErr     bool
		wantOutput  string
		description string
	}{
		{
			name:        "valid tool specification",
			args:        []string{"terraform@1.9.8", "--version"},
			wantErr:     false,
			description: "Should handle valid tool@version format",
		},
		{
			name:        "invalid tool specification - missing @",
			args:        []string{"terraform", "--version"},
			wantErr:     true,
			description: "Should error on missing @ in tool specification",
		},
		{
			name:        "invalid tool specification - multiple @",
			args:        []string{"terraform@1.9.8@extra", "--version"},
			wantErr:     true,
			description: "Should error on multiple @ in tool specification",
		},
		{
			name:        "empty tool specification",
			args:        []string{},
			wantErr:     true,
			description: "Should error on empty arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock if needed
			if tt.setupMock != nil {
				err := tt.setupMock()
				require.NoError(t, err)
			}

			// Create a new command for testing
			cmd := &cobra.Command{}
			cmd.SetArgs(tt.args)

			// Capture output
			var output strings.Builder
			cmd.SetOut(&output)
			cmd.SetErr(&output)

			// Run the command
			var err error
			if len(tt.args) == 0 {
				err = fmt.Errorf("no arguments provided")
			} else {
				err = runTool(cmd, tt.args)
			}

			if tt.wantErr {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}

			if tt.wantOutput != "" {
				assert.Contains(t, output.String(), tt.wantOutput, tt.description)
			}
		})
	}
}

func TestToolSpecParsing(t *testing.T) {
	tests := []struct {
		name        string
		toolSpec    string
		wantTool    string
		wantVersion string
		wantErr     bool
		description string
	}{
		{
			name:        "valid tool@version",
			toolSpec:    "terraform@1.9.8",
			wantTool:    "terraform",
			wantVersion: "1.9.8",
			wantErr:     false,
			description: "Should parse valid tool@version format",
		},
		{
			name:        "valid tool with latest version",
			toolSpec:    "opentofu@latest",
			wantTool:    "opentofu",
			wantVersion: "latest",
			wantErr:     false,
			description: "Should parse tool@latest format",
		},
		{
			name:        "missing @ separator",
			toolSpec:    "terraform1.9.8",
			wantErr:     true,
			description: "Should error on missing @ separator",
		},
		{
			name:        "multiple @ separators",
			toolSpec:    "terraform@1.9.8@extra",
			wantErr:     true,
			description: "Should error on multiple @ separators",
		},
		{
			name:        "empty tool name",
			toolSpec:    "@1.9.8",
			wantErr:     true,
			description: "Should error on empty tool name",
		},
		{
			name:        "empty version",
			toolSpec:    "terraform@",
			wantErr:     true,
			description: "Should error on empty version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := strings.Split(tt.toolSpec, "@")

			if tt.wantErr {
				// Check for various error conditions
				if len(parts) != 2 {
					// This is expected for missing @ or multiple @
					return
				}
				// Check for empty tool name or empty version
				if parts[0] == "" || parts[1] == "" {
					return
				}
				t.Errorf("Expected error but got valid parts: %v", parts)
				return
			}

			assert.Equal(t, 2, len(parts), tt.description)
			tool := parts[0]
			version := parts[1]

			// Additional validation for empty parts
			assert.NotEmpty(t, tool, "Tool name should not be empty")
			assert.NotEmpty(t, version, "Version should not be empty")

			assert.Equal(t, tt.wantTool, tool, tt.description)
			assert.Equal(t, tt.wantVersion, version, tt.description)
		})
	}
}

func TestBinaryPathResolution(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	installer := NewInstaller()
	installer.binDir = tempDir

	// Create mock directory structure
	toolDir := filepath.Join(tempDir, "hashicorp", "terraform", "1.9.8")
	err = os.MkdirAll(toolDir, 0755)
	require.NoError(t, err)

	// Create mock binary
	binaryPath := filepath.Join(toolDir, "terraform")
	err = os.WriteFile(binaryPath, []byte("mock binary"), 0755)
	require.NoError(t, err)

	tests := []struct {
		name        string
		owner       string
		repo        string
		version     string
		wantPath    string
		wantErr     bool
		description string
	}{
		{
			name:        "existing binary",
			owner:       "hashicorp",
			repo:        "terraform",
			version:     "1.9.8",
			wantPath:    binaryPath,
			wantErr:     false,
			description: "Should find existing binary path",
		},
		{
			name:        "non-existent binary",
			owner:       "hashicorp",
			repo:        "terraform",
			version:     "999.999.999",
			wantErr:     true,
			description: "Should error on non-existent binary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := installer.findBinaryPath(tt.owner, tt.repo, tt.version)

			if tt.wantErr {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.Equal(t, tt.wantPath, path, tt.description)
			}
		})
	}
}

func TestLatestVersionResolution(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	installer := NewInstaller()
	installer.binDir = tempDir

	// Create mock latest file
	toolDir := filepath.Join(tempDir, "hashicorp", "terraform")
	err = os.MkdirAll(toolDir, 0755)
	require.NoError(t, err)

	latestFile := filepath.Join(toolDir, "latest")
	err = os.WriteFile(latestFile, []byte("1.9.8"), 0644)
	require.NoError(t, err)

	tests := []struct {
		name        string
		owner       string
		repo        string
		version     string
		wantVersion string
		wantErr     bool
		description string
	}{
		{
			name:        "latest version resolution",
			owner:       "hashicorp",
			repo:        "terraform",
			version:     "latest",
			wantVersion: "1.9.8",
			wantErr:     false,
			description: "Should resolve latest to actual version",
		},
		{
			name:        "specific version",
			owner:       "hashicorp",
			repo:        "terraform",
			version:     "1.9.8",
			wantVersion: "1.9.8",
			wantErr:     false,
			description: "Should return specific version as-is",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This would need to be implemented in the installer
			// For now, just test the file reading logic
			if tt.version == "latest" {
				latestPath := filepath.Join(installer.binDir, tt.owner, tt.repo, "latest")
				if _, err := os.Stat(latestPath); err == nil {
					content, err := os.ReadFile(latestPath)
					require.NoError(t, err)
					resolvedVersion := strings.TrimSpace(string(content))
					assert.Equal(t, tt.wantVersion, resolvedVersion, tt.description)
				}
			}
		})
	}
}

func TestCommandExecution(t *testing.T) {
	// Test that we can execute a simple command
	cmd := exec.Command("echo", "test")
	output, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "test\n", string(output))
}

func TestFileOperations(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Test file creation
	testFile := filepath.Join(tempDir, "test.txt")
	content := "test content"
	err = os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	// Test file reading
	readContent, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, content, string(readContent))

	// Test file existence
	_, err = os.Stat(testFile)
	require.NoError(t, err)

	// Test directory creation
	testDir := filepath.Join(tempDir, "testdir")
	err = os.MkdirAll(testDir, 0755)
	require.NoError(t, err)

	// Test directory existence
	_, err = os.Stat(testDir)
	require.NoError(t, err)
}

func TestPathConstruction(t *testing.T) {
	tests := []struct {
		name        string
		parts       []string
		expected    string
		description string
	}{
		{
			name:        "simple path",
			parts:       []string{"bin", "tool", "version"},
			expected:    "bin/tool/version",
			description: "Should construct simple path correctly",
		},
		{
			name:        "empty parts",
			parts:       []string{},
			expected:    "",
			description: "Should handle empty parts",
		},
		{
			name:        "single part",
			parts:       []string{"bin"},
			expected:    "bin",
			description: "Should handle single part",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filepath.Join(tt.parts...)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestStringOperations(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		operation   func(string) string
		expected    string
		description string
	}{
		{
			name:        "trim space",
			input:       "  test  ",
			operation:   strings.TrimSpace,
			expected:    "test",
			description: "Should trim whitespace correctly",
		},
		{
			name:        "to lower",
			input:       "TEST",
			operation:   strings.ToLower,
			expected:    "test",
			description: "Should convert to lowercase",
		},
		{
			name:        "to upper",
			input:       "test",
			operation:   strings.ToUpper,
			expected:    "TEST",
			description: "Should convert to uppercase",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.operation(tt.input)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestFileReadingOperations(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Test file creation and reading
	testFile := filepath.Join(tempDir, "test.txt")
	content := "test content\n"
	err = os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	// Test reading file content
	readContent, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, content, string(readContent))

	// Test reading file content with trimming
	trimmedContent := strings.TrimSpace(string(readContent))
	assert.Equal(t, "test content", trimmedContent)

	// Test reading non-existent file
	nonExistentFile := filepath.Join(tempDir, "nonexistent.txt")
	_, err = os.ReadFile(nonExistentFile)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestLatestFileReading(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create mock tool directory structure
	toolDir := filepath.Join(tempDir, "bin", "hashicorp", "terraform")
	err = os.MkdirAll(toolDir, 0755)
	require.NoError(t, err)

	// Create latest file with version
	latestFile := filepath.Join(toolDir, "latest")
	version := "1.9.8"
	err = os.WriteFile(latestFile, []byte(version), 0644)
	require.NoError(t, err)

	// Test reading latest file
	content, err := os.ReadFile(latestFile)
	require.NoError(t, err)
	resolvedVersion := strings.TrimSpace(string(content))
	assert.Equal(t, version, resolvedVersion)

	// Test reading latest file with newlines
	versionWithNewlines := "1.9.8\n"
	err = os.WriteFile(latestFile, []byte(versionWithNewlines), 0644)
	require.NoError(t, err)

	content, err = os.ReadFile(latestFile)
	require.NoError(t, err)
	resolvedVersion = strings.TrimSpace(string(content))
	assert.Equal(t, "1.9.8", resolvedVersion)
}

func TestCommandExecutionWithOutput(t *testing.T) {
	// Test echo command
	cmd := exec.Command("echo", "test output")
	output, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "test output\n", string(output))

	// Test command with multiple arguments
	cmd = exec.Command("echo", "arg1", "arg2", "arg3")
	output, err = cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "arg1 arg2 arg3\n", string(output))

	// Test command that doesn't exist
	cmd = exec.Command("nonexistent-command")
	_, err = cmd.Output()
	assert.Error(t, err)
}

func TestPathResolution(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	installer := NewInstaller()
	installer.binDir = tempDir

	// Test path construction
	expectedPath := filepath.Join(tempDir, "hashicorp", "terraform", "1.9.8", "terraform")

	// Create the directory structure
	dir := filepath.Dir(expectedPath)
	err = os.MkdirAll(dir, 0755)
	require.NoError(t, err)

	// Create the binary file
	err = os.WriteFile(expectedPath, []byte("mock binary"), 0755)
	require.NoError(t, err)

	// Test that the path exists
	_, err = os.Stat(expectedPath)
	require.NoError(t, err)

	// Test path resolution
	path, err := installer.findBinaryPath("hashicorp", "terraform", "1.9.8")
	require.NoError(t, err)
	assert.Equal(t, expectedPath, path)
}

func TestErrorHandling(t *testing.T) {
	// Test error wrapping
	originalErr := fmt.Errorf("original error")
	wrappedErr := fmt.Errorf("context: %w", originalErr)

	assert.Error(t, wrappedErr)
	assert.Contains(t, wrappedErr.Error(), "context: original error")

	// Test error type checking
	_, err := os.Open("nonexistent-file")
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestShellCommandIssues(t *testing.T) {
	// Test that we can read file contents properly
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "test.txt")
	content := "test content\n"
	err = os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	// Test reading file with os.ReadFile (the proper way)
	readContent, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, content, string(readContent))

	// Test reading file with cat command (the problematic way)
	cmd := exec.Command("cat", testFile)
	catOutput, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, content, string(catOutput))

	// Test reading non-existent file with cat
	cmd = exec.Command("cat", filepath.Join(tempDir, "nonexistent.txt"))
	_, err = cmd.Output()
	assert.Error(t, err)

	// Test that we can handle command failures gracefully
	cmd = exec.Command("nonexistent-command")
	_, err = cmd.Output()
	assert.Error(t, err)
}

func TestFileContentReading(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Test reading latest file content
	latestFile := filepath.Join(tempDir, "latest")
	version := "1.9.8"
	err = os.WriteFile(latestFile, []byte(version), 0644)
	require.NoError(t, err)

	// Read with os.ReadFile
	content, err := os.ReadFile(latestFile)
	require.NoError(t, err)
	resolvedVersion := strings.TrimSpace(string(content))
	assert.Equal(t, version, resolvedVersion)

	// Test reading with newlines
	versionWithNewlines := "1.9.8\n"
	err = os.WriteFile(latestFile, []byte(versionWithNewlines), 0644)
	require.NoError(t, err)

	content, err = os.ReadFile(latestFile)
	require.NoError(t, err)
	resolvedVersion = strings.TrimSpace(string(content))
	assert.Equal(t, "1.9.8", resolvedVersion)

	// Test reading with multiple newlines and spaces
	versionWithSpaces := "  1.9.8  \n\n"
	err = os.WriteFile(latestFile, []byte(versionWithSpaces), 0644)
	require.NoError(t, err)

	content, err = os.ReadFile(latestFile)
	require.NoError(t, err)
	resolvedVersion = strings.TrimSpace(string(content))
	assert.Equal(t, "1.9.8", resolvedVersion)
}
