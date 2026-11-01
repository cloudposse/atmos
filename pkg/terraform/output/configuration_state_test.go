package output

import (
	"context"
	"encoding/json"
	"errors"
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
				return &schema.ComponentConfigurationState{BackendType: "local", Backend: map[string]any{"path": filepath.Join(stateRoot, "default.tfstate"), "workspace_dir": filepath.Join(stateRoot, "workspaces")}, Workspace: "prod", WorkingDirectory: stateRoot}, nil
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
				return runner, nil
			}))
			result, err := executor.execute(t.Context(), config, "network", "live", sections, nil, &OutputOptions{SkipInit: true}, true)
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
