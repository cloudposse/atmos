package output

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type stateRuntimeFixture struct {
	resolve func(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error)
}

func (*stateRuntimeFixture) Discover(context.Context) (map[string]map[string]any, error) {
	panic("unexpected discovery")
}

func (*stateRuntimeFixture) Prepare(context.Context, *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
	panic("state reads must not prepare")
}

func (s *stateRuntimeFixture) ResolveState(ctx context.Context, info *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
	return s.resolve(ctx, info)
}

func TestResolveConfigurationStateUsesTargetEnvironment(t *testing.T) {
	t.Setenv("ATMOS_STATE_AMBIENT_SECRET", "must-not-leak")
	ctrl := gomock.NewController(t)
	manager := types.NewMockAuthManager(ctrl)
	manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{SanitizedEnv: []string{}})
	sections := validSections()
	sections["settings"] = map[string]any{"configuration": true}
	sections["env"] = map[string]any{"EXPLICIT": "component"}
	config := validAtmosConfig(t)
	config.ComponentConfigurations = &stateRuntimeFixture{resolve: func(_ context.Context, info *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
		require.Equal(t, []string{"EXPLICIT=component"}, info.SanitizedEnv)
		require.Equal(t, "output", info.SubCommand)
		require.Equal(t, "network", info.ComponentFromArg)
		return &schema.ComponentConfigurationState{BackendType: "local", Workspace: "default"}, nil
	}}
	state, err := ResolveConfigurationState(t.Context(), config, &ConfigurationStateRequest{Sections: sections, Component: "network", Stack: "live", AuthManagers: []any{manager}})
	require.NoError(t, err)
	require.Equal(t, "local", state.BackendType)
	config.ComponentConfigurations = nil
	_, err = ResolveConfigurationState(t.Context(), config, &ConfigurationStateRequest{Sections: sections, Component: "network", Stack: "live"})
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	delete(sections, "settings")
	state, err = ResolveConfigurationState(t.Context(), config, &ConfigurationStateRequest{Sections: sections, Component: "network", Stack: "live"})
	require.NoError(t, err)
	require.Nil(t, state)
}

func TestConfigurationStateSectionsSelectsNamedWorkspace(t *testing.T) {
	root := t.TempDir()
	sections := map[string]any{"source": map[string]any{"uri": "never-fetch"}, "provision": map[string]any{}, "remote_state_backend_type": "static"}
	state := &schema.ComponentConfigurationState{BackendType: "local", Backend: map[string]any{"path": filepath.Join(root, "default.tfstate"), "workspace_dir": filepath.Join(root, "workspaces")}, Workspace: "prod", WorkingDirectory: root}
	resolved := ConfigurationStateSections(sections, state)
	require.Equal(t, filepath.Join(root, "workspaces", "prod", "terraform.tfstate"), resolved["backend"].(map[string]any)["path"])
	require.Equal(t, filepath.Join(root, "default.tfstate"), state.Backend["path"])
	require.Contains(t, sections, "source")
	require.NotContains(t, resolved, "source")
	delete(state.Backend, "workspace_dir")
	resolved = ConfigurationStateSections(sections, state)
	require.Equal(t, filepath.Join(root, "terraform.tfstate.d", "prod", "terraform.tfstate"), resolved["backend"].(map[string]any)["path"])
	state.Workspace = "default"
	resolved = ConfigurationStateSections(sections, state)
	require.Equal(t, state.Backend, resolved["backend"])
	require.Equal(t, sections, ConfigurationStateSections(sections, nil))
}

func TestPluginOutputUsesBackendOnlyScratchDirectory(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "missing workspace"}[fail], func(t *testing.T) {
			ctrl := gomock.NewController(t)
			runner := NewMockTerraformRunner(ctrl)
			runner.EXPECT().SetEnv(gomock.Any()).DoAndReturn(func(env map[string]string) error {
				require.NotEqual(t, "original-data", env["TF_DATA_DIR"])
				return nil
			})
			runner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
			var selectErr error
			if fail {
				selectErr = errors.New("workspace prod does not exist")
			}
			runner.EXPECT().WorkspaceSelect(gomock.Any(), "prod").Return(selectErr)
			if !fail {
				runner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{"result": {Type: json.RawMessage(`"string"`), Value: json.RawMessage(`"fresh"`)}}, nil)
			}
			config := validAtmosConfig(t)
			stateRoot := t.TempDir()
			config.ComponentConfigurations = &stateRuntimeFixture{resolve: func(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
				return &schema.ComponentConfigurationState{BackendType: "local", Backend: map[string]any{"path": filepath.Join(stateRoot, "default.tfstate")}, Workspace: "prod", WorkingDirectory: stateRoot}, nil
			}}
			sections := validSections()
			sections["settings"] = map[string]any{"configuration": true}
			sections["source"] = map[string]any{"uri": "https://must-not-fetch.invalid/module"}
			sections["env"] = map[string]any{"TF_DATA_DIR": "original-data"}
			binary, err := os.Executable()
			require.NoError(t, err)
			sections["command"] = binary
			var scratch string
			executor := NewExecutor(nil, WithRunnerFactory(func(dir, executable string) (TerraformRunner, error) {
				scratch = dir
				require.NotEqual(t, stateRoot, dir)
				files, err := os.ReadDir(dir)
				require.NoError(t, err)
				require.Len(t, files, 1)
				require.Equal(t, "backend.tf.json", files[0].Name())
				data, err := os.ReadFile(filepath.Join(dir, "backend.tf.json"))
				require.NoError(t, err)
				var backend map[string]any
				require.NoError(t, json.Unmarshal(data, &backend))
				local := backend["terraform"].(map[string]any)["backend"].(map[string]any)["local"].(map[string]any)
				require.Equal(t, filepath.Join(stateRoot, "terraform.tfstate.d"), local["workspace_dir"])
				return runner, nil
			}))
			result, err := executor.execute(t.Context(), config, "network", "live", sections, nil, &OutputOptions{}, true)
			if fail {
				require.ErrorContains(t, err, "workspace prod does not exist")
			} else {
				require.NoError(t, err)
				require.Equal(t, "fresh", result["result"])
			}
			require.NoDirExists(t, scratch)
			files, err := os.ReadDir(stateRoot)
			require.NoError(t, err)
			require.Empty(t, files)
		})
	}
}

func TestConfigurationStateFailureAndScratchCleanup(t *testing.T) {
	config := validAtmosConfig(t)
	sections := validSections()
	sections["settings"] = map[string]any{"configuration": true}
	config.ComponentConfigurations = &stateRuntimeFixture{resolve: func(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
		return nil, context.Canceled
	}}
	_, err := NewExecutor(nil).execute(t.Context(), config, "network", "live", sections, nil, nil, true)
	require.ErrorIs(t, err, context.Canceled)
	badTemp := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("TMPDIR", badTemp)
	t.Setenv("TMP", badTemp)
	t.Setenv("TEMP", badTemp)
	_, err = configurationOutputConfig(&ComponentConfig{}, &schema.ComponentConfigurationState{})
	require.Error(t, err)
}

func TestReadOnlyHTTPOutputDoesNotSelectWorkspace(t *testing.T) {
	runner := NewMockTerraformRunner(gomock.NewController(t))
	err := ensureOutputWorkspace(t.Context(), nil, runner, &ComponentConfig{ReadOnlyState: true, BackendType: "http"}, &workspaceTarget{component: "network", stack: "live"})
	require.NoError(t, err)
}

func TestOutputReaderAnchorsRelativeBackendFiles(t *testing.T) {
	unit := t.TempDir()
	absolute := filepath.Join(t.TempDir(), "credentials")
	backend := map[string]any{
		"bucket":                   "state",
		"shared_credentials_files": []any{"creds/aws", absolute},
		"shared_config_files":      []string{"config/aws"},
		"shared_credentials_file":  "legacy",
		"client_certificate_path":  "~/cert.pfx",
	}
	state := &schema.ComponentConfigurationState{BackendType: "s3", Backend: backend, WorkingDirectory: unit}
	config := &ComponentConfig{}
	cleanup, err := configurationOutputConfig(config, state)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.NotEqual(t, unit, config.ComponentPath)
	require.Equal(t, []any{filepath.Join(unit, "creds", "aws"), absolute}, config.Backend["shared_credentials_files"])
	require.Equal(t, []any{filepath.Join(unit, "config", "aws")}, config.Backend["shared_config_files"])
	require.Equal(t, filepath.Join(unit, "legacy"), config.Backend["shared_credentials_file"])
	require.Equal(t, "~/cert.pfx", config.Backend["client_certificate_path"], "home-relative paths are left to Terraform")
	require.Equal(t, "state", config.Backend["bucket"])
	require.Equal(t, []any{"creds/aws", absolute}, backend["shared_credentials_files"], "the provider's state is not modified")
	require.Equal(t, backend, absoluteBackendFiles("s3", backend, ""))
}

func TestOutputReaderAnchorsPathOnlyForLocalBackend(t *testing.T) {
	unit := t.TempDir()
	consul := map[string]any{"path": "live/vpc", "address": "consul:8500"}
	require.Equal(t, "live/vpc", absoluteBackendFiles("consul", consul, unit)["path"], "a remote key is not a file")
	local := map[string]any{"path": "state/terraform.tfstate", "workspace_dir": "ws"}
	anchored := absoluteBackendFiles("local", local, unit)
	require.Equal(t, filepath.Join(unit, "state", "terraform.tfstate"), anchored["path"])
	require.Equal(t, filepath.Join(unit, "ws"), anchored["workspace_dir"])
}

func TestOutputReaderDefaultsLocalBackendPaths(t *testing.T) {
	unit := t.TempDir()
	state := &schema.ComponentConfigurationState{BackendType: "local", Backend: map[string]any{}, WorkingDirectory: unit}
	config := &ComponentConfig{}
	cleanup, err := configurationOutputConfig(config, state)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.Equal(t, filepath.Join(unit, "terraform.tfstate"), config.Backend["path"])
	require.Equal(t, filepath.Join(unit, "terraform.tfstate.d"), config.Backend["workspace_dir"])
	require.Empty(t, state.Backend)
}

func TestConfigurationStateSectionsNamedWorkspaceWithoutBackendSettings(t *testing.T) {
	root := t.TempDir()
	state := &schema.ComponentConfigurationState{BackendType: "local", Workspace: "prod", WorkingDirectory: root}
	resolved := ConfigurationStateSections(map[string]any{}, state)
	require.Equal(t, filepath.Join(root, "terraform.tfstate.d", "prod", "terraform.tfstate"), resolved["backend"].(map[string]any)["path"])
	require.Nil(t, state.Backend)
}

// TestPluginOutputSkipInitReadsTheInitializedUnit verifies that skipping init is honored for a
// plugin-owned component: outputs are read from the unit's initialized directory as it is, with no
// init, no workspace selection, and no generated files.
func TestPluginOutputSkipInitReadsTheInitializedUnit(t *testing.T) {
	ctrl := gomock.NewController(t)
	runner := NewMockTerraformRunner(ctrl)
	runner.EXPECT().SetEnv(gomock.Any()).Return(nil)
	runner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{"result": {Type: json.RawMessage(`"string"`), Value: json.RawMessage(`"applied"`)}}, nil)
	config := validAtmosConfig(t)
	unit := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(unit, ".terraform"), 0o700))
	config.ComponentConfigurations = &stateRuntimeFixture{resolve: func(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
		return &schema.ComponentConfigurationState{BackendType: "local", Workspace: "prod", WorkingDirectory: unit}, nil
	}}
	sections := validSections()
	sections["settings"] = map[string]any{"configuration": true}
	sections["providers"] = map[string]any{"aws": map[string]any{"region": "us-east-2"}}
	binary, err := os.Executable()
	require.NoError(t, err)
	sections["command"] = binary
	executor := NewExecutor(nil, WithRunnerFactory(func(dir, _ string) (TerraformRunner, error) {
		require.Equal(t, unit, dir, "skipping init must read the unit's own initialized directory")
		return runner, nil
	}))

	result, err := executor.execute(t.Context(), config, "network", "live", sections, nil, &OutputOptions{SkipInit: true}, true)
	require.NoError(t, err)
	require.Equal(t, "applied", result["result"])
	files, err := os.ReadDir(unit)
	require.NoError(t, err)
	require.Len(t, files, 1, "no backend, provider, or marker file may be written into the unit")
	require.Equal(t, ".terraform", files[0].Name())
}

// TestPluginOutputSkipInitRejectsAnUninitializedUnit verifies that skipping init never falls back
// to initializing anything: a unit that was never initialized fails with a clear error.
func TestPluginOutputSkipInitRejectsAnUninitializedUnit(t *testing.T) {
	for name, state := range map[string]*schema.ComponentConfigurationState{
		"no working directory": {BackendType: "local", Workspace: "prod"},
		"not initialized":      {BackendType: "local", Workspace: "prod", WorkingDirectory: t.TempDir()},
	} {
		t.Run(name, func(t *testing.T) {
			config := validAtmosConfig(t)
			config.ComponentConfigurations = &stateRuntimeFixture{resolve: func(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
				return state, nil
			}}
			sections := validSections()
			sections["settings"] = map[string]any{"configuration": true}
			executor := NewExecutor(nil, WithRunnerFactory(func(string, string) (TerraformRunner, error) {
				t.Fatal("no Terraform process may start")
				return nil, nil
			}))
			_, err := executor.execute(t.Context(), config, "network", "live", sections, nil, &OutputOptions{SkipInit: true}, true)
			require.ErrorIs(t, err, errUtils.ErrTerraformOutputFailed)
		})
	}
}

// TestConfigurationInitializedOutputConfigUsesEffectiveDataDir verifies that the initialized
// directory is located through the effective TF_DATA_DIR, with SetupEnvironment's precedence.
func TestConfigurationInitializedOutputConfigUsesEffectiveDataDir(t *testing.T) {
	for _, test := range []struct {
		name      string
		inherited string
		base      []string
		env       map[string]any
		dataDir   string
		ok        bool
	}{
		{name: "base environment", base: []string{"TF_DATA_DIR=base-data"}, dataDir: "base-data", ok: true},
		{name: "inherited environment without base", inherited: "inherited-data", dataDir: "inherited-data", ok: true},
		{name: "base environment omits inherited value", inherited: "inherited-data", base: []string{"PATH=/bin"}, dataDir: "inherited-data"},
		{name: "component env overrides base", base: []string{"TF_DATA_DIR=base-data"}, env: map[string]any{"TF_DATA_DIR": "component-data"}, dataDir: "component-data", ok: true},
		{name: "default data directory", base: []string{}, dataDir: ".terraform", ok: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TF_DATA_DIR", test.inherited)
			unit := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(unit, test.dataDir), 0o700))
			config := &ComponentConfig{BaseEnvironment: test.base, Env: test.env}
			err := configurationInitializedOutputConfig(config, &schema.ComponentConfigurationState{Workspace: "prod", WorkingDirectory: unit}, "network", "live")
			if !test.ok {
				require.ErrorIs(t, err, errUtils.ErrTerraformOutputFailed)
				return
			}
			require.NoError(t, err)
			require.Equal(t, unit, config.ComponentPath)
		})
	}
}

// TestPluginOutputDoesNotResolveTheExecutionWorkdir verifies that a plugin-owned state read
// succeeds even when the component's execution workdir cannot be resolved.
func TestPluginOutputDoesNotResolveTheExecutionWorkdir(t *testing.T) {
	ctrl := gomock.NewController(t)
	runner := NewMockTerraformRunner(ctrl)
	runner.EXPECT().SetEnv(gomock.Any()).Return(nil)
	runner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{"result": {Type: json.RawMessage(`"string"`), Value: json.RawMessage(`"applied"`)}}, nil)
	config := validAtmosConfig(t)
	unit := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(unit, ".terraform"), 0o700))
	config.ComponentConfigurations = &stateRuntimeFixture{resolve: func(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
		return &schema.ComponentConfigurationState{BackendType: "local", Workspace: "prod", WorkingDirectory: unit}, nil
	}}
	sections := validSections()
	sections["settings"] = map[string]any{"configuration": true}
	// No base component: an ordinary component could not resolve its execution path at all.
	delete(sections, "component")
	_, err := ExtractComponentConfig(config, sections, "network", "live")
	require.ErrorIs(t, err, errUtils.ErrMissingComponentPath, "precondition: the execution path is unresolvable")
	executor := NewExecutor(nil, WithRunnerFactory(func(string, string) (TerraformRunner, error) { return runner, nil }))

	result, err := executor.execute(t.Context(), config, "network", "live", sections, nil, &OutputOptions{SkipInit: true}, true)
	require.NoError(t, err)
	require.Equal(t, "applied", result["result"])
}

// TestConfigurationStateSectionsKeepsExplicitRemoteStateOverrides verifies that remote-state read
// settings the stack configured survive for a plugin-owned component, layered on the plugin's
// backend, while the discovery placeholder backend never leaks into the read.
func TestConfigurationStateSectionsKeepsExplicitRemoteStateOverrides(t *testing.T) {
	state := &schema.ComponentConfigurationState{BackendType: "s3", Backend: map[string]any{"bucket": "unit-state", "key": "live/network.tfstate", "region": "us-east-2"}, Workspace: "default"}
	placeholder := map[string]any{"bucket": "placeholder", "region": "us-east-2"}
	for _, test := range []struct {
		name       string
		remoteType string
		remote     map[string]any
		wantType   string
		want       map[string]any
	}{
		{name: "no override derives nothing", remoteType: "s3", remote: maps.Clone(placeholder)},
		{
			name: "read role override is layered on the plugin backend", remoteType: "s3",
			remote:   map[string]any{"bucket": "placeholder", "region": "us-east-2", "role_arn": "arn:aws:iam::1:role/read"},
			wantType: "s3", want: map[string]any{"bucket": "unit-state", "key": "live/network.tfstate", "region": "us-east-2", "role_arn": "arn:aws:iam::1:role/read"},
		},
		{
			name: "explicit separate location wins over the plugin backend", remoteType: "s3",
			remote:   map[string]any{"bucket": "mirror", "region": "us-east-2"},
			wantType: "s3", want: map[string]any{"bucket": "mirror", "key": "live/network.tfstate", "region": "us-east-2"},
		},
		{
			name: "a different backend type is a separate read location", remoteType: "gcs",
			remote:   map[string]any{"bucket": "gcs-mirror", "prefix": "network"},
			wantType: "gcs", want: map[string]any{"bucket": "gcs-mirror", "prefix": "network"},
		},
		{
			name: "static remote state is kept", remoteType: "static",
			remote:   map[string]any{"vpc_id": "vpc-1"},
			wantType: "static", want: map[string]any{"vpc_id": "vpc-1"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			sections := map[string]any{"backend_type": "s3", "backend": maps.Clone(placeholder), "remote_state_backend_type": test.remoteType, "remote_state_backend": test.remote}
			resolved := ConfigurationStateSections(sections, state)
			require.Equal(t, state.Backend, resolved["backend"], "the plugin backend stays the primary location")
			if test.wantType == "" {
				require.NotContains(t, resolved, "remote_state_backend")
				require.NotContains(t, resolved, "remote_state_backend_type")
				return
			}
			require.Equal(t, test.wantType, resolved["remote_state_backend_type"])
			require.Equal(t, test.want, resolved["remote_state_backend"])
			require.Equal(t, map[string]any{"bucket": "unit-state", "key": "live/network.tfstate", "region": "us-east-2"}, state.Backend, "the plugin state must not be modified")
		})
	}
}
