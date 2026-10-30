package terraform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
)

type planRuntimeFixture struct {
	schema.ComponentConfigurationRuntime
	identity string
	state    *schema.ComponentConfigurationState
	err      error
}

func (p *planRuntimeFixture) PlanIdentity(context.Context, *schema.ConfigAndStacksInfo, []schema.ComponentConfigurationSource) (string, error) {
	return p.identity, p.err
}

func (p *planRuntimeFixture) ResolveState(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
	return p.state, p.err
}

func planFixture(t *testing.T) (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, *componentExecContext, *planRuntimeFixture, string) {
	t.Helper()
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: root, Components: schema.Components{Terraform: schema.Terraform{BasePath: "components"}}}
	info := &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: "network", FinalComponent: "network", SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": true}}
	working := ConstructTerraformComponentWorkingDir(config, info)
	require.NoError(t, os.MkdirAll(working, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(working, "main.tf"), []byte("original module"), 0o600))
	binary := filepath.Join(root, "terraform.exe")
	require.NoError(t, os.WriteFile(binary, []byte("selected engine"), 0o700))
	info.Command = binary
	path := filepath.Join(working, "reviewed.planfile")
	require.NoError(t, os.WriteFile(path, []byte("reviewed plan"), 0o600))
	prepared := &schema.PreparedComponentConfiguration{Engine: "terraform", Source: root, Inputs: map[string]any{"value": "reviewed"}, BackendType: "local", Backend: map[string]any{"path": filepath.Join(root, "state")}, Workspace: "default"}
	runtime := &planRuntimeFixture{identity: "provider-source-options", state: &schema.ComponentConfigurationState{BackendType: prepared.BackendType, Backend: prepared.Backend, Workspace: prepared.Workspace}}
	config.ComponentConfigurations = runtime
	execCtx := &componentExecContext{componentPath: working, workingDir: working, preparedConfiguration: prepared}
	return config, info, execCtx, runtime, path
}

func TestConfigurationPlanRestoresReviewedInputs(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	err := recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out", filepath.Base(path)})
	require.NoError(t, err)
	stat, err := os.Stat(path + configurationPlanSuffix)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0), stat.Mode().Perm()&0o077)
	info.SubCommand = "apply"
	info.UseTerraformPlan = true
	info.PlanFile = path
	prepared, err := restoreConfigurationPlan(t.Context(), config, info)
	require.NoError(t, err)
	require.Equal(t, execCtx.preparedConfiguration.Inputs, prepared.Inputs)
	require.True(t, info.UseTerraformPlan)
	require.Equal(t, path, info.PlanFile)
	require.FileExists(t, path)
}

func TestConfigurationPlanRejectsChangedBinding(t *testing.T) {
	for _, change := range []string{"plan", "engine", "provider", "workdir", "backend", "workspace", "coordinates", "missing binding", "corrupt binding", "missing plan", "runtime error", "unsupported operation", "stored plan"} {
		t.Run(change, func(t *testing.T) {
			config, info, execCtx, runtime, path := planFixture(t)
			require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out=" + path}))
			info.SubCommand = "apply"
			info.UseTerraformPlan = true
			info.PlanFile = path
			switch change {
			case "plan":
				require.NoError(t, os.WriteFile(path, []byte("unreviewed"), 0o600))
			case "engine":
				require.NoError(t, os.WriteFile(info.Command, []byte("changed executable"), 0o700))
			case "provider":
				runtime.identity = "other provider"
			case "workdir":
				require.NoError(t, os.WriteFile(filepath.Join(execCtx.workingDir, "main.tf"), []byte("modified"), 0o600))
			case "backend":
				runtime.state.Backend = map[string]any{"path": "other"}
			case "workspace":
				runtime.state.Workspace = "other"
			case "coordinates":
				info.Stack = "other"
			case "missing binding":
				require.NoError(t, os.Remove(path+configurationPlanSuffix))
			case "corrupt binding":
				require.NoError(t, os.WriteFile(path+configurationPlanSuffix, []byte("not json"), 0o600))
			case "missing plan":
				require.NoError(t, os.Remove(path))
			case "runtime error":
				runtime.err = context.Canceled
			case "unsupported operation":
				info.SubCommand = "destroy"
			case "stored plan":
				info.StoredPlanFile = path
			}
			prepared, err := restoreConfigurationPlan(t.Context(), config, info)
			require.Error(t, err)
			require.Nil(t, prepared)
		})
	}
}

func TestConfigurationPlanRecordingGuards(t *testing.T) {
	config, info, execCtx, runtime, path := planFixture(t)
	for _, args := range [][]string{nil, {"-out"}, {"-out="}} {
		require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, args))
	}
	info.DryRun = true
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}))
	info.DryRun = false
	info.SubCommand = "apply"
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}))
	info.SubCommand = "plan"
	config.ComponentConfigurations = nil
	require.ErrorIs(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}), errUtils.ErrPluginUnsupportedExtension)
	config.ComponentConfigurations = runtime
	runtime.err = context.Canceled
	require.ErrorIs(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}), context.Canceled)
	runtime.err = nil
	require.Error(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path + "missing"}))
	info.Command = path + "missing"
	require.Error(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}))
}

func TestConfigurationPlanPathsAndFileErrors(t *testing.T) {
	config, info, _, _, path := planFixture(t)
	require.Equal(t, ConstructTerraformComponentPlanfilePath(config, info), configurationPlanPath(config, info))
	info.PlanFile = filepath.Base(path)
	require.Equal(t, path, configurationPlanPath(config, info))
	info.PlanFile = path
	require.Equal(t, path, configurationPlanPath(config, info))
	require.Error(t, writePlanBinding(filepath.Join(t.TempDir(), "absent", "binding"), nil))
	_, err := fileDigest(t.TempDir())
	require.Error(t, err)
	_, err = configurationWorkdirDigest(filepath.Join(t.TempDir(), "absent"), path)
	require.Error(t, err)
	require.True(t, usesConfigurationPlan(info))
	require.False(t, usesConfigurationPlan(&schema.ConfigAndStacksInfo{}))
}

func TestConfigurationWorkdirDigestIgnoresMutableFiles(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.plan")
	first, err := configurationWorkdirDigest(root, plan)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(root, ".terraform"), 0o700))
	for _, name := range []string{"terraform.tfstate", "reviewed.plan", "reviewed.plan.atmos.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("mutable"), 0o600))
	}
	second, err := configurationWorkdirDigest(root, plan)
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("source"), 0o600))
	third, err := configurationWorkdirDigest(root, plan)
	require.NoError(t, err)
	require.NotEqual(t, first, third)
	if err := os.Symlink(filepath.Join(root, "main.tf"), filepath.Join(root, "linked.tf")); err != nil {
		t.Skip("symlinks unavailable")
	}
	_, err = configurationWorkdirDigest(root, plan)
	require.True(t, errors.Is(err, errUtils.ErrPluginUnsupportedExtension))
}
