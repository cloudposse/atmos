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

func TestCommandAdmissionPreservesDefaultArgumentValidation(t *testing.T) {
	t.Setenv("ATMOS_POLICY_DIR", "")
	t.Chdir(t.TempDir())
	for _, tc := range []struct {
		name      string
		children  bool
		validator cobra.PositionalArgs
		args      []string
		wantError bool
	}{
		{name: "unknown command", children: true, args: []string{"non-existent"}, wantError: true},
		{name: "root without arguments", children: true},
		{name: "root with explicit positional arguments", children: true, validator: cobra.ArbitraryArgs, args: []string{"positional"}},
		{name: "standalone root", args: []string{"positional"}},
		{name: "leaf positional arguments", children: true, args: []string{"known", "positional"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ran := false
			preRun := false
			usage := false
			root := &cobra.Command{
				Use: "atmos", Args: tc.validator, SilenceErrors: true,
				PersistentPreRun: func(*cobra.Command, []string) { preRun = true },
				Run:              func(*cobra.Command, []string) { ran = true },
			}
			root.SetUsageFunc(func(*cobra.Command) error { usage = true; return nil })
			if tc.children {
				root.AddCommand(&cobra.Command{Use: "known", Run: func(*cobra.Command, []string) { ran = true }})
			}
			root.SetArgs(tc.args)
			_, err := Execute(root)
			if tc.wantError {
				require.ErrorIs(t, err, errUtils.ErrUnknownSubcommand)
				require.False(t, preRun)
				require.False(t, ran)
				require.False(t, usage, "unknown commands must fail during Find, before usage rendering")
			} else {
				require.NoError(t, err)
				require.True(t, ran)
				require.True(t, preRun, "preserve inherited pre-run callbacks")
			}
			if tc.validator == nil {
				require.Nil(t, root.Args, "restore Cobra's default validation after execution")
			}
			require.Nil(t, root.PersistentPreRunE, "restore original persistent callbacks")
		})
	}
}

func TestRootCommandAdmissionBeforeHooks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deny.yaml"), []byte("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: safety}\nspec:\n  policies:\n    root:\n      on: command\n      action: deny\n      rule: input.command == 'atmos'\n"), 0o600))
	for _, errorHook := range []bool{false, true} {
		t.Run(map[bool]string{false: "PersistentPreRun", true: "PersistentPreRunE"}[errorHook], func(t *testing.T) {
			ran, plainHookRan, errorHookRan := false, false, false
			root := &cobra.Command{
				Use: "atmos", SilenceErrors: true, SilenceUsage: true,
				PersistentPreRun: func(*cobra.Command, []string) { plainHookRan = true },
				Run:              func(*cobra.Command, []string) { ran = true },
			}
			if errorHook {
				root.PersistentPreRunE = func(*cobra.Command, []string) error { errorHookRan = true; return nil }
			}
			root.AddCommand(&cobra.Command{Use: "known", Run: func(*cobra.Command, []string) { ran = true }})
			root.SetArgs([]string{})
			_, err := Execute(root)
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			require.False(t, ran)
			require.False(t, plainHookRan)
			require.False(t, errorHookRan)
			require.Nil(t, root.Args)

			root.SetArgs([]string{"known"})
			_, err = Execute(root)
			require.NoError(t, err)
			require.True(t, ran)
			require.Equal(t, errorHook, errorHookRan)
			require.Equal(t, !errorHook, plainHookRan, "retain Cobra's PersistentPreRunE precedence")
		})
	}
}
