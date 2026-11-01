package terraform

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

func configurationExecution(t *testing.T) (*Executor, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) {
	t.Helper()
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	binary, err := os.Executable()
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Command: binary, SubCommand: "plan", ComponentSection: map[string]any{}, ComponentEnvSection: map[string]any{}, ComponentSettingsSection: map[string]any{"configuration": true}}
	return NewExecutor(Services{}), config, info
}

func TestConfigurationEnvironmentPrecedesEvaluation(t *testing.T) {
	for _, identity := range []string{"explicit-production", "default-development", "none"} {
		t.Run(identity, func(t *testing.T) {
			executor, config, info := configurationExecution(t)
			info.Identity = identity
			runtime := NewMockComponentConfigurationRuntime(gomock.NewController(t))
			config.ComponentConfigurations = runtime
			events := []string{}
			executor.prepareAuthEnvironmentFn = func(_ *schema.AtmosConfiguration, got *schema.ConfigAndStacksInfo) error {
				events = append(events, "authenticate")
				require.Equal(t, identity, got.Identity)
				got.SanitizedEnv = []string{"AWS_PROFILE=" + identity, "PATH=identity-path"}
				got.ComponentEnvSection["FROM_COMPONENT"] = "override"
				return nil
			}
			executor.resolveConfigurationToolsFn = func(_ *schema.AtmosConfiguration, got *schema.ConfigAndStacksInfo) (*dependencies.ToolchainEnvironment, error) {
				events = append(events, "toolchain")
				require.Equal(t, []string{"AWS_PROFILE=" + identity, "PATH=identity-path"}, got.SanitizedEnv)
				return &dependencies.ToolchainEnvironment{}, nil
			}
			runtime.EXPECT().Prepare(gomock.Any(), info).DoAndReturn(func(_ context.Context, got *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
				events = append(events, "evaluate")
				require.Equal(t, "AWS_PROFILE="+identity, got.SanitizedEnv[0])
				require.Contains(t, got.ComponentEnvList, "FROM_COMPONENT=override")
				require.Contains(t, got.ComponentEnvList, "TF_IN_AUTOMATION=true")
				require.True(t, filepath.IsAbs(got.Command))
				return nil, nil
			})
			_, err := executor.prepareConfigurationEnvironment(t.Context(), config, info, true)
			require.NoError(t, err)
			_, err = prepareConfiguration(t.Context(), config, info)
			require.NoError(t, err)
			require.Equal(t, []string{"authenticate", "toolchain", "evaluate"}, events)
		})
	}
}

func TestConfigurationEnvironmentStopsBeforeEffects(t *testing.T) {
	for _, name := range []string{"auth failure", "toolchain failure", "cancellation", "cancellation during auth", "locked", "dry run", "ordinary"} {
		t.Run(name, func(t *testing.T) {
			executor, config, info := configurationExecution(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			events := []string{}
			executor.prepareAuthEnvironmentFn = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) error {
				events = append(events, "auth")
				if name == "auth failure" {
					return errUtils.ErrInvalidAuthConfig
				}
				if name == "cancellation during auth" {
					cancel()
				}
				return nil
			}
			executor.resolveConfigurationToolsFn = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (*dependencies.ToolchainEnvironment, error) {
				events = append(events, "tools")
				return nil, errUtils.ErrPluginInvalidConfig
			}
			switch name {
			case "cancellation":
				cancel()
			case "locked":
				info.ComponentIsLocked = true
				info.SubCommand = "apply"
			case "dry run":
				info.DryRun = true
			case "ordinary":
				info.ComponentSettingsSection = nil
			}
			_, err := executor.prepareConfigurationEnvironment(ctx, config, info, true)
			switch name {
			case "dry run", "ordinary":
				require.NoError(t, err)
				require.Empty(t, events)
			case "auth failure":
				require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
				require.Equal(t, []string{"auth"}, events)
			case "toolchain failure":
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
				require.Equal(t, []string{"auth", "tools"}, events)
			case "cancellation during auth":
				require.ErrorIs(t, err, context.Canceled)
				require.Equal(t, []string{"auth"}, events)
			default:
				require.Error(t, err)
				require.Empty(t, events)
			}
		})
	}
}

func TestConfigurationEnvironmentRCAndCache(t *testing.T) {
	executor, config, info := configurationExecution(t)
	config.Components.Terraform.RC = &schema.TerraformRCConfig{Enabled: true, Config: map[string]any{"disable_checkpoint": true}}
	info.DisablePluginCache = true
	executor.prepareAuthEnvironmentFn = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) error {
		info.SanitizedEnv = []string{"AWS_PROFILE=selected", "TF_PLUGIN_CACHE_DIR=/unwanted"}
		return nil
	}
	_, err := executor.prepareConfigurationEnvironment(t.Context(), config, info, false)
	require.NoError(t, err)
	require.NotContains(t, info.SanitizedEnv, "TF_PLUGIN_CACHE_DIR=/unwanted")
	var path string
	for _, entry := range info.ComponentEnvList {
		if strings.HasPrefix(entry, "TF_CLI_CONFIG_FILE=") {
			path = strings.TrimPrefix(entry, "TF_CLI_CONFIG_FILE=")
		}
	}
	require.NotEmpty(t, path)
	require.FileExists(t, path)
	require.NotNil(t, info.RCCleanup)
	require.NoError(t, info.RCCleanup())
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

func TestConfigurationPreparationUsesSelectedInstalledToolVersion(t *testing.T) {
	previous := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })
	executor, config, info := configurationExecution(t)
	config.BasePathAbsolute = config.BasePath
	config.Toolchain = schema.Toolchain{Install: schema.ToolchainInstallNever, InstallPath: filepath.Join(config.BasePath, "tools"), Aliases: map[string]string{"tofu": "opentofu/opentofu"}}
	config.Components.Terraform.Command = "tofu"
	binary, err := os.Executable()
	require.NoError(t, err)
	bytes, err := os.ReadFile(binary)
	require.NoError(t, err)
	name := "tofu"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		target := filepath.Join(config.Toolchain.InstallPath, "bin", "opentofu", "opentofu", version, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
		require.NoError(t, os.WriteFile(target, bytes, 0o755))
	}
	provider := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config.ComponentConfigurations = provider
	executor.prepareAuthEnvironmentFn = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) error {
		info.SanitizedEnv = []string{"AWS_PROFILE=selected"}
		return nil
	}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		info.Command = "tofu"
		info.ComponentEnvList = nil
		info.ComponentSection["dependencies"] = map[string]any{"tools": map[string]any{"tofu": version}}
		tenv, err := executor.prepareConfigurationEnvironment(t.Context(), config, info, true)
		require.NoError(t, err)
		expected := filepath.Join(config.Toolchain.InstallPath, "bin", "opentofu", "opentofu", version, name)
		require.Equal(t, expected, info.Command)
		require.Contains(t, strings.Split(tenv.PATH(), string(os.PathListSeparator)), filepath.Dir(expected))
		provider.EXPECT().Prepare(gomock.Any(), info).DoAndReturn(func(_ context.Context, got *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
			require.Equal(t, expected, got.Command)
			require.Contains(t, got.ComponentEnvList, "PATH="+tenv.PATH())
			return nil, nil
		})
		_, err = prepareConfiguration(t.Context(), config, info)
		require.NoError(t, err)
	}
}
