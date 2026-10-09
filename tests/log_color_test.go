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

// TestLogColorStartup exercises the real initialization order from issue #3342.
func TestLogColorStartup(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the Atmos binary")
	}
	binary := buildAtmosBinary(t)
	for _, tt := range []struct {
		name        string
		env         []string
		flags       []string
		wantColor   bool
		globalPlain bool
	}{
		{"CI color prerequisite", []string{"CI=true", "GITHUB_ACTIONS=true"}, nil, true, false},
		{"reporter command in CI", []string{"CI=true", "GITHUB_ACTIONS=true"}, []string{"--no-color"}, false, true},
		{"flag beats forced color", []string{"ATMOS_FORCE_COLOR=true"}, []string{"--no-color"}, false, true},
		{"NO_COLOR beats forced color", []string{"NO_COLOR=1", "ATMOS_FORCE_COLOR=true"}, []string{"--logs-color=true"}, false, true},
		{"logging false preserves color UI", []string{"ATMOS_FORCE_COLOR=true"}, []string{"--logs-color=false"}, false, false},
		{"logging true respects pipe", nil, []string{"--logs-color=true"}, false, false},
		{"force-color enables logs", nil, []string{"--logs-color=true", "--force-color"}, true, false},
		{"global veto beats logging true", []string{"CI=true", "GITHUB_ACTIONS=true"}, []string{"--logs-color=true", "--no-color"}, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			config := filepath.Join(dir, "atmos.yaml")
			require.NoError(t, os.WriteFile(config, []byte("base_path: .\n"), 0o600))
			logFile := filepath.Join(dir, "atmos.log")
			args := []string{"--logs-level=debug", "--logs-file=" + logFile}
			args = append(args, tt.flags...)
			args = append(args, "version")
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
			cmd.Env = append(cmd.Env, tt.env...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			require.NoError(t, cmd.Run(), "%s", stderr.String())
			logs, err := os.ReadFile(logFile)
			require.NoError(t, err)
			require.NotEmpty(t, logs)
			assert.Equal(t, tt.wantColor, bytes.Contains(logs, []byte("\x1b")), "raw log: %q", logs)
			if tt.globalPlain {
				assert.NotContains(t, stdout.String(), "\x1b")
				assert.NotContains(t, stderr.String(), "\x1b")
			}
			if tt.name == "logging false preserves color UI" {
				assert.Contains(t, stdout.String(), "\x1b")
			}
		})
	}
}
