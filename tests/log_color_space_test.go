package tests

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runAtmosScrubbedEnv runs the Atmos binary with color-related environment removed.
// It returns stdout, stderr and the exit error (nil on success).
func runAtmosScrubbedEnv(t *testing.T, binary, dir string, env, args []string) (stdout, stderr string, err error) {
	t.Helper()
	config := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(config, []byte("base_path: .\n"), 0o600))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "ATMOS_") || key == "NO_COLOR" || key == "FORCE_COLOR" || key == "CLICOLOR" || key == "CLICOLOR_FORCE" || key == "CI" || key == "GITHUB_ACTIONS" || key == "TERM" || key == "COLORTERM" {
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "ATMOS_CONFIG="+config, "ATMOS_BASE_PATH="+dir, "ATMOS_TELEMETRY_ENABLED=false", "ATMOS_VERSION_CHECK_ENABLED=false", "TERM=xterm-256color")
	cmd.Env = append(cmd.Env, env...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err = cmd.Run()
	return out.String(), errOut.String(), err
}

// TestLogColorSpaceSeparatedBoolValues verifies that "--flag true|false" works for boolean
// flags, and that a bare boolean flag never swallows the subcommand that follows it.
func TestLogColorSpaceSeparatedBoolValues(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the Atmos binary")
	}
	binary := buildAtmosBinary(t)

	t.Run("logs-color false disables log color", func(t *testing.T) {
		dir := t.TempDir()
		logFile := filepath.Join(dir, "atmos.log")
		stdout, stderr, err := runAtmosScrubbedEnv(t, binary, dir, []string{"ATMOS_FORCE_COLOR=true"},
			[]string{"--logs-level=debug", "--logs-file=" + logFile, "--force-color", "--logs-color", "false", "version"})
		require.NoError(t, err, "%s", stderr)
		assert.NotContains(t, stderr, "Unknown command")
		assert.Contains(t, stdout, "\x1b", "UI output keeps color when only logs are plain")
		logs, readErr := os.ReadFile(logFile)
		require.NoError(t, readErr)
		require.NotEmpty(t, logs)
		assert.NotContains(t, string(logs), "\x1b", "raw log: %q", logs)
	})

	t.Run("logs-color true enables log color", func(t *testing.T) {
		dir := t.TempDir()
		logFile := filepath.Join(dir, "atmos.log")
		_, stderr, err := runAtmosScrubbedEnv(t, binary, dir, nil,
			[]string{"--logs-level=debug", "--logs-file=" + logFile, "--force-color", "--logs-color", "true", "version"})
		require.NoError(t, err, "%s", stderr)
		logs, readErr := os.ReadFile(logFile)
		require.NoError(t, readErr)
		require.NotEmpty(t, logs)
		assert.Contains(t, string(logs), "\x1b", "raw log: %q", logs)
	})

	t.Run("no-color false keeps the version command", func(t *testing.T) {
		dir := t.TempDir()
		stdout, stderr, err := runAtmosScrubbedEnv(t, binary, dir, nil, []string{"--no-color", "false", "version"})
		require.NoError(t, err, "%s", stderr)
		assert.NotContains(t, stderr, "Unknown command")
		assert.Contains(t, stdout, "Atmos")
	})

	t.Run("bare logs-color does not swallow the version command", func(t *testing.T) {
		dir := t.TempDir()
		stdout, stderr, err := runAtmosScrubbedEnv(t, binary, dir, nil, []string{"--logs-color", "version"})
		require.NoError(t, err, "%s", stderr)
		assert.Contains(t, stdout, "Atmos")
		assert.NotContains(t, stdout, "Available Commands", "version output, not root help")
	})

	t.Run("bare no-color does not swallow the version command", func(t *testing.T) {
		dir := t.TempDir()
		stdout, stderr, err := runAtmosScrubbedEnv(t, binary, dir, nil, []string{"--no-color", "version"})
		require.NoError(t, err, "%s", stderr)
		assert.Contains(t, stdout, "Atmos")
		assert.NotContains(t, stdout, "Available Commands", "version output, not root help")
	})
}
