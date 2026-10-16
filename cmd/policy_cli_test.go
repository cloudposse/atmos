package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
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
	for _, args := range [][]string{{"policy", "check"}, {"policy", "list"}, {"policy", "schema", "function"}, {"policy", "test", cases}} {
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
	require.NotNil(t, RootCmd.PersistentFlags().Lookup("explain"))
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"terraform", "plan", "--explain"}, true},
		{[]string{"--explain=false"}, false},
		{[]string{"--", "--explain"}, false},
		{[]string{"--explain", "--explain=false"}, false},
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
