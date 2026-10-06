package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/reexec"
	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/version"
)

// resetEarlyChdir lets a test exercise the early --chdir handling that normally runs once per process.
func resetEarlyChdir(t *testing.T) {
	t.Helper()
	chdirProcessed = false
	t.Cleanup(func() { chdirProcessed = false })
}

func writeStandaloneScript(t *testing.T, dir, name, source string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))
	return path
}

func TestStandaloneScriptAcceptsLeadingGlobalFlags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    func(dir string) []string
		globals func(dir string) []string
	}{
		{
			name: "chdir with equals",
			args: func(dir string) []string {
				return []string{"atmos", "--chdir=" + dir, "tool.star", "one", "--chdir=script"}
			},
			globals: func(dir string) []string { return []string{"--chdir=" + dir} },
		},
		{
			name:    "chdir with a separate value",
			args:    func(dir string) []string { return []string{"atmos", "--chdir", dir, "./tool.star", "one"} },
			globals: func(dir string) []string { return []string{"--chdir", dir} },
		},
		{
			name:    "chdir shorthand",
			args:    func(dir string) []string { return []string{"atmos", "-C", dir, "./tool.star"} },
			globals: func(dir string) []string { return []string{"-C", dir} },
		},
		{
			name: "logs level with a separate value and a bool flag",
			args: func(dir string) []string {
				return []string{"atmos", "--logs-level", "Debug", "--no-color", "--chdir=" + dir, "tool.star"}
			},
			globals: func(dir string) []string { return []string{"--logs-level", "Debug", "--no-color", "--chdir=" + dir} },
		},
		{
			name: "double dash ends the global flags",
			args: func(dir string) []string {
				return []string{"atmos", "--chdir=" + dir, "--", "tool.star", "--chdir=script"}
			},
			globals: func(dir string) []string { return []string{"--chdir=" + dir} },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			NewTestKit(t)
			resetEarlyChdir(t)
			origin := t.TempDir()
			t.Chdir(origin) // Restores the working directory when the test ends.
			dir, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			writeStandaloneScript(t, dir, "tool.star", `print("ran")`)

			original := tc.args(dir)
			os.Args = original
			before := reflect.ValueOf(RootCmd.RunE).Pointer()
			restore, err := prepareStandaloneScript()
			require.NoError(t, err)
			require.NotNil(t, restore)

			assert.NotEqual(t, before, reflect.ValueOf(RootCmd.RunE).Pointer(), "the script runner replaces the root command")
			assert.Equal(t, append([]string{"atmos"}, tc.globals(dir)...), os.Args, "global flags stay for normal processing; the script and its arguments are removed")
			cwd, err := os.Getwd()
			require.NoError(t, err)
			resolved, err := filepath.EvalSymlinks(cwd)
			require.NoError(t, err)
			assert.Equal(t, dir, resolved, "the script path is resolved after --chdir")
			assert.Equal(t, original, reexec.Args(), "a re-exec forwards the original command line")

			restore()
			assert.Equal(t, original, os.Args)
			assert.Equal(t, before, reflect.ValueOf(RootCmd.RunE).Pointer())
			assert.Zero(t, reexec.ScriptArgs(), "the recorded script is cleared on restore")
		})
	}
}

func TestStandaloneScriptLeavesNonScriptsToNormalCommandHandling(t *testing.T) {
	NewTestKit(t)
	resetEarlyChdir(t)
	root := t.TempDir()
	t.Chdir(root)
	require.NoError(t, os.Mkdir(filepath.Join(root, "stacks"), 0o700))
	writeStandaloneScript(t, root, "notes.txt", "plain text")

	for _, args := range [][]string{
		{"atmos", "./stacks"},
		{"atmos", "./stacks/"},
		{"atmos", "--no-color", "./stacks"},
		{"atmos", "./missing/tool"},
		{"atmos", "./notes.txt"},
		{"atmos", "--bogus-flag", "tool.star"},
		{"atmos", "--chdir"},
		{"atmos", "terraform", "plan", "x.star"},
	} {
		os.Args = args
		before := reflect.ValueOf(RootCmd.RunE).Pointer()
		restore, err := prepareStandaloneScript()
		require.NoError(t, err, "%v", args)
		assert.Equal(t, args, os.Args, "%v", args)
		assert.Equal(t, before, reflect.ValueOf(RootCmd.RunE).Pointer(), "%v", args)
		restore()
	}
}

func TestStandaloneScriptStarDirectoryKeepsClearError(t *testing.T) {
	NewTestKit(t)
	resetEarlyChdir(t)
	root := t.TempDir()
	t.Chdir(root)
	require.NoError(t, os.Mkdir(filepath.Join(root, "tools.star"), 0o700))
	os.Args = []string{"atmos", "./tools.star"}
	restore, err := prepareStandaloneScript()
	require.ErrorContains(t, err, "must be a regular file")
	assert.Nil(t, restore)
	assert.Equal(t, []string{"atmos", "./tools.star"}, os.Args)
}

// Version switching reads its command line from DefaultReexecConfig. The script path and its
// arguments used to be dropped because prepareStandaloneScript had already replaced os.Args.
func TestStandaloneScriptReexecForwardsScriptAndArguments(t *testing.T) {
	NewTestKit(t)
	resetEarlyChdir(t)
	root := t.TempDir()
	t.Chdir(root)
	writeStandaloneScript(t, root, "v.star", `print("ran")`)
	original := []string{"atmos", "./v.star", "hello", "--use-version", "script-owned", "--chdir=script-owned"}
	os.Args = original
	restore, err := prepareStandaloneScript()
	require.NoError(t, err)
	defer restore()

	require.Equal(t, []string{"atmos"}, os.Args, "Atmos itself sees no script arguments")
	config := version.DefaultReexecConfig()
	assert.Equal(t, original, config.Args)
	assert.Equal(t, 5, config.ScriptArgs)
}

func TestStandaloneScriptRunsWithRelativeTracebackAndFileNameStep(t *testing.T) {
	NewTestKit(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Chdir(root)
	path := writeStandaloneScript(t, root, "boom.star", "def inner():\n    fail(\"boom\")\ninner()\n")
	command := &cobra.Command{}
	command.SetContext(context.Background())
	var runErr error
	captureStdoutStderr(t, func() {
		iolib.Reset()
		require.NoError(t, iolib.Initialize())
		data.InitWriter(iolib.GetContext())
		runErr = runStandaloneScript(command, &script.File{Path: path, Interpreter: "starlark"})
	})
	require.Error(t, runErr)
	assert.Contains(t, cockroach.FlattenDetails(runErr), "boom.star:2:", "tracebacks are relative to the working directory")
	assert.NotContains(t, cockroach.FlattenDetails(runErr), root, "the absolute path is not repeated")
	assert.Equal(t, root, standaloneProjectRoot())
}

func TestStandaloneScriptLogFieldsUseTheFileName(t *testing.T) {
	NewTestKit(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Chdir(root)
	var logs bytes.Buffer
	logger := log.New()
	logger.SetLevel(log.WarnLevel)
	logger.SetOutput(&logs)
	previous := log.Default()
	log.SetDefault(logger)
	t.Cleanup(func() { log.SetDefault(previous) })

	path := writeStandaloneScript(t, root, "logs.star", `log.warn("careful")`)
	command := &cobra.Command{}
	command.SetContext(context.Background())
	captureStdoutStderr(t, func() {
		iolib.Reset()
		require.NoError(t, iolib.Initialize())
		data.InitWriter(iolib.GetContext())
		require.NoError(t, runStandaloneScript(command, &script.File{Path: path, Interpreter: "starlark"}))
	})
	assert.Contains(t, logs.String(), "step=logs.star")
	assert.NotContains(t, logs.String(), root)
}
