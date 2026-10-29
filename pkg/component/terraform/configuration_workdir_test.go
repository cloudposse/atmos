package terraform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConfigurationWorkdirOwnsArtifactsNotState(t *testing.T) {
	for _, mode := range []string{"component directory", "isolated workdir", "explicit workdir", "relative workdir"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "module")
			require.NoError(t, os.MkdirAll(source, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(source, "main.tf"), []byte("variable \"value\" {}\n"), 0o644))
			state := filepath.Join(root, "terragrunt-state", "terraform.tfstate")
			require.NoError(t, os.MkdirAll(filepath.Dir(state), 0o755))
			require.NoError(t, os.WriteFile(state, []byte("existing state"), 0o600))
			config := &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, TerraformDirAbsolutePath: filepath.Join(root, "components")}
			config.Components.Terraform.BasePath = "components"
			provider := NewMockComponentConfigurationRuntime(gomock.NewController(t))
			config.ComponentConfigurations = provider
			binary, err := os.Executable()
			require.NoError(t, err)
			paths := map[string]string{}
			for _, stack := range []string{"dev", "prod"} {
				if mode == "component directory" && stack == "prod" {
					continue
				}
				info := &schema.ConfigAndStacksInfo{Stack: stack, ComponentFromArg: "app", Component: "app", FinalComponent: "app", Command: binary, SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": true}, ComponentEnvSection: map[string]any{}, ComponentSection: map[string]any{"component": "app", "atmos_component": "app", "atmos_stack": stack, "source": map[string]any{"uri": "./module"}}}
				if mode != "component directory" {
					info.ComponentSection["provision"] = map[string]any{"workdir": map[string]any{"enabled": true}}
				}
				if mode == "explicit workdir" {
					info.ComponentSection["metadata"] = map[string]any{"working_directory": filepath.Join(root, "chosen", stack)}
				}
				if mode == "relative workdir" {
					info.ComponentSection["settings"] = map[string]any{"working_directory": filepath.Join("chosen", stack)}
				}
				for _, revision := range []string{"first", "second"} {
					provider.EXPECT().Prepare(gomock.Any(), info).Return(&schema.PreparedComponentConfiguration{Engine: "terraform", Source: source, Inputs: map[string]any{"value": revision}, BackendType: "local", Backend: map[string]any{"path": state}, Workspace: "default", Files: []schema.ComponentConfigurationFile{{Path: "generated.tf", Content: []byte("# {{ literal }} " + revision), IfExists: "overwrite"}}}, nil)
					execution, err := testExecutor(t).prepareComponentExecution(context.Background(), provisioner.OutputWriters{}, config, info, false)
					require.NoError(t, err)
					require.Equal(t, execution.componentPath, execution.workingDir)
					if mode != "component directory" {
						require.Equal(t, execution.workingDir, info.ComponentSection[provWorkdir.WorkdirPathKey])
						require.NotEqual(t, filepath.Join(root, "components", "app"), execution.workingDir)
					}
					if mode == "explicit workdir" || mode == "relative workdir" {
						require.Equal(t, filepath.Join(root, "chosen", stack), execution.workingDir)
					}
					generated, err := os.ReadFile(filepath.Join(execution.workingDir, "generated.tf"))
					require.NoError(t, err)
					require.Equal(t, "# {{ literal }} "+revision, string(generated))
					backend, err := os.ReadFile(filepath.Join(execution.workingDir, "backend.tf.json"))
					require.NoError(t, err)
					var decoded map[string]any
					require.NoError(t, json.Unmarshal(backend, &decoded))
					require.Equal(t, state, decoded["terraform"].(map[string]any)["backend"].(map[string]any)["local"].(map[string]any)["path"])
					contents, err := os.ReadFile(state)
					require.NoError(t, err)
					require.Equal(t, "existing state", string(contents))
					require.NoFileExists(t, filepath.Join(source, "generated.tf"))
					require.NoFileExists(t, filepath.Join(source, "backend.tf.json"))
					if previous := paths[stack]; previous != "" {
						require.Equal(t, previous, execution.workingDir)
					}
					paths[stack] = execution.workingDir
				}
			}
			if mode != "component directory" {
				require.NotEqual(t, paths["dev"], paths["prod"])
			}
		})
	}
}

func TestConfigurationExplicitWorkdirRejectsAnotherOwner(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "module")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "main.tf"), []byte("# source"), 0o644))
	target := filepath.Join(root, "chosen")
	config := &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, TerraformDirAbsolutePath: filepath.Join(root, "components")}
	config.Components.Terraform.BasePath = "components"
	provider := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config.ComponentConfigurations = provider
	binary, err := os.Executable()
	require.NoError(t, err)
	for _, stack := range []string{"dev", "prod"} {
		info := &schema.ConfigAndStacksInfo{Stack: stack, ComponentFromArg: "app", Component: "app", FinalComponent: "app", Command: binary, SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": true}, ComponentEnvSection: map[string]any{}, ComponentSection: map[string]any{"component": "app", "atmos_component": "app", "atmos_stack": stack, "source": map[string]any{"uri": source}, "provision": map[string]any{"workdir": map[string]any{"enabled": true}}, "metadata": map[string]any{"working_directory": target}}}
		provider.EXPECT().Prepare(gomock.Any(), info).Return(&schema.PreparedComponentConfiguration{Engine: "terraform", Source: source, Inputs: map[string]any{}, BackendType: "local", Backend: map[string]any{}, Workspace: "default", Files: []schema.ComponentConfigurationFile{{Path: "generated.tf", Content: []byte(stack), IfExists: "overwrite"}}}, nil)
		_, err := testExecutor(t).prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, config, info, false)
		if stack == "dev" {
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(target, "terraform.tfstate"), []byte("existing state"), 0o600))
		} else {
			require.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
		}
		generated, err := os.ReadFile(filepath.Join(target, "generated.tf"))
		require.NoError(t, err)
		require.Equal(t, "dev", string(generated))
		state, err := os.ReadFile(filepath.Join(target, "terraform.tfstate"))
		require.NoError(t, err)
		require.Equal(t, "existing state", string(state))
	}
}
