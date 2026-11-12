package terraform

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	configurationadapter "github.com/cloudposse/atmos/pkg/plugin/adapter/configuration"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

// sourcePipelineProvider replaces only the RPC boundary; source validation is
// performed by the real Session.Prepare reached through component preparation.
type sourcePipelineProvider struct {
	unit     *pluginv1.DiscoveredComponent
	result   *pluginv1.PrepareComponentResponse
	prepares int
}

func (*sourcePipelineProvider) ComponentType() string { return "terraform" }
func (p *sourcePipelineProvider) Discover(context.Context, *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
	return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{p.unit}}, nil
}

func (p *sourcePipelineProvider) Prepare(context.Context, *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
	p.prepares++
	return p.result, nil
}

func TestConfigurationSourceSymlinkRejectedBeforeInitAndMain(t *testing.T) {
	for _, escaping := range []bool{false, true} {
		name := "in-root source reaches init and validate"
		if escaping {
			name = "escaping source stops before init and validate"
		}
		t.Run(name, func(t *testing.T) {
			project := newReviewedPlanProject(t)
			targetRoot := project.root
			if escaping {
				targetRoot = t.TempDir()
			}
			target := filepath.Join(targetRoot, "inputs.hcl")
			require.NoError(t, os.WriteFile(target, []byte("# source"), 0o600))
			if err := os.Symlink(target, filepath.Join(project.root, "source.hcl")); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			value, err := adapter.EncodeObject(map[string]any{"source": project.module})
			require.NoError(t, err)
			empty, err := adapter.EncodeObject(map[string]any{})
			require.NoError(t, err)
			provider := &sourcePipelineProvider{
				unit:   &pluginv1.DiscoveredComponent{Provider: "terragrunt", Engine: "terraform", Source: "terragrunt.hcl", Address: &pluginv1.ComponentAddress{Stack: "dev", Component: "app", ComponentType: "terraform"}, Configuration: value},
				result: &pluginv1.PrepareComponentResponse{Engine: "terraform", ModuleSource: project.module, Inputs: empty, State: &pluginv1.ComponentStateIdentity{BackendType: "http", Backend: empty, Workspace: "default"}, Sources: []*pluginv1.ComponentSource{{Path: "source.hcl", Kind: pluginv1.ComponentSource_FILE}}},
			}
			project.config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true, Discovery: schema.ComponentDiscoveryConfig{Enabled: true}}}
			session, err := configurationadapter.New(t.Context(), project.config, "validate", func(context.Context, []string) (map[string]configurationadapter.Provider, error) {
				return map[string]configurationadapter.Provider{"terragrunt": provider}, nil
			})
			require.NoError(t, err)
			project.config.ComponentConfigurations = session
			info := project.info(t, "validate")
			shell := &fakeShell{}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			// Use the same preparation and command phases as Executor.execute, keeping
			// implicit init enabled so the passing case proves both phases are reachable.
			execution, err := executor.prepareComponentExecution(t.Context(), provisioner.OutputWriters{}, project.config, info, false)
			if err == nil {
				err = executor.executeCommandPipeline(project.config, info, execution)
			}
			require.Equal(t, 1, provider.prepares, "must reach the real session's preparation boundary")
			if escaping {
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
				require.Nil(t, execution)
				require.Empty(t, shell.calls, "neither Terraform init nor the main command may run")
				return
			}
			require.NoError(t, err)
			var commands []string
			for _, call := range shell.calls {
				if len(call.args) > 0 {
					commands = append(commands, call.args[0])
				}
			}
			require.Contains(t, commands, "init")
			require.Contains(t, commands, "validate")
		})
	}
}
