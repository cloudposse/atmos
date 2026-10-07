package cmd

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
	starlarkengine "github.com/cloudposse/atmos/pkg/script/starlark"
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
	require.ErrorIs(t, err, errUtils.ErrScript)
	assert.Nil(t, restore)
	assert.Equal(t, before, reflect.ValueOf(RootCmd.RunE).Pointer())
	assert.Equal(t, []string{"atmos", path}, os.Args)

	// A script can disappear after detection but before execution.
	command := &cobra.Command{}
	command.SetContext(context.Background())
	err = runStandaloneScript(command, &script.File{Path: path, Interpreter: "starlark"})
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorIs(t, err, errUtils.ErrScript)
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

			cfg.DisableMetricsSummaryByDefault(&config)

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
		require.NoError(t, runStandaloneScript(command, &script.File{Path: path, Interpreter: "starlark"}))
	})

	require.NotNil(t, atmosConfig.Settings.Metrics.Enabled)
	assert.False(t, *atmosConfig.Settings.Metrics.Enabled)
}

func TestStandaloneScriptUsesRegisteredExtension(t *testing.T) {
	NewTestKit(t)
	engine := starlarkengine.New(starlarkengine.WithReadFile(func(string) ([]byte, error) {
		return []byte(`message = "selected registered engine"`), nil
	}))
	script.Register("standalone-extension-test", engine, script.WithExtensions(".host-test"))
	path := filepath.Join(t.TempDir(), "release.host-test")
	require.NoError(t, os.WriteFile(path, []byte("load(\"virtual.star\", \"message\")\noutput = message"), 0o600))
	os.Args = []string{"atmos", path}
	restore, err := prepareStandaloneScript()
	require.NoError(t, err)
	defer restore()
	command := &cobra.Command{}
	command.SetContext(t.Context())
	stdout, stderr := captureStdoutStderr(t, func() {
		iolib.Reset()
		require.NoError(t, iolib.Initialize())
		data.InitWriter(iolib.GetContext())
		err = RootCmd.RunE(command, nil)
	})
	require.NoError(t, err)
	assert.Equal(t, "selected registered engine\n", stdout)
	assert.Empty(t, stderr)
	err = runStandaloneScript(command, &script.File{Path: path, Interpreter: "unregistered"})
	require.ErrorIs(t, err, errUtils.ErrScript)
	assert.ErrorContains(t, err, `"unregistered" is unavailable`)
	assert.NotErrorIs(t, err, errUtils.ErrStarlark)
}

func TestStandaloneStdinExecution(t *testing.T) {
	for _, tc := range []struct {
		name, source, want, errorText string
		args                          []string
	}{
		{name: "arguments", source: `output = [ctx.args, ctx.script == None]`, args: []string{"-", "foo", "bar"}, want: "[[\"foo\",\"bar\"],true]\n"},
		{name: "separator", source: `output = ctx.args`, args: []string{"-", "--", "foo", "bar"}, want: "[\"foo\",\"bar\"]\n"},
		{name: "script flags", source: `output = ctx.args`, args: []string{"-", "--help", "--chdir=script-owned", "--interpreter=script-owned"}, want: "[\"--help\",\"--chdir=script-owned\",\"--interpreter=script-owned\"]\n"},
		{name: "load from working directory", source: "load(\"helper.star\", \"message\")\noutput = message", args: []string{"--interpreter=starlark", "-"}, want: "loaded from cwd\n"},
		{name: "globals before stdin", source: `output = ctx.args`, args: []string{"--logs-level", "Off", "--interpreter=starlark", "-", "--chdir=script-owned"}, want: "[\"--chdir=script-owned\"]\n"},
		{name: "traceback", source: `fail("stdin failure")`, args: []string{"-"}, errorText: "stdin failure"},
		{name: "empty source", args: []string{"-"}},
		{name: "parsed arguments", source: "def main(args, flags):\n    print(args[\"name\"])\ncli.command(main, args=[cli.arg(\"name\")])", args: []string{"-", "--", "hello"}, want: "hello\n"},
		{name: "missing required argument", source: "def main(args, flags):\n    fail(\"must not run\")\ncli.command(main, args=[cli.arg(\"name\")])", args: []string{"-"}, errorText: "missing required argument `<name>`"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			NewTestKit(t)
			dir := t.TempDir()
			t.Chdir(dir)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "helper.star"), []byte(`message = "loaded from cwd"`), 0o600))
			setStandaloneTestStdin(t, tc.source)
			os.Args = append([]string{"atmos"}, tc.args...)
			restore, err := prepareStandaloneScript()
			require.NoError(t, err)
			defer restore()
			command := &cobra.Command{}
			command.SetContext(t.Context())
			stdout, stderr := captureStdoutStderr(t, func() {
				iolib.Reset()
				require.NoError(t, iolib.Initialize())
				data.InitWriter(iolib.GetContext())
				err = RootCmd.RunE(command, nil)
			})
			if tc.errorText != "" {
				require.ErrorContains(t, err, tc.errorText)
				if tc.name == "traceback" {
					assert.Contains(t, strings.Join(cockroach.GetAllDetails(err), "\n"), "<stdin>:1:5")
				}
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, stdout)
			assert.Empty(t, stderr)
		})
	}
}

func TestStandaloneStdinHelpAndInterpreter(t *testing.T) {
	NewTestKit(t)
	script.Register("stdin-test-engine", starlarkengine.New(starlarkengine.WithReadFile(func(string) ([]byte, error) {
		return []byte(`description = "registered stdin interpreter"`), nil
	})))
	setStandaloneTestStdin(t, "load(\"virtual.star\", \"description\")\ndef main(args, flags):\n    fail(\"must not run\")\ncli.command(main, description=description)")
	os.Args = []string{"atmos", "--interpreter", "stdin-test-engine", "-", "--help"}
	restore, err := prepareStandaloneScript()
	require.NoError(t, err)
	defer restore()
	command := &cobra.Command{}
	command.SetContext(t.Context())
	stdout, stderr := captureStdoutStderr(t, func() {
		iolib.Reset()
		require.NoError(t, iolib.Initialize())
		data.InitWriter(iolib.GetContext())
		err = RootCmd.RunE(command, nil)
	})
	require.NoError(t, err)
	assert.Contains(t, stdout, "registered stdin interpreter")
	assert.Contains(t, stdout, "stdin [flags]")
	assert.NotContains(t, stdout, "must not run")
	assert.Empty(t, stderr)
}

func TestStandaloneStdinReadFailure(t *testing.T) {
	NewTestKit(t)
	setStandaloneTestStdin(t, `fail("must not run")`)
	require.NoError(t, os.Stdin.Close())
	command := &cobra.Command{}
	command.SetContext(t.Context())
	iolib.Reset()
	require.NoError(t, iolib.Initialize())
	err := runStandaloneScript(command, &script.File{Path: "-", Stdin: true, Interpreter: "starlark"})
	require.ErrorIs(t, err, errUtils.ErrScript)
	require.ErrorIs(t, err, os.ErrClosed)
	assert.ErrorContains(t, err, "read script")
	assert.NotContains(t, err.Error(), "must not run")
}

func setStandaloneTestStdin(t *testing.T, source string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input")
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))
	input, err := os.Open(path)
	require.NoError(t, err)
	previous := os.Stdin
	os.Stdin = input
	t.Cleanup(func() { os.Stdin = previous; _ = input.Close(); iolib.Reset() })
}
