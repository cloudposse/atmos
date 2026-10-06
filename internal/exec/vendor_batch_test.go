package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/vendoring/concurrency"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
)

func vendorBatchPlanFixture(t *testing.T) (*cobra.Command, string) {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("ATMOS_BASE_PATH", root)
	t.Setenv(concurrency.Env, "")
	require.NoError(t, os.Unsetenv(concurrency.Env))
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte("base_path: .\nedition: '2026-09-14'\n"), 0o644))
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "main.tf"), []byte("# prepared only when executed\n"), 0o644))
	target := filepath.Join(root, "components", "terraform", "vpc")
	manifest := "apiVersion: atmos/v1\nkind: AtmosVendorConfig\nspec:\n  sources:\n    - component: vpc\n      source: '" + filepath.ToSlash(source) + "'\n      targets: [components/terraform/vpc]\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "vendor.yaml"), []byte(manifest), 0o644))
	cmd := newTestCommandWithGlobalFlags("pull")
	cmd.Flags().AddFlagSet(newVendorPullFlagSet(true))
	cmd.Flags().String("type", "terraform", "")
	cmd.Flags().Int(concurrency.Flag, 0, "")
	cmd.SetContext(context.Background())
	return cmd, target
}

func TestPlanVendorPullNoWritesAndPrecedence(t *testing.T) {
	for _, test := range []struct {
		name, env, flag string
		want            int
		invalid         bool
	}{
		{name: "edition default", want: 1},
		{name: "environment", env: "3", want: 3},
		{name: "flag overrides environment", env: "3", flag: "2", want: 2},
		{name: "invalid environment", env: "many", invalid: true},
		{name: "zero flag", flag: "0", invalid: true},
		{name: "negative flag", flag: "-1", invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd, target := vendorBatchPlanFixture(t)
			if test.env != "" {
				t.Setenv(concurrency.Env, test.env)
			}
			if test.flag != "" {
				require.NoError(t, cmd.Flags().Set(concurrency.Flag, test.flag))
			}
			plan, err := PlanVendorPull(cmd, nil)
			if test.invalid {
				require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
				assert.Nil(t, plan)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.want, plan.Options.MaxConcurrency)
				assert.Equal(t, test.want, plan.Config.Vendor.MaxConcurrency)
				require.Len(t, plan.Packages, 1)
				assert.Equal(t, "vpc", plan.Packages[0].Name)
				assert.Equal(t, target, plan.Packages[0].Target())
			}
			assert.NoDirExists(t, target, "planning must not create target directories")
			assert.NoFileExists(t, "vendor.lock.yaml", "planning must not write receipts")
		})
	}
}

func TestExecuteVendorPackagesSummaryAndCancellation(t *testing.T) {
	for _, mode := range []string{"install", "dry-run", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			cmd, target := vendorBatchPlanFixture(t)
			stderr, cleanup := setupVendorModelTestUI(t)
			defer cleanup()
			plan, err := PlanVendorPull(cmd, nil)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			plan.Options.DryRun = mode == "dry-run"
			err = ExecuteVendorPackages(ctx, &plan.Config, plan.Packages, plan.Options)
			if mode == "cancel" {
				require.ErrorIs(t, err, context.Canceled)
				require.ErrorIs(t, err, ErrVendorComponents)
			} else {
				require.NoError(t, err)
			}
			switch mode {
			case "install":
				content, readErr := os.ReadFile(filepath.Join(target, "main.tf"))
				require.NoError(t, readErr)
				assert.Equal(t, "# prepared only when executed\n", string(content))
				assert.Contains(t, stderr.String(), "Vendored 1 packages, 0 unchanged, 0 failed, 0 canceled")
				stderr.Reset()
				require.NoError(t, ExecuteVendorPackages(context.Background(), &plan.Config, plan.Packages, plan.Options))
				assert.Contains(t, stderr.String(), "Vendored 0 packages, 1 unchanged")
			case "dry-run":
				assert.NoDirExists(t, target)
				assert.NoFileExists(t, "vendor.lock.yaml")
				assert.Contains(t, stderr.String(), "Checked 1 packages, 0 failed, 0 canceled")
			case "cancel":
				assert.NoDirExists(t, target)
			}
		})
	}
	t.Run("empty batch", func(t *testing.T) {
		require.NoError(t, ExecuteVendorPackages(context.Background(), nil, nil, install.InstallOptions{}))
	})
}
