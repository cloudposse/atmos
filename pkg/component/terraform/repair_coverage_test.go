package terraform

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProviderRecoveryFailurePreventsInitExecution(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "implicit", true: "explicit"}[explicit], func(t *testing.T) {
			root, cache := t.TempDir(), t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, ".terraform.lock.hcl"), []byte("provider \"registry.terraform.io/hashicorp/null\" {\n version = \"3.2.1\"\n}\n"), 0o600))
			require.NoError(t, os.MkdirAll(filepath.Join(cache, "registry.terraform.io", "hashicorp", "null", "3.2.1", runtime.GOOS+"_"+runtime.GOARCH), 0o755))
			require.NoError(t, os.Mkdir(filepath.Join(root, ".terraform"), 0o755))
			blocked := filepath.Join(root, ".terraform", "atmos-provider-recovery")
			require.NoError(t, os.WriteFile(blocked, []byte("caller-owned blocker"), 0o600))
			executable, err := os.Executable()
			require.NoError(t, err)
			argsFile := filepath.Join(root, "engine-invoked")
			info := &schema.ConfigAndStacksInfo{Command: executable, SubCommand: "plan", ComponentEnvList: []string{"TF_PLUGIN_CACHE_DIR=" + cache, "_ATMOS_TEST_ARGS_FILE=" + argsFile}}
			if explicit {
				info.SubCommand = "init"
				err = testExecutor(t).runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"init"}, root)
			} else {
				_, err = testExecutor(t).runTerraformInitSubprocess(&schema.AtmosConfiguration{}, info, root, []string{"init"})
			}
			require.ErrorIs(t, err, errUtils.ErrTerraformInit)
			require.ErrorContains(t, err, "creating provider recovery cache")
			require.NoFileExists(t, argsFile, "engine must not run after recovery preparation fails")
			content, err := os.ReadFile(blocked)
			require.NoError(t, err)
			require.Equal(t, "caller-owned blocker", string(content))
		})
	}
}

func TestDryRunCleanupRetainsCanonicalCallerPlan(t *testing.T) {
	root := t.TempDir()
	cfg := &schema.AtmosConfiguration{BasePath: root}
	cfg.Components.Terraform.BasePath = "components"
	info := &schema.ConfigAndStacksInfo{DryRun: true, SubCommand: "apply", Component: "probe", FinalComponent: "probe", ContextPrefix: "dev"}
	plan := ConstructTerraformComponentPlanfilePath(cfg, info)
	require.NoError(t, os.MkdirAll(filepath.Dir(plan), 0o755))
	require.NoError(t, os.WriteFile(plan, []byte("reviewed saved plan"), 0o600))
	cleanupTerraformFiles(cfg, info)
	content, err := os.ReadFile(plan)
	require.NoError(t, err)
	require.Equal(t, "reviewed saved plan", string(content))
}

func TestShellDryRunPreservesDisabledAuthAndFilesystem(t *testing.T) {
	for _, unsafe := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid source", true: "unsafe source"}[unsafe], func(t *testing.T) {
			root := t.TempDir()
			cfg := &schema.AtmosConfiguration{BasePath: root}
			cfg.Components.Terraform.BasePath = "components"
			executor := testExecutor(t)
			executor.services.CreateAndAuthenticateAuthManager = func(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
				require.True(t, info.AuthDisabled)
				require.Equal(t, config.IdentityFlagDisabledValue, info.Identity)
				require.True(t, authdeferred.AuthDisabled(ac.AuthManager), "nested deferred references must inherit disabled authentication")
				return nil, nil
			}
			executor.services.ProcessStacks = func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
				name := "probe"
				if unsafe {
					name = "../outside"
				}
				info.FinalComponent = "probe"
				info.ComponentSection = map[string]any{"component": name, "atmos_component": "probe", "atmos_stack": "dev", "source": map[string]any{"uri": "./missing-source"}, "provision": map[string]any{"workdir": map[string]any{"enabled": true}}}
				return info, nil
			}
			err := executor.ExecuteTerraformShell(&ShellOptions{Component: "probe", Stack: "dev", Identity: "OFF", DryRun: true}, cfg)
			if unsafe {
				require.ErrorIs(t, err, errUtils.ErrSourceComponentNameInvalid)
			} else {
				require.NoError(t, err, "dry-run must not fetch the missing source or launch a shell")
			}
			entries, readErr := os.ReadDir(root)
			require.NoError(t, readErr)
			require.Empty(t, entries, "dry-run must not create source, backend, or workdir files")
		})
	}
}

func TestPrerequisiteInitAcceptsPartialStreamsWithoutPollutingData(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	var stdout, diagnostics bytes.Buffer
	info := &schema.ConfigAndStacksInfo{Command: executable, SubCommand: "output", ComponentEnvList: []string{"_ATMOS_TEST_STDOUT=initializing\n", "_ATMOS_TEST_STDERR=diagnostic\n"}}
	captured, err := testExecutor(t).runTerraformInitSubprocess(&schema.AtmosConfiguration{}, info, t.TempDir(), []string{"init"},
		component.WithProcessStreams(process.Streams{Stdout: &stdout}), component.WithStderrCapture(&diagnostics))
	require.NoError(t, err)
	require.Empty(t, stdout.String(), "prerequisite diagnostics must not corrupt output data")
	require.Contains(t, captured, "initializing")
	require.Contains(t, diagnostics.String(), "initializing")
	require.Contains(t, diagnostics.String(), "diagnostic")
}
