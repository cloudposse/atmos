package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestColorProfilesRespectOptOuts verifies early and configured startup cannot
// re-enable color after an explicit opt-out, even when forced color is requested.
func TestColorProfilesRespectOptOuts(t *testing.T) {
	for _, stage := range []string{"early args", "configured", "parsed flags"} {
		t.Run(stage, func(t *testing.T) {
			_ = NewTestKit(t)
			t.Setenv("NO_COLOR", "")
			t.Setenv("ATMOS_NO_COLOR", "")
			os.Args = []string{"atmos", "--no-color", "--force-color"}
			var output bytes.Buffer
			log.SetOutput(&output)
			ui.SetColorProfile(termenv.TrueColor)
			switch stage {
			case "early args":
				setupColorProfileFromEnvWithArgs(os.Args)
			case "configured":
				cfg := &schema.AtmosConfiguration{}
				cfg.Settings.Terminal.ForceColor = true
				setupColorProfile(cfg)
			case "parsed flags":
				command := &cobra.Command{Use: "test"}
				command.Flags().Bool("no-color", true, "")
				command.Flags().Bool("force-color", true, "")
				configureEarlyColorProfile(command)
			}
			assert.Equal(t, termenv.Ascii, lipgloss.ColorProfile())
			log.Error("\x1b[31mplain\x1b[0m")
			assert.Contains(t, output.String(), "plain")
			assert.NotContains(t, output.String(), "\x1b")
		})
	}
}

// TestLoggerForceColorAfterFileChange verifies a new log destination retains
// explicitly requested color after Charm recreates its renderer.
func TestLoggerForceColorAfterFileChange(t *testing.T) {
	_ = NewTestKit(t)
	t.Setenv("NO_COLOR", "")
	t.Setenv("ATMOS_NO_COLOR", "")
	os.Args = []string{"atmos"}
	cfg := &schema.AtmosConfiguration{Logs: schema.Logs{Level: "Debug", File: filepath.Join(t.TempDir(), "atmos.log")}}
	cfg.Settings.Terminal.ForceColor = true
	SetupLogger(cfg)
	t.Cleanup(cleanupLogFile)
	log.Debug("forced log")
	output, err := os.ReadFile(cfg.Logs.File)
	require.NoError(t, err)
	assert.Contains(t, string(output), "forced log")
	assert.Contains(t, string(output), "\x1b")
}

// TestInvalidLogsColorIsFatalForVersion keeps invalid color configuration actionable
// even for version commands that normally tolerate missing project configuration.
func TestInvalidLogsColorIsFatalForVersion(t *testing.T) {
	_ = NewTestKit(t)
	err := handleConfigInitErrorWithArgs(errUtils.ErrInvalidLogsColor, &schema.AtmosConfiguration{}, []string{"atmos", "version"})
	require.ErrorIs(t, err, errUtils.ErrInvalidLogsColor)
}

// TestRootLogsColorFlagOverridesEnvironment exercises the real root startup path,
// keeping forced UI color while an explicit flag disables file-log color.
func TestRootLogsColorFlagOverridesEnvironment(t *testing.T) {
	_ = NewTestKit(t)
	dir := t.TempDir()
	t.Chdir(dir)
	config := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(config, []byte("base_path: .\n"), 0o600))
	t.Setenv("ATMOS_CONFIG", config)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("ATMOS_LOGS_COLOR", "true")
	t.Setenv("ATMOS_VERSION_CHECK_ENABLED", "false")
	t.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	t.Setenv("NO_COLOR", "")
	t.Setenv("ATMOS_NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	logPath := filepath.Join(dir, "atmos.log")
	args := []string{"version", "--force-color", "--logs-color=false", "--logs-level=debug", "--logs-file=" + logPath}
	os.Args = append([]string{"atmos"}, args...)
	RootCmd.SetArgs(args)
	t.Cleanup(cleanupLogFile)
	stdout, _ := captureStdoutStderr(t, func() { require.NoError(t, Execute()) })
	log.Debug("root flag took precedence")
	output, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(output), "root flag took precedence")
	assert.NotContains(t, string(output), "\x1b")
	assert.Contains(t, stdout, "\x1b")
}
