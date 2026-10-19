package startup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields the startup logger reads.
var _ = schema.Logs{Level: "", File: ""}

// restoreLogger returns the process-wide logger to its current state when the test ends.
func restoreLogger(t *testing.T) {
	t.Helper()
	level := log.GetLevel()
	t.Cleanup(func() {
		log.SetLevel(level)
		log.SetOutput(os.Stderr)
		CloseLogFile()
	})
}

// logsCommand builds a command carrying the logging flags and parses args against them.
func logsCommand(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "atmos"}
	f := cmd.PersistentFlags()
	f.String(logsLevelFlagName, "Info", "")
	f.String(logsFileFlagName, "/dev/stderr", "")
	f.Bool(verboseFlagName, false, "")
	require.NoError(t, cmd.ParseFlags(args))
	return cmd
}

func TestLogsConfigFromCommand(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		args      []string
		wantLevel string
		wantFile  string
	}{
		{name: "defaults", wantLevel: "Warning", wantFile: "/dev/stderr"},
		{name: "flag default is not an override", args: []string{}, wantLevel: "Warning", wantFile: "/dev/stderr"},
		{name: "environment", env: map[string]string{"ATMOS_LOGS_LEVEL": "Trace", "ATMOS_LOGS_FILE": "/dev/null"}, wantLevel: "Trace", wantFile: "/dev/null"},
		{name: "flag beats environment", env: map[string]string{"ATMOS_LOGS_LEVEL": "Trace"}, args: []string{"--logs-level=Off"}, wantLevel: "Off", wantFile: "/dev/stderr"},
		{name: "verbose environment", env: map[string]string{"ATMOS_VERBOSE": "true"}, wantLevel: "Debug", wantFile: "/dev/stderr"},
		{name: "verbose flag", args: []string{"--verbose"}, wantLevel: "Debug", wantFile: "/dev/stderr"},
		{name: "file flag", args: []string{"--logs-file=/dev/stdout"}, wantLevel: "Warning", wantFile: "/dev/stdout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"ATMOS_LOGS_LEVEL", "ATMOS_LOGS_FILE", "ATMOS_VERBOSE"} {
				t.Setenv(key, "")
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			logs := LogsConfigFromCommand(logsCommand(t, tt.args...))

			assert.Equal(t, tt.wantLevel, logs.Level)
			assert.Equal(t, tt.wantFile, logs.File)
		})
	}
}

func TestLogsConfigFromCommandWithoutLoggingFlags(t *testing.T) {
	t.Setenv("ATMOS_LOGS_LEVEL", "Debug")
	t.Setenv("ATMOS_LOGS_FILE", "")
	t.Setenv("ATMOS_VERBOSE", "")

	assert.Equal(t, "Debug", LogsConfigFromCommand(&cobra.Command{Use: "bare"}).Level)
	assert.Equal(t, "Debug", LogsConfigFromCommand(nil).Level)
}

func TestConfigureAppliesLogLevelFlagAndEnvironment(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  string
		args []string
		want string
	}{
		{name: "flag", args: []string{"--logs-level=Debug"}, want: "debug"},
		{name: "environment", env: "Trace", want: "trace"},
		{name: "flag beats environment", env: "Trace", args: []string{"--logs-level=Off"}, want: "off"},
		{name: "default", want: "warn"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resetGlobals(t)
			restoreLogger(t)
			t.Setenv("ATMOS_LOGS_LEVEL", tt.env)
			t.Setenv("ATMOS_LOGS_FILE", "")
			t.Setenv("ATMOS_VERBOSE", "")

			require.NoError(t, Configure(logsCommand(t, tt.args...)))

			assert.Equal(t, tt.want, log.GetLevelString())
		})
	}
}

func TestConfigureRejectsUnsupportedLogLevelWithUsageExitCode(t *testing.T) {
	resetGlobals(t)
	restoreLogger(t)
	t.Setenv("ATMOS_LOGS_LEVEL", "")
	t.Setenv("ATMOS_LOGS_FILE", "")

	err := Configure(logsCommand(t, "--logs-level=Loud"))

	require.ErrorIs(t, err, log.ErrInvalidLogLevel)
	assert.Equal(t, usageExitCode, errUtils.GetExitCode(err))
}

func TestConfigureWritesLogsToConfiguredFile(t *testing.T) {
	resetGlobals(t)
	restoreLogger(t)
	t.Setenv("ATMOS_LOGS_LEVEL", "")
	t.Setenv("ATMOS_VERBOSE", "")
	path := filepath.Join(t.TempDir(), "doctor.log")
	t.Setenv("ATMOS_LOGS_FILE", path)

	require.NoError(t, Configure(logsCommand(t, "--logs-level=Debug")))
	log.Debug("startup logger smoke message")
	CloseLogFile()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "startup logger smoke message")
}

func TestConfigureReportsUnusableLogFile(t *testing.T) {
	resetGlobals(t)
	restoreLogger(t)
	t.Setenv("ATMOS_LOGS_LEVEL", "")
	t.Setenv("ATMOS_LOGS_FILE", "")

	// A directory cannot be opened for appending, on every platform.
	err := Configure(logsCommand(t, "--logs-file="+t.TempDir()))

	require.ErrorIs(t, err, errUtils.ErrOpenFile)
	assert.Equal(t, usageExitCode, errUtils.GetExitCode(err))
}

func TestCloseLogFileWithoutOpenFile(t *testing.T) {
	CloseLogFile()
	CloseLogFile()
}
