package internal

import (
	"os"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestCommandAdmissionBeforeHooks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deny.yaml"), []byte("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: safety}\nspec:\n  policies:\n    deploy:\n      on: command\n      action: deny\n      rule: input.path[1] == 'deploy'\n"), 0o600))
	for _, kind := range []string{"terraform", "helm", "helmfile", "kubernetes", "packer", "custom"} {
		t.Run(kind, func(t *testing.T) {
			run := false
			root := &cobra.Command{Use: "atmos", SilenceErrors: true, SilenceUsage: true}
			parent := &cobra.Command{Use: kind}
			cmd := &cobra.Command{Use: "deploy", Aliases: []string{"go"}, PersistentPreRun: func(*cobra.Command, []string) { run = true }, Run: func(*cobra.Command, []string) { run = true }}
			parent.AddCommand(cmd)
			root.AddCommand(parent)
			root.SetArgs([]string{kind, "go"})
			require.ErrorIs(t, CheckEarlyCommandAdmission(root, []string{kind, "go"}), errUtils.ErrPolicyDenied)
			_, err := Execute(root)
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			require.False(t, run, "no --affected flag needed; pre-run must not execute")
			require.Nil(t, cmd.Args, "restore original callbacks")
			for _, flag := range []string{"--help", "-h", "--help=true"} {
				require.NoError(t, CheckEarlyCommandAdmission(root, []string{kind, "go", flag}))
				root.SetArgs([]string{kind, "go", flag})
				_, err := Execute(root)
				require.NoError(t, err)
				require.False(t, run, "help must not execute the command")
			}
			require.ErrorIs(t, CheckEarlyCommandAdmission(root, []string{kind, "go", "--help=false"}), errUtils.ErrPolicyDenied)
			require.ErrorIs(t, CheckEarlyCommandAdmission(root, []string{kind, "go", "--", "-h"}), errUtils.ErrPolicyDenied)
		})
	}
}
