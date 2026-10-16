package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags/compat"
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

const destroyDenyManifest = "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: no-destroy}\nspec:\n  policies:\n    guard:\n      on: command\n      action: deny\n      rule: input.destroy\n"

func newDestroyAdmissionRoot(family, sub string) *cobra.Command {
	root := &cobra.Command{Use: "atmos", SilenceErrors: true, SilenceUsage: true}
	parent := &cobra.Command{Use: family}
	parent.AddCommand(&cobra.Command{
		Use: sub, Run: func(*cobra.Command, []string) {}, Args: cobra.ArbitraryArgs,
		FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	})
	root.AddCommand(parent)
	return root
}

func TestCommandAdmissionDestroyInput(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deny.yaml"), []byte(destroyDenyManifest), 0o600))
	tests := []struct {
		name   string
		family string
		args   []string
		denied bool
	}{
		{name: "destroy command", family: "terraform", args: []string{"terraform", "destroy", "vpc", "-s", "dev"}, denied: true},
		{name: "apply -destroy", family: "terraform", args: []string{"terraform", "apply", "vpc", "-s", "dev", "-destroy", "-auto-approve"}, denied: true},
		{name: "apply --destroy=true", family: "terraform", args: []string{"terraform", "apply", "vpc", "--destroy=true"}, denied: true},
		{name: "apply after separator", family: "terraform", args: []string{"terraform", "apply", "vpc", "--", "-destroy"}, denied: true},
		{name: "plan -destroy", family: "terraform", args: []string{"terraform", "plan", "vpc", "-destroy"}, denied: true},
		{name: "deploy -destroy", family: "terraform", args: []string{"terraform", "deploy", "vpc", "-destroy"}, denied: true},
		{name: "apply -destroy=false", family: "terraform", args: []string{"terraform", "apply", "vpc", "-destroy=false", "-auto-approve"}},
		{name: "plain plan", family: "terraform", args: []string{"terraform", "plan", "vpc", "-s", "dev"}},
		{name: "helmfile destroy", family: "helmfile", args: []string{"helmfile", "destroy", "app"}, denied: true},
		{name: "helmfile apply with destroy flag", family: "helmfile", args: []string{"helmfile", "apply", "app", "-destroy"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newDestroyAdmissionRoot(tc.family, tc.args[1])
			err := CheckEarlyCommandAdmission(root, tc.args)
			if tc.denied {
				require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCommandAdmissionDestroyInputDuringExecution(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deny.yaml"), []byte(destroyDenyManifest), 0o600))
	t.Cleanup(compat.ResetSeparated)

	t.Run("pass-through flags kept aside by the compatibility translator", func(t *testing.T) {
		compat.SetSeparated([]string{"-destroy", "-auto-approve"})
		root := newDestroyAdmissionRoot("terraform", "apply")
		root.SetArgs([]string{"terraform", "apply", "vpc"})
		_, err := Execute(root)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	})

	t.Run("separated false value", func(t *testing.T) {
		compat.SetSeparated([]string{"-destroy=false"})
		root := newDestroyAdmissionRoot("terraform", "apply")
		root.SetArgs([]string{"terraform", "apply", "vpc"})
		_, err := Execute(root)
		require.NoError(t, err)
	})

	t.Run("no separated flags", func(t *testing.T) {
		compat.ResetSeparated()
		root := newDestroyAdmissionRoot("terraform", "apply")
		root.SetArgs([]string{"terraform", "apply", "vpc"})
		_, err := Execute(root)
		require.NoError(t, err)
	})

	t.Run("destroy command", func(t *testing.T) {
		compat.ResetSeparated()
		root := newDestroyAdmissionRoot("terraform", "destroy")
		root.SetArgs([]string{"terraform", "destroy", "vpc"})
		_, err := Execute(root)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	})

	t.Run("positional after separator", func(t *testing.T) {
		compat.ResetSeparated()
		root := newDestroyAdmissionRoot("terraform", "apply")
		root.SetArgs([]string{"terraform", "apply", "vpc", "--", "-destroy"})
		_, err := Execute(root)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	})
}
