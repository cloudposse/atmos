package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mock_configuration_test.go -package=terraform github.com/cloudposse/atmos/pkg/schema ComponentConfigurationRuntime

func TestPrepareConfiguration(t *testing.T) {
	runtime := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}
	binary, err := os.Executable()
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Command: binary, SubCommand: "apply", Stack: "live/dev", ComponentFromArg: "app", ComponentSection: map[string]any{}, ComponentSettingsSection: map[string]any{"configuration": true}}
	expected := &schema.PreparedComponentConfiguration{Engine: "terraform", Source: "/module", Inputs: map[string]any{"fresh": "output"}, BackendType: "local", Backend: map[string]any{"path": "state.tfstate"}, Workspace: "default"}
	runtime.EXPECT().Prepare(gomock.Any(), info).Return(expected, nil)
	result, err := prepareConfiguration(t.Context(), config, info)
	require.NoError(t, err)
	require.Same(t, expected, result)
	require.True(t, filepath.IsAbs(info.Command))
	require.Equal(t, expected.Inputs, info.ComponentVarsSection)
	require.Equal(t, map[string]any{"uri": expected.Source}, info.ComponentSection["source"])
	require.Equal(t, expected.Backend, info.ComponentBackendSection)
	require.Equal(t, "default", info.TerraformWorkspace)
	require.True(t, config.Components.Terraform.AutoGenerateBackendFile)
}

func TestPrepareConfigurationGuards(t *testing.T) {
	for _, test := range []struct {
		name    string
		info    schema.ConfigAndStacksInfo
		runtime bool
		want    error
	}{
		{name: "ordinary"},
		{name: "explicitly disabled", info: schema.ConfigAndStacksInfo{ComponentSettingsSection: map[string]any{"configuration": false}}},
		{name: "null", info: schema.ConfigAndStacksInfo{ComponentSettingsSection: map[string]any{"configuration": nil}}},
		{name: "unavailable", info: schema.ConfigAndStacksInfo{ComponentSettingsSection: map[string]any{"configuration": true}}, want: errUtils.ErrPluginUnsupportedExtension},
		{name: "abstract", runtime: true, info: schema.ConfigAndStacksInfo{ComponentSettingsSection: map[string]any{"configuration": true}, SubCommand: "apply", ComponentIsAbstract: true}, want: errUtils.ErrAbstractComponentCantBeProvisioned},
		{name: "dry run", runtime: true, info: schema.ConfigAndStacksInfo{ComponentSettingsSection: map[string]any{"configuration": true}, DryRun: true}},
		{name: "missing binary", runtime: true, info: schema.ConfigAndStacksInfo{ComponentSettingsSection: map[string]any{"configuration": true}, Command: "atmos-no-such-engine-234856"}, want: errUtils.ErrPluginInvalidConfig},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			if test.runtime {
				config.ComponentConfigurations = NewMockComponentConfigurationRuntime(gomock.NewController(t))
			}
			result, err := prepareConfiguration(t.Context(), config, &test.info)
			require.ErrorIs(t, err, test.want)
			require.Nil(t, result)
		})
	}
}

func TestConfigurationArtifactNames(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "live/dev", ComponentFromArg: "region/app", ComponentSettingsSection: map[string]any{"configuration": true}}
	names := []string{ConstructTerraformComponentVarfileName(info), ConstructTerraformComponentPlanfileName(info), ConstructTerraformComponentTestVarfileName(info)}
	for _, name := range names {
		require.Equal(t, filepath.Base(name), name)
		require.Contains(t, name, "atmos-")
	}
	first := names[0]
	info.Stack = "live/prod"
	require.NotEqual(t, first, ConstructTerraformComponentVarfileName(info))
	info.Stack = "live/dev"
	info.ComponentFromArg = "other/app"
	require.NotEqual(t, first, ConstructTerraformComponentVarfileName(info))
	// Without ComponentFromArg the component identity still keeps names distinct.
	info.ComponentFromArg, info.Component = "", "region/app"
	app := ConstructTerraformComponentPlanfileName(info)
	info.Component = "region/other"
	require.NotEqual(t, app, ConstructTerraformComponentPlanfileName(info))
	info.ComponentFromArg, info.Component = "region/app", "ignored"
	require.Equal(t, app, ConstructTerraformComponentPlanfileName(info), "ComponentFromArg keeps precedence")
	info.ComponentSettingsSection = nil
	_, owned := configurationArtifactName(info, ".plan")
	require.False(t, owned)
}

// A plan written by one process is applied by another, so the artifact name must never depend on
// process state. The digest is pinned so a change to the naming scheme is deliberate.
func TestConfigurationIdentityIsStableAndSeparatesFields(t *testing.T) {
	const pinned = "060d8d2f24f80685c67572911d8f2cd5"
	first := configurationIdentity("live/dev", "region/app")
	require.Equal(t, pinned, first)
	require.Equal(t, first, configurationIdentity("live/dev", "region/app"))
	require.NotEqual(t, first, configurationIdentity("live/dev", "region/app2"))
	// The separator keeps adjacent fields from sliding into each other.
	require.NotEqual(t, configurationIdentity("ab", "c"), configurationIdentity("a", "bc"))
}

func TestCheckConfigurationWorkspace(t *testing.T) {
	for _, test := range []struct {
		name      string
		base      []string
		component []string
		want      error
	}{
		{name: "unset", base: []string{}},
		{name: "empty", base: []string{"TF_WORKSPACE="}},
		{name: "matching base", base: []string{"TF_WORKSPACE=stage"}},
		{name: "matching component", base: []string{}, component: []string{"TF_WORKSPACE=stage"}},
		{name: "conflicting base", base: []string{"TF_WORKSPACE=prod"}, want: errUtils.ErrPluginInvalidConfig},
		{name: "conflicting component", base: []string{"TF_WORKSPACE=stage"}, component: []string{"TF_WORKSPACE=prod"}, want: errUtils.ErrPluginInvalidConfig},
		{name: "component overrides conflicting base", base: []string{"TF_WORKSPACE=prod"}, component: []string{"TF_WORKSPACE=stage"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{TerraformWorkspace: "stage", SanitizedEnv: test.base, ComponentEnvList: test.component}
			require.ErrorIs(t, checkConfigurationWorkspace(info), test.want)
		})
	}
}

func TestRejectRuntimeArtifacts(t *testing.T) {
	require.NoError(t, rejectRuntimeArtifacts([]schema.ComponentConfigurationFile{{Path: "backend.tf", IfExists: "overwrite"}}))
	for _, path := range []string{"examples/terraform.tfstate", "nested/provider.tf"} {
		require.NoError(t, rejectRuntimeArtifacts([]schema.ComponentConfigurationFile{{Path: path}}), path)
	}
	for _, path := range []string{"terraform.tfstate", "terraform.tfstate.backup", ".terraform.tfstate.lock.info", ".terraform/environment", "nested/.terraform/modules/modules.json", "terraform.tfstate.d/stage/terraform.tfstate", ".terraform.lock.hcl", "nested/.dev-app.terraform.lock.hcl", ".atmos/metadata.json"} {
		files := []schema.ComponentConfigurationFile{{Path: "backend.tf", IfExists: "overwrite"}, {Path: path, IfExists: "overwrite"}}
		require.ErrorIs(t, rejectRuntimeArtifacts(files), errUtils.ErrPluginInvalidConfig, path)
	}
}

func TestPreparedConfigurationSensitiveInputsNeverHitDisk(t *testing.T) {
	atmosio.Reset()
	t.Cleanup(atmosio.Reset)
	require.NoError(t, atmosio.Initialize())
	for _, disabled := range []bool{false, true} {
		atmosio.ApplyMaskingConfig(&atmosio.Config{DisableMasking: disabled})
		runtime := NewMockComponentConfigurationRuntime(gomock.NewController(t))
		config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}
		binary, err := os.Executable()
		require.NoError(t, err)
		info := &schema.ConfigAndStacksInfo{Command: binary, SubCommand: "apply", ComponentSection: map[string]any{}, ComponentSettingsSection: map[string]any{"configuration": true}}
		inputs := map[string]any{"credentials": map[string]any{"secret-input-key": ""}, "empty": map[string]any{}, "null": nil, "public": "visible"}
		prepared := &schema.PreparedComponentConfiguration{Inputs: inputs, SensitiveInputKeys: map[string]bool{"credentials": true, "empty": true, "null": true, "absent": true}}
		runtime.EXPECT().Prepare(gomock.Any(), info).Return(prepared, nil)
		_, err = prepareConfiguration(t.Context(), config, info)
		require.NoError(t, err)
		// Partition repeatedly without relying on registered secret strings.
		ComputeTerraformSecretVarKeys(info)
		ComputeTerraformSecretVarKeys(info)
		require.Equal(t, map[string]bool{"credentials": true, "empty": true, "null": true}, info.TerraformSecretVarKeys)
		require.Equal(t, map[string]any{"public": "visible"}, DiskSafeVars(info))
		env, err := SecretVarEnv(info)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{`TF_VAR_credentials={"secret-input-key":""}`, `TF_VAR_empty={}`, `TF_VAR_null=null`}, env)
	}
}
