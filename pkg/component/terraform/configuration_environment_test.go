package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	componentpkg "github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/provisioner"
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

// Discovery supplies a placeholder backend, so state lookup must fail instead of reading it.
func TestGetTerraformStateRejectsConfigurationOwnedBackend(t *testing.T) {
	executor := testExecutor(t)
	executor.services.ExecuteDescribeComponent = func(*componentpkg.ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{
			"settings":     map[string]any{"configuration": map[string]any{"provider": "terragrunt"}},
			"backend_type": "local",
			"backend":      map[string]any{"local": map[string]any{}},
		}, nil
	}
	value, err := executor.GetTerraformState(&schema.AtmosConfiguration{}, "!terraform.state", "dev", "native", "id", true, nil, nil)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	require.ErrorContains(t, err, "does not support state lookup")
	require.Nil(t, value)
}

// TestPreExecutionEnvPreservesConfigurationRouting checks prepared-environment reuse,
// workspace validation before secret injection, and ordinary/dry-run assembly.
func TestPreExecutionEnvPreservesConfigurationRouting(t *testing.T) {
	for _, tc := range []struct {
		name          string
		configuration bool
		dryRun        bool
		workspace     string
		invalidSecret bool
	}{
		{name: "prepared environment", configuration: true, workspace: "stage"},
		{name: "workspace error before secret encoding", configuration: true, workspace: "other", invalidSecret: true},
		{name: "secret encoding error", configuration: true, workspace: "stage", invalidSecret: true},
		{name: "ordinary component", workspace: "other"},
		{name: "ordinary dry run", dryRun: true, workspace: "other"},
		{name: "configuration dry run", configuration: true, dryRun: true, workspace: "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, config, info := configurationExecution(t)
			info.ComponentSettingsSection["configuration"] = tc.configuration
			info.DryRun = tc.dryRun
			info.TerraformWorkspace = "stage"
			info.SanitizedEnv = []string{"TF_WORKSPACE=" + tc.workspace}
			info.ComponentEnvList = []string{"TF_IN_AUTOMATION=true", "PREPARED=value"}
			info.ComponentEnvSection["FROM_COMPONENT"] = "visible"
			info.TerraformSecretVarKeys = map[string]bool{"token": true}
			info.ComponentVarsSection = map[string]any{"token": "secret-env-fixture"}
			if tc.invalidSecret {
				info.ComponentVarsSection["token"] = make(chan int)
			}

			err := assemblePreExecutionEnvVars(config, info, nil)
			switch tc.name {
			case "workspace error before secret encoding":
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
				require.Equal(t, []string{"TF_IN_AUTOMATION=true", "PREPARED=value"}, info.ComponentEnvList)
			case "secret encoding error":
				var encodingErr *json.UnsupportedTypeError
				require.ErrorAs(t, err, &encodingErr)
				require.Equal(t, []string{"TF_IN_AUTOMATION=true", "PREPARED=value"}, info.ComponentEnvList)
			case "prepared environment":
				require.NoError(t, err)
				require.Equal(t, []string{"TF_IN_AUTOMATION=true", "PREPARED=value", "TF_VAR_token=secret-env-fixture"}, info.ComponentEnvList)
			default:
				require.NoError(t, err)
				require.Contains(t, info.ComponentEnvList, "FROM_COMPONENT=visible")
				require.Equal(t, "TF_IN_AUTOMATION=true", info.ComponentEnvList[0])
				require.Equal(t, "TF_VAR_token=secret-env-fixture", info.ComponentEnvList[len(info.ComponentEnvList)-1])
			}
		})
	}
}

// A provider that excludes the operation ends preparation before any source is fetched; a failing
// read-config hook stops it the same way, and the unit's other hooks stay out of preparation.
func TestPrepareComponentExecutionHonorsExclusionAndReadConfigHooks(t *testing.T) {
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, TerraformDirAbsolutePath: filepath.Join(root, "components")}
	config.Components.Terraform.BasePath = "components"
	provider := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config.ComponentConfigurations = provider
	binary, err := os.Executable()
	require.NoError(t, err)
	newInfo := func() *schema.ConfigAndStacksInfo {
		return &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "app", Component: "app", FinalComponent: "app", Command: binary, SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": true}, ComponentEnvSection: map[string]any{}, ComponentSection: map[string]any{"component": "app", "atmos_component": "app", "atmos_stack": "dev", "source": map[string]any{"uri": "./module"}}}
	}

	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&schema.PreparedComponentConfiguration{
		Engine: "terraform", Source: "./module", Inputs: map[string]any{}, BackendType: "local", Backend: map[string]any{}, Workspace: "default",
		Exclusion: &schema.ComponentExclusion{Actions: []string{"plan"}},
	}, nil)
	_, err = testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, config, newInfo(), false)
	require.ErrorIs(t, err, errUtils.ErrComponentExcluded)
	require.NoDirExists(t, filepath.Join(root, ".workdir"), "an excluded operation must not provision a source")

	hooks := &schema.ComponentExecution{AfterHooks: []schema.ComponentHook{{Name: "read", Commands: []string{"terragrunt-read-config"}, Execute: []string{"failing-hook"}}}}
	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&schema.PreparedComponentConfiguration{
		Engine: "terraform", Source: "./module", Inputs: map[string]any{}, BackendType: "local", Backend: map[string]any{}, Workspace: "default", Execution: hooks,
	}, nil)
	shell := newShellFixture(t, func(*shellCall) error { return errors.New("hook failed") })
	executor := NewExecutor(Services{ExecuteShellCommand: shell.mock.Execute})
	_, err = executor.prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, config, newInfo(), false)
	require.ErrorIs(t, err, errUtils.ErrComponentHookFailed)
	require.Equal(t, []string{"failing-hook"}, shell.names())
}

// A unit whose provider deferred its registry source is prepared in two steps: the read-config hook
// runs after the first preparation, and only then is the provider asked for the source. An ignore
// policy that matches the registry failure ends the run before any source is provisioned.
func TestPrepareComponentExecutionResolvesADeferredSourceAfterReadConfigHooks(t *testing.T) {
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, TerraformDirAbsolutePath: filepath.Join(root, "components")}
	config.Components.Terraform.BasePath = "components"
	provider := NewMockDeferredSourceRuntime(gomock.NewController(t))
	config.ComponentConfigurations = provider
	binary, err := os.Executable()
	require.NoError(t, err)
	const registry = "tfr://registry.example/acme/vpc/aws?version=1.0.0"
	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "app", Component: "app", FinalComponent: "app", Command: binary, SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": true}, ComponentEnvSection: map[string]any{}, ComponentSection: map[string]any{"component": "app", "atmos_component": "app", "atmos_stack": "dev", "source": map[string]any{"uri": registry}}}

	var events []string
	hooks := &schema.ComponentExecution{
		AfterHooks: []schema.ComponentHook{{Name: "login", Commands: []string{"terragrunt-read-config"}, Execute: []string{"registry-login"}}},
		Errors:     &schema.ComponentErrors{Ignore: []schema.ComponentIgnorePolicy{{Name: "private", Patterns: []string{"status code 401"}}}},
	}
	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
		events = append(events, "prepare")
		return &schema.PreparedComponentConfiguration{Engine: "terraform", Source: registry, SourceDeferred: true, Inputs: map[string]any{}, BackendType: "local", Backend: map[string]any{}, Workspace: "default", Execution: hooks}, nil
	}).Times(1)
	provider.EXPECT().ResolveDeferredSource(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
		events = append(events, "resolve source")
		return &schema.PreparedComponentConfiguration{Source: registry, IgnoredRun: &schema.ComponentIgnoredRun{Policy: "private"}}, nil
	}).Times(1)
	shell := newShellFixture(t, func(*shellCall) error {
		events = append(events, "read-config hook")
		return nil
	})
	executor := NewExecutor(Services{ExecuteShellCommand: shell.mock.Execute})
	_, err = executor.prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, config, info, false)
	require.ErrorIs(t, err, errUtils.ErrComponentRunIgnored)
	require.Equal(t, []string{"prepare", "read-config hook", "resolve source"}, events)
	require.NoDirExists(t, filepath.Join(root, ".workdir"), "an ignored registry failure must not provision a source")
}
