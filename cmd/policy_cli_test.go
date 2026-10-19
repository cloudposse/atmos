package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/stretchr/testify/require"
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
