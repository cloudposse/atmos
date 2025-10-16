package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProcessTagExec_CustomBinary tests the ProcessTagExec function using a custom bash binary.
func TestProcessTagExec_CustomBinary(t *testing.T) {
	// Create a temporary directory for our custom binary.
	tempDir := t.TempDir()

	// Create a custom bash script that outputs predictable content.
	customBinaryPath := filepath.Join(tempDir, "test-bash")
	if runtime.GOOS == "windows" {
		customBinaryPath += ".bat"
	}

	// Script content based on OS.
	var scriptContent string
	if runtime.GOOS == "windows" {
		scriptContent = `@echo off
if "%1"=="simple" echo hello world
if "%1"=="json" echo {"key": "value", "number": 42}
if "%1"=="array" echo ["item1", "item2", "item3"]
if "%1"=="empty" echo.
if "%1"=="number" echo 123
if "%1"=="complex" echo {"nested": {"key": "value"}, "array": [1, 2, 3], "bool": true, "null": null}
if "%1"=="invalid" echo invalid json {
`
	} else {
		scriptContent = `#!/bin/bash
case "$1" in
  "simple") echo "hello world" ;;
  "json") echo '{"key": "value", "number": 42}' ;;
  "array") echo '["item1", "item2", "item3"]' ;;
  "empty") echo ;;
  "number") echo "123" ;;
  "complex") echo '{"nested": {"key": "value"}, "array": [1, 2, 3], "bool": true, "null": null}' ;;
  "invalid") echo "invalid json {" ;;
esac
`
	}

	err := os.WriteFile(customBinaryPath, []byte(scriptContent), 0o755)
	assert.NoError(t, err)

	// Save original PATH and modify it to include our temp directory.
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+originalPath)
	assert.NoError(t, err)

	tests := []struct {
		name     string
		input    string
		expected interface{}
		wantErr  bool
	}{
		{
			name:     "simple string output",
			input:    "!exec test-bash simple",
			expected: "hello world",
			wantErr:  false,
		},
		{
			name:     "JSON output that gets parsed",
			input:    "!exec test-bash json",
			expected: map[string]interface{}{"key": "value", "number": float64(42)},
			wantErr:  false,
		},
		{
			name:     "JSON array output",
			input:    "!exec test-bash array",
			expected: []interface{}{"item1", "item2", "item3"},
			wantErr:  false,
		},
		{
			name:     "empty output",
			input:    "!exec test-bash empty",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "numeric string output",
			input:    "!exec test-bash number",
			expected: float64(123),
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessTagExec(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Normalize line endings for cross-platform compatibility.
			if resultStr, ok := result.(string); ok {
				// Remove trailing whitespace and normalize line endings.
				resultStr = strings.TrimSpace(resultStr)
				assert.Equal(t, tt.expected, resultStr)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestProcessTagExec_InvalidCommand tests error handling for invalid commands.
func TestProcessTagExec_InvalidCommand(t *testing.T) {
	input := "!exec invalid_command_that_does_not_exist"
	result, err := ProcessTagExec(input)

	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestProcessTagExec_MalformedTag tests error handling for malformed tags.
func TestProcessTagExec_MalformedTag(t *testing.T) {
	input := "!exec"
	result, err := ProcessTagExec(input)

	// This should handle the case where there's no command after !exec.
	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestProcessTagExec_ComplexJSON tests parsing of complex JSON output.
func TestProcessTagExec_ComplexJSON(t *testing.T) {
	// Create a temporary directory for our custom binary.
	tempDir := t.TempDir()

	// Create a custom bash script that outputs complex JSON.
	customBinaryPath := filepath.Join(tempDir, "test-bash")
	if runtime.GOOS == "windows" {
		customBinaryPath += ".bat"
	}

	// Script content based on OS.
	var scriptContent string
	if runtime.GOOS == "windows" {
		scriptContent = `@echo off
echo {"nested": {"key": "value"}, "array": [1, 2, 3], "bool": true, "null": null}
`
	} else {
		scriptContent = `#!/bin/bash
echo '{"nested": {"key": "value"}, "array": [1, 2, 3], "bool": true, "null": null}'
`
	}

	err := os.WriteFile(customBinaryPath, []byte(scriptContent), 0o755)
	assert.NoError(t, err)

	// Save original PATH and modify it to include our temp directory.
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+originalPath)
	assert.NoError(t, err)

	input := "!exec test-bash"
	result, err := ProcessTagExec(input)

	assert.NoError(t, err)

	expected := map[string]interface{}{
		"nested": map[string]interface{}{"key": "value"},
		"array":  []interface{}{float64(1), float64(2), float64(3)},
		"bool":   true,
		"null":   nil,
	}

	assert.Equal(t, expected, result)
}

// TestProcessTagExec_InvalidJSON tests handling of invalid JSON that should be returned as string.
func TestProcessTagExec_InvalidJSON(t *testing.T) {
	// Create a temporary directory for our custom binary.
	tempDir := t.TempDir()

	// Create a custom bash script that outputs invalid JSON.
	customBinaryPath := filepath.Join(tempDir, "test-bash")
	if runtime.GOOS == "windows" {
		customBinaryPath += ".bat"
	}

	// Script content based on OS.
	var scriptContent string
	if runtime.GOOS == "windows" {
		scriptContent = `@echo off
echo invalid json {
`
	} else {
		scriptContent = `#!/bin/bash
echo "invalid json {"
`
	}

	err := os.WriteFile(customBinaryPath, []byte(scriptContent), 0o755)
	assert.NoError(t, err)

	// Save original PATH and modify it to include our temp directory.
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+originalPath)
	assert.NoError(t, err)

	input := "!exec test-bash"
	result, err := ProcessTagExec(input)

	assert.NoError(t, err)
	// Should return as string when JSON parsing fails.
	// Normalize line endings for cross-platform compatibility.
	resultStr := strings.TrimSpace(result.(string))
	assert.Equal(t, "invalid json {", resultStr)
}
