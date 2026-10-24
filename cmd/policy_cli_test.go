package cmd

import (
	"bytes"
	stdio "io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	githubci "github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/flags"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestPolicyCLIIsOffline(t *testing.T) {
	_ = NewTestKit(t)
	root := t.TempDir()
	t.Chdir(root)
	directory := filepath.Join(root, "policy.d")
	require.NoError(t, os.Mkdir(directory, 0o700))
	t.Setenv("ATMOS_POLICY_DIR", directory)
	manifest := `apiVersion: atmos/v1
kind: Policy
metadata: {name: commands}
spec:
  policies:
    deny:
      on: command
      action: deny
      rule: "true"
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))
	// Loading this file would fail: authoring commands must not evaluate project configuration.
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte("broken: !not-a-valid-function value\n"), 0o600))
	cases := filepath.Join(root, "cases.json")
	require.NoError(t, os.WriteFile(cases, []byte(`[{"name":"deny","request":{"kind":"command","input":{}},"status":"deny"}]`), 0o600))
	for _, args := range [][]string{{"policy", "check"}, {"policy", "check", "--format", "json"}, {"policy", "list"}, {"policy", "schema", "function"}, {"policy", "test", cases}} {
		t.Run(args[1], func(t *testing.T) {
			_ = NewTestKit(t)
			os.Args = append([]string{"atmos"}, args...)
			RootCmd.SetArgs(args)
			require.NoError(t, Execute())
		})
	}
	t.Run("invalid policy", func(t *testing.T) {
		_ = NewTestKit(t)
		require.NoError(t, os.WriteFile(filepath.Join(directory, "bad.yaml"), []byte("kind: Nope"), 0o600))
		os.Args = []string{"atmos", "policy", "check"}
		RootCmd.SetArgs(os.Args[1:])
		require.Error(t, Execute())
	})
}

func TestPolicyExplainArgument(t *testing.T) {
	_ = NewTestKit(t)
	// The arguments alone decide each case, whatever the caller's environment holds.
	t.Setenv("ATMOS_EXPLAIN", "false")
	require.NotNil(t, RootCmd.PersistentFlags().Lookup("explain"))
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"terraform", "plan", "--explain"}, true},
		{[]string{"--explain=false"}, false},
		{[]string{"--", "--explain"}, false},
		{[]string{"--explain", "--explain=false"}, false},
		{[]string{"terraform", "plan", "--explain", "false"}, false},
		{[]string{"--explain", "true"}, true},
		{[]string{"--explain", "FALSE", "--", "--explain"}, false},
		{[]string{"terraform", "plan", "vpc", "--explain", "stack-name"}, true},
		{[]string{"--explain", "false", "--explain"}, true},
	} {
		require.Equal(t, tc.want, policyExplainRequested(tc.args))
	}
}

func TestPolicyExplainUsesStderrAndStillDenies(t *testing.T) {
	_ = NewTestKit(t)
	directory := t.TempDir()
	t.Chdir(directory)
	t.Setenv("ATMOS_POLICY_DIR", directory)
	manifest := `apiVersion: atmos/v1
kind: Policy
metadata: {name: trace}
spec:
  policies:
    deny:
      on: command
      action: deny
      rule: "true"
    future:
      on: terraform.plan
      action: deny
      rule: "true"
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))
	os.Args = []string{"atmos", "version", "--explain"}
	RootCmd.SetArgs(os.Args[1:])
	stdout, stderr := captureStdoutStderr(t, func() {
		ui.ReinitFormatter()
		require.Error(t, Execute())
	})
	ui.ReinitFormatter()
	require.Empty(t, stdout)
	require.Contains(t, stderr, "command trace/deny: deny")
	require.Contains(t, stderr, "terraform.plan")
	require.Contains(t, stderr, "not reached; no verdict")
}

func TestPolicyExplainRequestedReadsEnvironment(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  string
		args []string
		want bool
	}{
		{"env enables it", "true", []string{"terraform", "plan"}, true},
		{"env disabled", "false", []string{"terraform", "plan"}, false},
		{"flag overrides a disabled env", "false", []string{"--explain"}, true},
		{"flag overrides an enabled env", "true", []string{"--explain=false"}, false},
		{"env is ignored after the argument terminator", "false", []string{"--", "--explain"}, false},
		{"unparsable env means off", "maybe", []string{"version"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(flags.ExplainEnvVar, tc.env)
			assert.Equal(t, tc.want, policyExplainRequested(tc.args))
		})
	}
}

func TestExportPolicyExplanationRestoresPreviousValue(t *testing.T) {
	t.Run("sets and unsets when it was absent", func(t *testing.T) {
		t.Setenv(flags.ExplainEnvVar, "")
		require.NoError(t, os.Unsetenv(flags.ExplainEnvVar))
		restore := exportPolicyExplanation(true)
		assert.Equal(t, "true", os.Getenv(flags.ExplainEnvVar))
		restore()
		_, found := os.LookupEnv(flags.ExplainEnvVar)
		assert.False(t, found)
	})
	t.Run("disabled and absent leaves the environment alone", func(t *testing.T) {
		t.Setenv(flags.ExplainEnvVar, "")
		require.NoError(t, os.Unsetenv(flags.ExplainEnvVar))
		exportPolicyExplanation(false)()
		_, found := os.LookupEnv(flags.ExplainEnvVar)
		assert.False(t, found)
	})
	t.Run("an explicit flag off reaches children even when env enables it", func(t *testing.T) {
		t.Setenv(flags.ExplainEnvVar, "true")
		restore := exportPolicyExplanation(false)
		assert.Equal(t, "false", os.Getenv(flags.ExplainEnvVar))
		restore()
		assert.Equal(t, "true", os.Getenv(flags.ExplainEnvVar))
	})
}

func TestBeginPolicyExplanationForwardsToNestedProcesses(t *testing.T) {
	_ = NewTestKit(t)
	directory := t.TempDir()
	t.Chdir(directory)
	t.Setenv("ATMOS_POLICY_DIR", directory)
	manifest := `apiVersion: atmos/v1
kind: Policy
metadata: {name: trace}
spec:
  policies:
    deny:
      on: command
      action: deny
      rule: "true"
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))
	t.Setenv(flags.ExplainEnvVar, "")
	require.NoError(t, os.Unsetenv(flags.ExplainEnvVar))

	restore, err := beginPolicyExplanation([]string{"gov", "destroy", "--explain"})
	require.NoError(t, err)
	// A nested atmos started by a script or step inherits os.Environ(), so it explains as well.
	assert.True(t, slices.Contains(os.Environ(), flags.ExplainEnvVar+"=true"))
	restore()
	_, found := os.LookupEnv(flags.ExplainEnvVar)
	assert.False(t, found, "the environment is restored when the invocation ends")

	restore, err = beginPolicyExplanation([]string{"gov", "destroy"})
	require.NoError(t, err)
	_, found = os.LookupEnv(flags.ExplainEnvVar)
	assert.False(t, found, "no explain request means nothing is forwarded")
	restore()
}

func TestPolicyExplainEnvironmentIsTheDefaultAndArgumentsWin(t *testing.T) {
	_ = NewTestKit(t)
	t.Setenv("ATMOS_EXPLAIN", "true")
	require.True(t, policyExplainRequested([]string{"terraform", "plan"}), "a nested process inherits the parent's setting")
	require.False(t, policyExplainRequested([]string{"terraform", "plan", "--explain=false"}), "the command line wins")
	require.False(t, policyExplainRequested([]string{"terraform", "plan", "--explain", "false"}), "a literal value wins")
	t.Setenv("ATMOS_EXPLAIN", "false")
	require.True(t, policyExplainRequested([]string{"terraform", "plan", "--explain"}))
}

// annotationStreams captures the UI channel so a test can read CI annotations.
type annotationStreams struct{ stderr *bytes.Buffer }

func (s *annotationStreams) Input() stdio.Reader     { return &bytes.Buffer{} }
func (s *annotationStreams) Output() stdio.Writer    { return stdio.Discard }
func (s *annotationStreams) Error() stdio.Writer     { return s.stderr }
func (s *annotationStreams) RawOutput() stdio.Writer { return stdio.Discard }
func (s *annotationStreams) RawError() stdio.Writer  { return s.stderr }

// A command denied before configuration loads still reports one annotation per
// denying rule through the detected CI provider, and only once. The main function
// reports the error returned by Execute through AnnotateFailure.
func TestExecuteAnnotatesPolicyDenialInCI(t *testing.T) {
	_ = NewTestKit(t)
	root := t.TempDir()
	t.Chdir(root)
	directory := filepath.Join(root, "policy.d")
	require.NoError(t, os.Mkdir(directory, 0o700))
	t.Setenv("ATMOS_POLICY_DIR", directory)
	t.Setenv("NO_COLOR", "1")
	manifest := "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: no-about}\nspec:\n  policies:\n" +
		"    guard:\n      on: command\n      action: deny\n      reason: \"**About** is disabled here.\"\n      rule: input.command == 'about'\n"
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))

	restore := ci.SwapRegistryForTest()
	t.Cleanup(restore)
	ci.Register(githubci.NewProvider())

	run := func(t *testing.T, githubActions string) (string, error) {
		t.Helper()
		// The kit snapshots os.Args and RootCmd state before this run changes them, and its cleanup restores
		// both and re-initializes the default UI formatter, so nothing here leaks into later tests.
		_ = NewTestKit(t)
		t.Setenv("GITHUB_ACTIONS", githubActions)
		stderr := &bytes.Buffer{}
		ioCtx, err := iolib.NewContext(iolib.WithStreams(&annotationStreams{stderr: stderr}))
		require.NoError(t, err)
		ui.InitFormatter(ioCtx)
		os.Args = []string{"atmos", "about"}
		RootCmd.SetArgs(os.Args[1:])
		executeErr := Execute()
		AnnotateFailure(executeErr)
		return stderr.String(), executeErr
	}

	t.Run("in GitHub Actions", func(t *testing.T) {
		output, err := run(t, "true")
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		var annotations []string
		for _, line := range strings.Split(output, "\n") {
			if strings.HasPrefix(line, "::") {
				annotations = append(annotations, line)
			}
		}
		require.Equal(t, []string{"::error title=Policy no-about/guard denied execution::About is disabled here."}, annotations)
	})

	t.Run("outside CI", func(t *testing.T) {
		output, err := run(t, "")
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		require.NotContains(t, output, "::error")
	})
}

// Commands that end the process through CheckErrorPrintAndExit never return to main, so the
// registered failure reporter annotates them once before the process exits.
func TestCheckErrorPrintAndExitAnnotatesInCI(t *testing.T) {
	_ = NewTestKit(t)
	restore := ci.SwapRegistryForTest()
	t.Cleanup(restore)
	ci.Register(githubci.NewProvider())
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("NO_COLOR", "1")

	stderr := &bytes.Buffer{}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(&annotationStreams{stderr: stderr}))
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)

	var exitCodes []int
	previousExit := errUtils.OsExit
	errUtils.OsExit = func(code int) { exitCodes = append(exitCodes, code) }
	t.Cleanup(func() { errUtils.OsExit = previousExit })

	errUtils.CheckErrorPrintAndExit(errUtils.ErrPolicyDenied, "", "")

	var annotations []string
	for _, line := range strings.Split(stderr.String(), "\n") {
		if strings.HasPrefix(line, "::") {
			annotations = append(annotations, line)
		}
	}
	require.Len(t, annotations, 1, stderr.String())
	require.True(t, strings.HasPrefix(annotations[0], "::error"), annotations[0])
	require.Len(t, exitCodes, 1)
}
