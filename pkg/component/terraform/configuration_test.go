package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mock_configuration_test.go -package=terraform github.com/cloudposse/atmos/pkg/schema ComponentConfigurationRuntime

func TestPrepareConfiguration(t *testing.T) {
	runtime := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}
	binary, err := os.Executable()
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Command: binary, SubCommand: "apply", Stack: "live/dev", ComponentFromArg: "app", ComponentSection: map[string]any{}, ComponentSettingsSection: map[string]any{"configuration": true}}
	expected := &schema.PreparedComponentConfiguration{Engine: "terraform", Source: filepath.Join(t.TempDir(), "module"), Inputs: map[string]any{"fresh": "output"}, BackendType: "local", Backend: map[string]any{"path": "state.tfstate"}, Workspace: "default"}
	runtime.EXPECT().Prepare(gomock.Any(), info).Return(expected, nil)
	result, err := prepareConfiguration(t.Context(), config, info)
	require.NoError(t, err)
	require.Same(t, expected, result)
	require.True(t, filepath.IsAbs(info.Command))
	require.Equal(t, expected.Inputs, info.ComponentVarsSection)
	// Local module edits are detected by the source provisioner, so no TTL forces a refresh.
	require.Equal(t, map[string]any{"uri": expected.Source}, info.ComponentSection["source"])
	require.Equal(t, expected.Backend, info.ComponentBackendSection)
	require.Equal(t, "default", info.TerraformWorkspace)
	require.True(t, config.Components.Terraform.AutoGenerateBackendFile)
}

func TestPrepareConfigurationNeverForcesSourceRefresh(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		{name: "relative path", source: "./modules/unit"},
		{name: "absolute path", source: filepath.Join(t.TempDir(), "module")},
		{name: "git", source: "git::https://example.invalid/modules.git//unit?ref=v1"},
		{name: "archive", source: "https://example.invalid/unit.zip"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewMockComponentConfigurationRuntime(gomock.NewController(t))
			config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}
			binary, err := os.Executable()
			require.NoError(t, err)
			info := &schema.ConfigAndStacksInfo{Command: binary, SubCommand: "apply", ComponentSection: map[string]any{}, ComponentSettingsSection: map[string]any{"configuration": true}}
			runtime.EXPECT().Prepare(gomock.Any(), info).Return(&schema.PreparedComponentConfiguration{Engine: "terraform", Source: test.source}, nil)

			_, err = prepareConfiguration(t.Context(), config, info)

			require.NoError(t, err)
			source, ok := info.ComponentSection["source"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, test.source, source["uri"])
			require.NotContains(t, source, "ttl", "a TTL of zero would re-copy the module and discard saved plans on every run")
		})
	}
}

func TestPrepareConfigurationGuards(t *testing.T) {
	for _, test := range []struct {
		name    string
		info    schema.ConfigAndStacksInfo
		runtime bool
		want    error
	}{
		{name: "ordinary"},
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
	info.ComponentSettingsSection = nil
	_, owned := configurationArtifactName(info, ".plan")
	require.False(t, owned)
}

func TestCacheLocalStatePath(t *testing.T) {
	cache := filepath.Join("live", "dev", "unit", ".terragrunt-cache", "abc", "def", "terraform.tfstate")
	for _, test := range []struct {
		name     string
		prepared schema.PreparedComponentConfiguration
		want     bool
	}{
		{"default local state inside the cache", schema.PreparedComponentConfiguration{BackendType: "local", Backend: map[string]any{"path": cache}}, true},
		{"stable local path", schema.PreparedComponentConfiguration{BackendType: "local", Backend: map[string]any{"path": filepath.Join("live", "dev", "unit", "state", "terraform.tfstate")}}, false},
		{"similar name is not the cache", schema.PreparedComponentConfiguration{BackendType: "local", Backend: map[string]any{"path": filepath.Join("a", "x.terragrunt-cache", "terraform.tfstate")}}, false},
		{"remote backend", schema.PreparedComponentConfiguration{BackendType: "s3", Backend: map[string]any{"path": cache}}, false},
		{"no backend settings", schema.PreparedComponentConfiguration{BackendType: "local"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, found := cacheLocalStatePath(&test.prepared)
			require.Equal(t, test.want, found)
			if found {
				require.Equal(t, cache, path)
			}
		})
	}
}

// Warning about cache-local state must never change where the state lives or fail the command.
func TestPrepareConfigurationKeepsCacheLocalStateLocation(t *testing.T) {
	runtime := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}
	binary, err := os.Executable()
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Command: binary, SubCommand: "plan", Stack: "live/dev", ComponentFromArg: "app", ComponentSection: map[string]any{}, ComponentSettingsSection: map[string]any{"configuration": true}}
	cache := filepath.Join(t.TempDir(), ".terragrunt-cache", "a", "b", "terraform.tfstate")
	runtime.EXPECT().Prepare(gomock.Any(), info).Return(&schema.PreparedComponentConfiguration{Engine: "terraform", Source: "./module", BackendType: "local", Backend: map[string]any{"path": cache}}, nil)

	_, err = prepareConfiguration(t.Context(), config, info)

	require.NoError(t, err)
	require.Equal(t, cache, info.ComponentBackendSection["path"])
}
