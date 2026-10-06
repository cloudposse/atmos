package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels: a rename of a schema field these tests rely on fails the build.
var (
	_ = schema.CommandFlag{Type: "int", Default: 3}
	_ = schema.CommandArgument{Required: false, Default: ""}
)

// registerCustomCommandForTest registers command under a fresh parent and returns the registered
// cobra command, ready for PreRun/Run.
func registerCustomCommandForTest(t *testing.T, workDir string, command *schema.Command) *cobra.Command {
	t.Helper()

	parentCmd := &cobra.Command{Use: "atmos"}
	command.WorkingDirectory = workDir
	require.NoError(t, processCustomCommands(schema.AtmosConfiguration{BasePath: workDir}, []schema.Command{*command}, parentCmd))
	customCmd := findSubcommand(parentCmd, command.Name)
	require.NotNil(t, customCmd)
	return customCmd
}

// captureCustomCommandExit records an attempt to exit the process instead of exiting, and reports
// whether one happened.
func captureCustomCommandExit(t *testing.T) func() bool {
	t.Helper()

	var mu sync.Mutex
	exited := false
	original := errUtils.OsExit
	t.Cleanup(func() { errUtils.OsExit = original })
	errUtils.OsExit = func(int) {
		mu.Lock()
		defer mu.Unlock()
		exited = true
	}
	return func() bool {
		mu.Lock()
		defer mu.Unlock()
		return exited
	}
}

func TestCustomCommandArgumentsSurviveCommasEmptyValuesAndUnicode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "a comma inside a value does not split the arguments",
			args: []string{"api,v2", "us-east-1", "b"},
			want: "api,v2|us-east-1|b",
		},
		{
			name: "an empty string argument stays empty and does not shift the next one",
			args: []string{"api", "", "zone-b"},
			want: "api||zone-b",
		},
		{
			name: "unicode values with commas",
			args: []string{"ünï,cødé", "日本語,テスト", "z"},
			want: "ünï,cødé|日本語,テスト|z",
		},
		{
			name: "omitted optional argument is empty and the omitted default applies",
			args: []string{"api"},
			want: "api||z,1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)
			ensureIOInitialized(t)

			workDir := t.TempDir()
			customCmd := registerCustomCommandForTest(t, workDir, &schema.Command{
				Name: "args-roundtrip",
				Arguments: []schema.CommandArgument{
					{Name: "service", Required: true},
					{Name: "region"},
					{Name: "zone", Default: "z,1"},
				},
				Steps: []schema.Task{{
					Name:    "write",
					Type:    schema.TaskTypeShell,
					Command: `printf %s '{{ .Arguments.service }}|{{ .Arguments.region }}|{{ .Arguments.zone }}' > out.txt`,
				}},
			})

			customCmd.PreRun(customCmd, tt.args)
			customCmd.Run(customCmd, tt.args)

			got, err := os.ReadFile(filepath.Join(workDir, "out.txt"))
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestCustomCommandOptionalArgumentWithoutDefaultIsAllowed(t *testing.T) {
	_ = NewTestKit(t)
	ensureIOInitialized(t)
	exited := captureCustomCommandExit(t)

	workDir := t.TempDir()
	customCmd := registerCustomCommandForTest(t, workDir, &schema.Command{
		Name:      "optional-arg",
		Arguments: []schema.CommandArgument{{Name: "service", Required: true}, {Name: "region", Required: false}},
		Steps: []schema.Task{{
			Name:    "write",
			Type:    schema.TaskTypeShell,
			Command: `printf %s 'region=[{{ .Arguments.region }}]' > out.txt`,
		}},
	})

	customCmd.PreRun(customCmd, []string{"api"})
	customCmd.Run(customCmd, []string{"api"})

	assert.False(t, exited(), "an optional argument must not fail the command")
	got, err := os.ReadFile(filepath.Join(workDir, "out.txt"))
	require.NoError(t, err)
	assert.Equal(t, "region=[]", string(got))
}

func TestCustomCommandRequiredArgumentWithoutDefaultStillFails(t *testing.T) {
	_ = NewTestKit(t)
	ensureIOInitialized(t)
	exited := captureCustomCommandExit(t)

	customCmd := registerCustomCommandForTest(t, t.TempDir(), &schema.Command{
		Name:      "required-arg",
		Arguments: []schema.CommandArgument{{Name: "service", Required: true}},
		Steps:     []schema.Task{{Name: "noop", Type: schema.TaskTypeShell, Command: "true"}},
	})

	customCmd.PreRun(customCmd, nil)

	assert.True(t, exited(), "a missing required argument must stop the command")
}

func TestCustomCommandIntFlag(t *testing.T) {
	tests := []struct {
		name string
		set  string
		want string
	}{
		{name: "default", want: "3|int"},
		{name: "explicit value", set: "7", want: "7|int"},
		{name: "negative value", set: "-2", want: "-2|int"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)
			ensureIOInitialized(t)

			workDir := t.TempDir()
			customCmd := registerCustomCommandForTest(t, workDir, &schema.Command{
				Name: "int-flag",
				Flags: []schema.CommandFlag{
					{Name: "count", Type: "int", Default: 3, Description: "How many"},
				},
				Steps: []schema.Task{{
					Name:    "write",
					Type:    schema.TaskTypeShell,
					Command: `printf %s '{{ .Flags.count }}|{{ printf "%T" .Flags.count }}' > out.txt`,
				}},
			})

			flag := customCmd.PersistentFlags().Lookup("count")
			require.NotNil(t, flag)
			assert.Equal(t, "int", flag.Value.Type(), "an int flag must register as an int flag, not a string flag")
			assert.Equal(t, "3", flag.DefValue)
			if tt.set != "" {
				require.NoError(t, customCmd.PersistentFlags().Set("count", tt.set))
			}

			customCmd.PreRun(customCmd, nil)
			customCmd.Run(customCmd, nil)

			got, err := os.ReadFile(filepath.Join(workDir, "out.txt"))
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestCustomCommandIntFlagRejectsNonIntegerValue(t *testing.T) {
	_ = NewTestKit(t)

	customCmd := registerCustomCommandForTest(t, t.TempDir(), &schema.Command{
		Name:  "int-flag-invalid",
		Flags: []schema.CommandFlag{{Name: "count", Type: "int"}},
		Steps: []schema.Task{{Name: "noop", Type: schema.TaskTypeShell, Command: "true"}},
	})

	require.Error(t, customCmd.PersistentFlags().Set("count", "many"))
}

func TestCustomCommandRejectsUnsupportedFlagType(t *testing.T) {
	tests := []struct {
		name    string
		flag    schema.CommandFlag
		wantErr error
	}{
		{name: "unknown type", flag: schema.CommandFlag{Name: "ratio", Type: "float"}, wantErr: errUtils.ErrCustomCommandFlagType},
		{name: "non-numeric int default", flag: schema.CommandFlag{Name: "count", Type: "int", Default: "many"}, wantErr: errUtils.ErrCustomCommandFlagDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			parentCmd := &cobra.Command{Use: "atmos"}
			_, err := createCustomCommand(&schema.AtmosConfiguration{}, &schema.Command{
				Name:  "bad-flag",
				Flags: []schema.CommandFlag{tt.flag},
			}, parentCmd)
			require.ErrorIs(t, err, tt.wantErr)

			err = processCustomCommands(schema.AtmosConfiguration{}, []schema.Command{{
				Name:  "bad-flag",
				Flags: []schema.CommandFlag{tt.flag},
			}}, parentCmd)
			require.ErrorIs(t, err, tt.wantErr, "registration of the whole command set must fail loudly")
		})
	}
}

func TestCustomCommandScriptStepPassesAmbientEnvVerbatim(t *testing.T) {
	_ = NewTestKit(t)
	ensureIOInitialized(t)

	exe, err := os.Executable()
	require.NoError(t, err)
	workDir := t.TempDir()
	dumpFile := filepath.Join(workDir, "env-dump.txt")
	// The ambient process environment carries template syntax. It must reach the child exactly as
	// it is: not fail the step, and not be evaluated.
	t.Setenv("_ATMOS_TEST_DUMP_ENV", dumpFile)
	t.Setenv("AMBIENT_BAD", "{{bad")
	t.Setenv("AMBIENT_INJECT", `{{ "injected" }}`)

	customCmd := registerCustomCommandForTest(t, workDir, &schema.Command{
		Name: "script-ambient-env",
		Steps: []schema.Task{{
			Name:        "dump",
			Type:        schema.TaskTypeScript,
			Interpreter: "starlark",
			Env:         map[string]string{"EXE": exe, "DECLARED": `{{ "rendered" }}`},
			Script:      "exec.run([env[\"EXE\"]])\n",
		}},
	})

	customCmd.PreRun(customCmd, nil)
	customCmd.Run(customCmd, nil)

	dump, err := os.ReadFile(dumpFile)
	require.NoError(t, err)
	lines := strings.Split(string(dump), "\n")
	assert.Contains(t, lines, "AMBIENT_BAD={{bad")
	assert.Contains(t, lines, `AMBIENT_INJECT={{ "injected" }}`)
	assert.Contains(t, lines, "DECLARED=rendered", "declared step env values are still rendered")
}

func TestCustomCommandShellStepTimeout(t *testing.T) {
	_ = NewTestKit(t)
	ensureIOInitialized(t)
	exited := captureCustomCommandExit(t)

	exe, err := os.Executable()
	require.NoError(t, err)
	workDir := t.TempDir()
	t.Setenv("_ATMOS_TEST_SLEEP_MS", "20000")

	customCmd := registerCustomCommandForTest(t, workDir, &schema.Command{
		Name: "shell-timeout",
		Steps: []schema.Task{{
			Name:    "slow",
			Type:    schema.TaskTypeShell,
			Timeout: 500 * time.Millisecond,
			Env:     map[string]string{"EXE": exe},
			Command: `"$EXE"`,
		}},
	})

	start := time.Now()
	customCmd.PreRun(customCmd, nil)
	customCmd.Run(customCmd, nil)

	assert.True(t, exited(), "a step that outlives its timeout must fail the command")
	assert.Less(t, time.Since(start), 15*time.Second, "the timeout must cancel the running process")
}

// A step's `timeout:` bounds the whole step, retries and backoff included: the command fails when
// the timeout elapses instead of waiting out the backoff and running a second attempt.
func TestCustomCommandShellStepTimeoutBoundsRetries(t *testing.T) {
	_ = NewTestKit(t)
	ensureIOInitialized(t)
	exited := captureCustomCommandExit(t)

	exe, err := os.Executable()
	require.NoError(t, err)
	workDir := t.TempDir()

	maxAttempts := 3
	// The backoff is far longer than the timeout, so a retry loop that ignored the timeout would
	// still be waiting long after the timeout elapsed.
	delay := 5 * time.Second
	customCmd := registerCustomCommandForTest(t, workDir, &schema.Command{
		Name: "shell-timeout-retry",
		Steps: []schema.Task{{
			Name:    "flaky",
			Type:    schema.TaskTypeShell,
			Timeout: 500 * time.Millisecond,
			Retry:   &schema.RetryConfig{MaxAttempts: &maxAttempts, InitialDelay: &delay, BackoffStrategy: schema.BackoffConstant},
			Env:     map[string]string{"EXE": exe, "_ATMOS_TEST_EXIT_ONE": "1"},
			Command: `"$EXE"`,
		}},
	})

	start := time.Now()
	customCmd.PreRun(customCmd, nil)
	customCmd.Run(customCmd, nil)

	assert.True(t, exited(), "a step that fails every attempt must fail the command")
	assert.Less(t, time.Since(start), delay, "the timeout must end the step before the first backoff completes")
}

func TestCustomCommandParallelChildrenUseTheSameTemplateFunctionsAsSequentialSteps(t *testing.T) {
	_ = NewTestKit(t)
	ensureIOInitialized(t)
	exited := captureCustomCommandExit(t)

	workDir := t.TempDir()
	customCmd := registerCustomCommandForTest(t, workDir, &schema.Command{
		Name: "parallel-sprig",
		Steps: []schema.Task{
			{
				Name:    "sequential",
				Type:    schema.TaskTypeShell,
				Command: `printf %s '{{ "x" | upper }}' > sequential.txt`,
			},
			{
				Name: "group",
				Type: schema.TaskTypeParallel,
				Steps: []schema.WorkflowStep{{
					Name:    "child",
					Type:    schema.TaskTypeShell,
					Command: `printf %s '{{ "x" | upper }}' > parallel.txt`,
				}},
			},
		},
	})

	customCmd.PreRun(customCmd, nil)
	customCmd.Run(customCmd, nil)

	assert.False(t, exited(), "a Sprig function must work in a parallel child, as it does in a sequential step")
	for _, name := range []string{"sequential.txt", "parallel.txt"} {
		got, err := os.ReadFile(filepath.Join(workDir, name))
		require.NoError(t, err, name)
		assert.Equal(t, "X", string(got), name)
	}
}
