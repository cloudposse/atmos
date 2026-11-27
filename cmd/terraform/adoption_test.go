package terraform

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestAdoptionPlan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX argument recorder")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_COMMAND", "")
	require.NoError(t, os.WriteFile("main.tf", nil, 0o600))
	bin := filepath.Join(dir, "native-cli")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$PWD\" \"$@\"\n"), 0o700))
	cmd := &cobra.Command{Use: "plan"}
	cmd.SetContext(context.Background())
	flags.RegisterComponentCommandFlags(cmd, "terraform")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("stack", "", "")
	cmd.Flags().AddFlagSet(cmd.PersistentFlags())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	require.NoError(t, cmd.ParseFlags([]string{"--command", bin}))
	orig := compat.GetSeparated()
	t.Cleanup(func() { compat.SetSeparated(orig) })
	compat.SetSeparated([]string{"-var-file=base vars.tfvars", "-var-file=dev.tfvars"})
	require.NoError(t, runAdoptionPlan(cmd, []string{"."}))
	require.Contains(t, out.String(), "plan\n-var-file=base vars.tfvars\n-var-file=dev.tfvars\n")
	require.ErrorContains(t, runAdoptionPlan(cmd, []string{".", "second"}), "one root")
	require.Error(t, runAdoptionPlan(cmd, []string{"./missing"}))
	require.NoError(t, cmd.Flags().Set("command", ""))
	require.Error(t, runAdoptionPlan(cmd, []string{"."}))
	require.NoError(t, cmd.Flags().Set("stack", "dev"))
	require.ErrorContains(t, runAdoptionPlan(cmd, []string{"."}), "--stack")
	require.False(t, isAdoptionPlan(nil))
	require.False(t, isAdoptionPlan([]string{"network/vpc"}))
}

func TestAdoptionPlanRequiresPolicySubject(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	policyDir := filepath.Join(root, "policy.d")
	require.NoError(t, os.Mkdir(policyDir, 0o700))
	t.Setenv("ATMOS_POLICY_DIR", policyDir)
	cmd := &cobra.Command{Use: "plan"}
	cmd.SetContext(t.Context())
	require.Error(t, runAdoptionPlan(cmd, []string{"."}))
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, "policy.yaml"), []byte("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: production}\nspec:\n  policies:\n    review:\n      on: terraform.plan\n      rule: 'true'\n      action: deny\n"), 0o600))
	require.ErrorContains(t, runAdoptionPlan(cmd, []string{"."}), "policies require a named component")
}

func TestAdoptionPlanDispatch(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	_, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile("main.tf", nil, 0o600))
	cmd := &cobra.Command{Use: "plan"}
	cmd.SetContext(t.Context())
	flags.RegisterComponentCommandFlags(cmd, "terraform")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("no-color", false, "")
	exe, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, cmd.ParseFlags([]string{"--command", exe, "--dry-run", "--no-color"}))
	args := []string{"."}
	require.True(t, isAdoptionPlan(args))
	require.NoError(t, planCmd.PreRunE(cmd, args))
	require.NoError(t, planCmd.RunE(cmd, args))
	require.NoError(t, planCmd.PostRunE(cmd, args))
	require.NoFileExists(t, "atmos.yaml")
}
