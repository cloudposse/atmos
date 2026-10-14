package cmd

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestStandaloneScriptDispatch(t *testing.T) {
	NewTestKit(t)
	for _, args := range [][]string{{"atmos"}, {"atmos", "version"}} {
		os.Args = args
		before := reflect.ValueOf(RootCmd.RunE).Pointer()
		restore, err := prepareStandaloneScript()
		require.NoError(t, err)
		assert.Equal(t, before, reflect.ValueOf(RootCmd.RunE).Pointer())
		assert.Equal(t, args, os.Args)
		restore()
	}
	path := filepath.Join(t.TempDir(), "deploy.star")
	require.NoError(t, os.WriteFile(path, []byte("print(ctx.args)"), 0o600))
	original := []string{"atmos", path, "--help", "--chdir=script-argument", "--use-version=script-argument"}
	os.Args = original
	before := reflect.ValueOf(RootCmd.RunE).Pointer()
	restore, err := prepareStandaloneScript()
	require.NoError(t, err)
	assert.Equal(t, []string{"atmos"}, os.Args)
	assert.NotEqual(t, before, reflect.ValueOf(RootCmd.RunE).Pointer())
	restore()
	assert.Equal(t, original, os.Args)
	assert.Equal(t, before, reflect.ValueOf(RootCmd.RunE).Pointer())
}

func TestStandaloneScriptExecution(t *testing.T) {
	for _, tc := range []struct {
		name, source, stdout, errorText string
	}{
		{
			name: "script arguments and sibling imports",
			source: `load("helper.star", "message")
output = {"message": message, "args": ctx.args}`,
			stdout: "{\"args\":[\"--help\",\"--chdir=script-owned\"],\"message\":\"sibling loaded\"}\n",
		},
		{name: "print without output", source: `print("script says hello")`, stdout: "script says hello\n"},
		{name: "empty output still writes newline", source: `output = ""`, stdout: "\n"},
		{name: "interpreter failure", source: `fail("standalone failure")`, errorText: "standalone failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			NewTestKit(t)
			dir := t.TempDir()
			t.Chdir(t.TempDir()) // Imports must resolve beside the script, not in the invocation directory.
			path := filepath.Join(dir, "script with spaces.star")
			require.NoError(t, os.WriteFile(path, []byte(tc.source), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "helper.star"), []byte(`message = "sibling loaded"`), 0o600))
			os.Args = []string{"atmos", path, "--help", "--chdir=script-owned"}
			restore, err := prepareStandaloneScript()
			require.NoError(t, err)
			defer restore()
			command := &cobra.Command{}
			command.SetContext(context.Background())
			stdout, stderr := captureStdoutStderr(t, func() {
				iolib.Reset()
				require.NoError(t, iolib.Initialize())
				data.InitWriter(iolib.GetContext())
				err = RootCmd.RunE(command, nil)
			})
			if tc.errorText != "" {
				require.ErrorContains(t, err, tc.errorText)
				require.ErrorIs(t, err, errUtils.ErrStarlark)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.stdout, stdout)
			assert.Empty(t, stderr)
		})
	}
}

func TestStandaloneScriptMissingFile(t *testing.T) {
	NewTestKit(t)
	path := filepath.Join(t.TempDir(), "missing.star")
	os.Args = []string{"atmos", path}
	before := reflect.ValueOf(RootCmd.RunE).Pointer()
	restore, err := prepareStandaloneScript()
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Nil(t, restore)
	assert.Equal(t, before, reflect.ValueOf(RootCmd.RunE).Pointer())
	assert.Equal(t, []string{"atmos", path}, os.Args)

	// A script can disappear after detection but before execution.
	command := &cobra.Command{}
	command.SetContext(context.Background())
	err = runStandaloneScript(command, &script.File{Path: path})
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.ErrorContains(t, err, "read script")
}

func TestStandaloneScriptMetricsSummaryOptIn(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	for _, tc := range []struct {
		name     string
		enabled  *bool
		expected bool
	}{
		{name: "unset is suppressed", enabled: nil, expected: false},
		{name: "explicit true is preserved", enabled: boolPtr(true), expected: true},
		{name: "explicit false is preserved", enabled: boolPtr(false), expected: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := schema.AtmosConfiguration{}
			config.Settings.Metrics.Enabled = tc.enabled

			suppressMetricsSummaryByDefault(&config)

			require.NotNil(t, config.Settings.Metrics.Enabled)
			assert.Equal(t, tc.expected, *config.Settings.Metrics.Enabled)
		})
	}
}

func TestStandaloneScriptAppliesMetricsDefault(t *testing.T) {
	NewTestKit(t)
	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	atmosConfig.Settings.Metrics.Enabled = nil

	path := filepath.Join(t.TempDir(), "noop.star")
	require.NoError(t, os.WriteFile(path, []byte(`output = "ok"`), 0o600))
	command := &cobra.Command{}
	command.SetContext(context.Background())
	captureStdoutStderr(t, func() {
		iolib.Reset()
		require.NoError(t, iolib.Initialize())
		data.InitWriter(iolib.GetContext())
		require.NoError(t, runStandaloneScript(command, &script.File{Path: path}))
	})

	require.NotNil(t, atmosConfig.Settings.Metrics.Enabled)
	assert.False(t, *atmosConfig.Settings.Metrics.Enabled)
}
