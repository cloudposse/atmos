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

// TestIsSensitiveEnvVar verifies that the sensitive-variable detector matches all known
// credential patterns (prefixes and suffixes) and correctly passes safe variables.
func TestIsSensitiveEnvVar(t *testing.T) {
	// Each entry documents which pattern it exercises.
	sensitive := []struct {
		name    string
		pattern string
	}{
		{"AWS_SECRET_ACCESS_KEY", "prefix AWS_SECRET_"},
		{"AWS_SECRET_ANYTHING", "prefix AWS_SECRET_"},
		{"AWS_SESSION_TOKEN", "suffix _TOKEN"},
		{"GITHUB_TOKEN", "suffix _TOKEN"},
		{"GH_TOKEN", "suffix _TOKEN"},
		{"GITLAB_TOKEN", "suffix _TOKEN"},
		{"VAULT_TOKEN", "suffix _TOKEN"},
		{"MY_APP_PASSWORD", "suffix _PASSWORD"},
		{"DB_PASSWORD", "suffix _PASSWORD"},
		{"SERVICE_API_KEY", "suffix _API_KEY"},
		{"MY_API_KEY", "suffix _API_KEY"},
		{"DB_PASSWD", "suffix _PASSWD"},
		{"DEPLOY_SECRET", "suffix _SECRET"},
		{"MY_SECRET", "suffix _SECRET"},
		{"TLS_PRIVATE_KEY", "suffix _PRIVATE_KEY"},
		{"SSH_PRIVATE_KEY", "suffix _PRIVATE_KEY"},
	}
	for _, tc := range sensitive {
		assert.True(t, isSensitiveEnvVar(tc.name), "expected %q (pattern: %s) to be sensitive", tc.name, tc.pattern)
	}

	notSensitive := []string{
		"PATH",
		"HOME",
		"USER",
		"AWS_REGION",
		"AWS_ACCESS_KEY_ID",
		"TOKENIZE",     // does not end with _TOKEN
		"MY_PASSWORDS", // does not end with _PASSWORD
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

// TestExtractCommandNamesFromShell verifies that the shell AST parser correctly extracts
// command names and identifies dynamic (non-literal) command references.
func TestExtractCommandNamesFromShell(t *testing.T) {
	tests := []struct {
		name       string
		cmd        string
		wantNames  []string
		wantLitAll bool // expected allLiteral return value
		wantErr    bool
	}{
		{
			name:       "single literal command",
			cmd:        "echo hello",
			wantNames:  []string{"echo"},
			wantLitAll: true,
		},
		{
			name:       "pipe with two literal commands",
			cmd:        "curl http://example.com | sh",
			wantNames:  []string{"curl", "sh"},
			wantLitAll: true,
		},
		{
			name:       "dynamic command via variable",
			cmd:        "$CMD arg",
			wantNames:  nil,   // no literal names extracted
			wantLitAll: false, // allLiteral must be false
		},
		{
			name:       "command substitution in pipeline position",
			cmd:        "echo $(date +%Y)",
			wantNames:  []string{"echo", "date"},
			wantLitAll: true, // both echo and date are literals
		},
		{
			name:       "dynamic command via substitution",
			cmd:        "$(get_cmd) arg",
			wantNames:  []string{"get_cmd"}, // get_cmd inside $() is a literal, but the outer call is dynamic
			wantLitAll: false,               // the outer command $(get_cmd) is not a static literal
		},
		{
			name:    "invalid shell syntax",
			cmd:     "echo <<<",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names, allLiteral, err := extractCommandNamesFromShell(tt.cmd)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantNames, names)
			assert.Equal(t, tt.wantLitAll, allLiteral)
		})
	}
}

// TestProcessTagExec_AllowedCommands_DynamicCommandBlocked verifies that a dynamic command
// reference (e.g. $VAR) is rejected when an allowlist is configured.
func TestProcessTagExec_AllowedCommands_DynamicCommandBlocked(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Exec: schema.ExecConfig{
			AllowedCommands: []string{"echo"},
		},
	}

	// $SOME_CMD is a dynamic command — cannot be statically verified against allowlist.
	result, err := ProcessTagExec("!exec $SOME_CMD hello", cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecCommandNotAllowed)
	assert.Nil(t, result)
}

// TestProcessTagExec_AllowedCommands_InvalidSyntax verifies that a command with invalid shell
// syntax is rejected (with ErrExecCommandNotAllowed) when an allowlist is configured.
func TestProcessTagExec_AllowedCommands_InvalidSyntax(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Exec: schema.ExecConfig{
			AllowedCommands: []string{"echo"},
		},
	}

	// "echo <<<" is invalid POSIX shell syntax — the parser will error.
	result, err := ProcessTagExec("!exec echo <<<", cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecCommandNotAllowed)
	assert.Nil(t, result)
}
