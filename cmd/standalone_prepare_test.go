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
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
		// scriptIndex is the position of the script path in args.
		scriptIndex int
	}{
		{
			name: "chdir with equals",
			args: func(dir string) []string {
				return []string{"atmos", "--chdir=" + dir, "tool.star", "one", "--chdir=script"}
			},
			globals:     func(dir string) []string { return []string{"--chdir=" + dir} },
			scriptIndex: 2,
		},
		{
			name:        "chdir with a separate value",
			args:        func(dir string) []string { return []string{"atmos", "--chdir", dir, "./tool.star", "one"} },
			globals:     func(dir string) []string { return []string{"--chdir", dir} },
			scriptIndex: 3,
		},
		{
			name:        "chdir shorthand",
			args:        func(dir string) []string { return []string{"atmos", "-C", dir, "./tool.star"} },
			globals:     func(dir string) []string { return []string{"-C", dir} },
			scriptIndex: 3,
		},
		{
			name: "logs level with a separate value and a bool flag",
			args: func(dir string) []string {
				return []string{"atmos", "--logs-level", "Debug", "--no-color", "--chdir=" + dir, "tool.star"}
			},
			globals:     func(dir string) []string { return []string{"--logs-level", "Debug", "--no-color", "--chdir=" + dir} },
			scriptIndex: 5,
		},
		{
			name: "double dash ends the global flags",
			args: func(dir string) []string {
				return []string{"atmos", "--chdir=" + dir, "--", "tool.star", "--chdir=script"}
			},
			globals:     func(dir string) []string { return []string{"--chdir=" + dir} },
			scriptIndex: 3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			NewTestKit(t)
			resetEarlyChdir(t)
			origin := t.TempDir()
			t.Chdir(origin) // Restores the working directory when the test ends.
			dir, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			// The script sits where the user stands; --chdir moves Atmos somewhere else.
			scriptPath := writeStandaloneScript(t, origin, "tool.star", `print("ran")`)

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
			assert.Equal(t, dir, resolved, "--chdir is applied")
			// A re-exec runs after --chdir was stripped, so it receives the path the user meant.
			wantReexec := append([]string(nil), original...)
			wantReexec[tc.scriptIndex] = scriptPath
			assert.Equal(t, wantReexec, reexec.Args(), "a re-exec forwards the command line with the script anchored where the user typed it")

			restore()
			assert.Equal(t, original, os.Args)
			assert.Equal(t, before, reflect.ValueOf(RootCmd.RunE).Pointer())
			assert.Zero(t, reexec.ScriptArgs(), "the recorded script is cleared on restore")
		})
	}
}

// A relative script path means what the user typed it against: the directory they stood in when
// they ran Atmos, before --chdir or ATMOS_CHDIR moved the process.
func TestStandaloneScriptRelativePathIgnoresChdir(t *testing.T) {
	for _, tc := range []struct {
		name string
		args func(dir string) []string
		env  func(t *testing.T, dir string)
	}{
		{name: "flag", args: func(dir string) []string { return []string{"atmos", "--chdir=" + dir, "./tool.star", "arg"} }},
		{
			name: "environment",
			args: func(string) []string { return []string{"atmos", "./tool.star", "arg"} },
			env:  func(t *testing.T, dir string) { t.Setenv("ATMOS_CHDIR", dir) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			NewTestKit(t)
			resetEarlyChdir(t)
			origin := t.TempDir()
			t.Chdir(origin)
			dir, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			// A same-named script in the target directory must not win over the one beside the user.
			writeStandaloneScript(t, dir, "tool.star", `print("wrong script")`)
			writeStandaloneScript(t, origin, "tool.star", `print("right script")`)
			if tc.env != nil {
				tc.env(t, dir)
			}
			os.Args = tc.args(dir)
			restore, err := prepareStandaloneScript()
			require.NoError(t, err)
			defer restore()

			cwd, err := os.Getwd()
			require.NoError(t, err)
			resolved, err := filepath.EvalSymlinks(cwd)
			require.NoError(t, err)
			assert.Equal(t, dir, resolved)
			command := &cobra.Command{}
			command.SetContext(context.Background())
			stdout, _ := captureStdoutStderr(t, func() {
				iolib.Reset()
				require.NoError(t, iolib.Initialize())
				data.InitWriter(iolib.GetContext())
				require.NoError(t, RootCmd.RunE(command, nil))
			})
			assert.Equal(t, "right script\n", stdout)
		})
	}

	t.Run("usage hints keep the spelling the user typed", func(t *testing.T) {
		NewTestKit(t)
		resetEarlyChdir(t)
		origin := t.TempDir()
		t.Chdir(origin)
		dir := t.TempDir()
		writeStandaloneScript(t, origin, "tool.star", `print("ran")`)
		os.Args = []string{"atmos", "--chdir=" + dir, "./tool.star"}
		file, err := detectStandaloneFile([]string{"--chdir=" + dir}, []string{"./tool.star"})
		require.NoError(t, err)
		require.NotNil(t, file)
		assert.Equal(t, "./tool.star", file.Invoked)
	})
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
		{"atmos", "--bogus-flag", "stacks"},
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

func TestStandaloneScriptNamesLeadingFlagMistakes(t *testing.T) {
	NewTestKit(t)
	resetEarlyChdir(t)
	root := t.TempDir()
	t.Chdir(root)
	writeStandaloneScript(t, root, "tool.star", `print("ran")`)

	for _, tc := range []struct {
		name string
		args []string
		want string
		hint string
	}{
		{"unknown flag", []string{"atmos", "--bogus-flag", "./tool.star", "api"}, `unknown flag "--bogus-flag"`, "belong to the script"},
		{"profile with a space", []string{"atmos", "--profile", "dev", "./tool.star"}, `"--profile dev" before a script path is ambiguous`, "Write --profile=dev."},
		{"identity with a space", []string{"atmos", "--identity", "dev", "./tool.star", "api"}, `"--identity dev" before a script path is ambiguous`, "Write --identity=dev."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			os.Args = tc.args
			restore, err := prepareStandaloneScript()
			require.ErrorIs(t, err, errUtils.ErrScriptUsage)
			assert.Nil(t, restore)
			assert.ErrorContains(t, err, tc.want)
			assert.Contains(t, cockroach.FlattenHints(err), tc.hint)
			assert.Equal(t, 2, errUtils.GetExitCode(err))
			assert.Equal(t, tc.args, os.Args, "a rejected command line is left untouched")
		})
	}

	for _, args := range [][]string{
		{"atmos", "--profile=dev", "./tool.star"},
		{"atmos", "--no-color", "./tool.star"},
		{"atmos", "--help", "./tool.star"},
		{"atmos", "--profile", "./tool.star"},
		{"atmos", "describe", "stacks", "./tool.star"},
	} {
		os.Args = args
		restore, err := prepareStandaloneScript()
		require.NoError(t, err, "%v", args)
		restore()
	}
}

func TestColorScanStopsAtTheScriptPath(t *testing.T) {
	NewTestKit(t)
	resetEarlyChdir(t)
	root := t.TempDir()
	t.Chdir(root)
	writeStandaloneScript(t, root, "tool.star", `print("ran")`)

	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"script flag after the path is not scanned", []string{"atmos", "./tool.star", "api", "--force-color"}, []string{}},
		{"global flag before the path is scanned", []string{"atmos", "--force-color", "./tool.star", "api"}, []string{"--force-color"}},
		{"global flag with a value is scanned", []string{"atmos", "--logs-level", "Debug", "--force-color=true", "./tool.star", "--force-color"}, []string{"--logs-level", "Debug", "--force-color=true"}},
		{"stdin selection stops the scan", []string{"atmos", "-", "--force-color"}, []string{}},
		{"other commands are scanned in full", []string{"atmos", "describe", "stacks", "--force-color"}, []string{"atmos", "describe", "stacks", "--force-color"}},
		{"no arguments", []string{"atmos"}, []string{"atmos"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, append([]string{}, colorScanArgs(tc.args)...))
		})
	}
}

func TestSetupColorProfileIgnoresScriptFlags(t *testing.T) {
	NewTestKit(t)
	resetEarlyChdir(t)
	root := t.TempDir()
	t.Chdir(root)
	writeStandaloneScript(t, root, "tool.star", `print("ran")`)
	viper.Set("force-color", false)
	t.Cleanup(func() { viper.Set("force-color", false) })

	t.Setenv("CLICOLOR_FORCE", "")
	require.NoError(t, os.Unsetenv("CLICOLOR_FORCE"))
	setupColorProfileFromEnvWithArgs([]string{"atmos", "./tool.star", "api", "--force-color"})
	_, forced := os.LookupEnv("CLICOLOR_FORCE")
	assert.False(t, forced, "a script's own --force-color must not enable Atmos color")

	t.Cleanup(func() { _ = os.Unsetenv("CLICOLOR_FORCE") })
	setupColorProfileFromEnvWithArgs([]string{"atmos", "--force-color", "./tool.star", "api"})
	assert.Equal(t, "1", os.Getenv("CLICOLOR_FORCE"), "an Atmos global flag before the path still applies")
}

func TestStandaloneSelectionEnvForwardsGlobalFlags(t *testing.T) {
	NewTestKit(t)
	t.Setenv("ATMOS_PROFILE", "")
	require.NoError(t, os.Unsetenv("ATMOS_PROFILE"))
	viper.Set("profile", []string{"dev", "stage"})
	t.Cleanup(func() { viper.Set("profile", nil) })
	cmd := &cobra.Command{Use: "atmos"}
	cmd.Flags().StringSlice("profile", nil, "")
	cmd.Flags().String("identity", "", "")
	require.NoError(t, cmd.Flags().Parse([]string{"--identity=admin"}))

	env := standaloneSelectionEnv(cmd)
	assert.Equal(t, "admin", env["ATMOS_IDENTITY"])
	assert.Equal(t, "dev,stage", env["ATMOS_PROFILE"], "the profiles chosen with --profile reach nested atmos calls")

	cmd = &cobra.Command{Use: "atmos"}
	cmd.Flags().StringSlice("profile", nil, "")
	cmd.Flags().String("identity", "", "")
	assert.NotContains(t, standaloneSelectionEnv(cmd), "ATMOS_IDENTITY", "an identity that was not set is not forwarded")

	viper.Set("profile", nil)
	assert.NotContains(t, standaloneSelectionEnv(cmd), "ATMOS_PROFILE", "no profile means no entry")
}
