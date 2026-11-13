package provider

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/engine"
	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	source, err := filepath.Abs(filepath.Join("../../fixtures", name))
	require.NoError(t, err)
	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, os.DirFS(source)))
	root, err = filepath.EvalSymlinks(root)
	require.NoError(t, err)
	return root
}

func TestDiscoveryFixtures(t *testing.T) {
	for _, test := range []struct {
		name, scope string
		names       []string
	}{{"advanced", "live/dev/**", []string{"app", "cache", "database", "network"}}, {"explicit-stack", "live/**", []string{"dev/app", "dev/network", "prod/app", "prod/network"}}, {"explicit-stack", "**", []string{"dev/app", "dev/network", "prod/app", "prod/network"}}} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t, test.name)
			opts, err := encode(map[string]any{"enabled": true, "mode": "native", "discovery": map[string]any{"enabled": true, "included_paths": []string{test.scope}}})
			require.NoError(t, err)
			// Exercise the real evaluator; include its diagnostics only inside local tests.
			p := New(func(ctx context.Context, req model.Request) (*model.Resolution, error) {
				result, err := engine.Resolve(ctx, req)
				require.NoError(t, err)
				return result, err
			})
			request := &pluginv1.DiscoverComponentsRequest{RootDirectory: root, Provider: "terragrunt", ProviderConfiguration: opts, IncludePaths: []string{test.scope}, Operation: &pluginv1.ComponentOperation{Mode: "native", Command: "list"}, Invocation: &pluginv1.Invocation{Environment: map[string]string{"ATMOS_TERRAFORM_BINARY": "must-not-execute"}}}
			response, err := p.Discover(t.Context(), request)
			require.NoError(t, err)
			expected := map[string]map[string]pluginv1.ComponentDependency_Kind{}
			if test.name == "advanced" {
				expected = map[string]map[string]pluginv1.ComponentDependency_Kind{
					"app":      {"network": pluginv1.ComponentDependency_ORDER, "database": pluginv1.ComponentDependency_OUTPUT, "cache": pluginv1.ComponentDependency_OUTPUT},
					"cache":    {"network": pluginv1.ComponentDependency_OUTPUT},
					"database": {"network": pluginv1.ComponentDependency_OUTPUT},
				}
			} else {
				expected = map[string]map[string]pluginv1.ComponentDependency_Kind{
					"dev/app":  {"dev/network": pluginv1.ComponentDependency_OUTPUT},
					"prod/app": {"prod/network": pluginv1.ComponentDependency_OUTPUT},
				}
			}
			names := []string{}
			for _, instance := range response.Components {
				names = append(names, instance.Address.Component)
				require.Equal(t, "terraform", instance.Address.ComponentType)
				require.NotEmpty(t, instance.Sources)
				dependencies := make(map[string]pluginv1.ComponentDependency_Kind)
				for _, dependency := range instance.Dependencies {
					require.NotNil(t, dependency.Target)
					require.Equal(t, instance.Address.Stack, dependency.Target.Stack)
					require.Equal(t, "terraform", dependency.Target.ComponentType)
					require.NotContains(t, dependencies, dependency.Target.Component)
					dependencies[dependency.Target.Component] = dependency.Kind
				}
				require.Len(t, dependencies, len(expected[instance.Address.Component]))
				for target, kind := range expected[instance.Address.Component] {
					require.Contains(t, dependencies, target)
					require.Equal(t, kind, dependencies[target])
				}
			}
			require.Equal(t, test.names, names)
			require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				require.NoError(t, err)
				require.NotEqual(t, ".terragrunt-stack", entry.Name())
				require.NotEqual(t, ".terragrunt-cache", entry.Name())
				return nil
			}))
		})
	}
}

func TestExplicitStackStateAndPreparation(t *testing.T) {
	root := fixture(t, "explicit-stack")
	opts, err := encode(map[string]any{"enabled": true, "mode": "native", "discovery": map[string]any{"enabled": true, "included_paths": []string{"live/**"}}})
	require.NoError(t, err)
	provider := New(nil)
	inventory, err := provider.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{RootDirectory: root, Provider: "terragrunt", ProviderConfiguration: opts, IncludePaths: []string{"live/**"}, Operation: &pluginv1.ComponentOperation{Mode: "native", Command: "plan"}, Invocation: &pluginv1.Invocation{Environment: map[string]string{"ATMOS_TERRAFORM_BINARY": "must-not-execute"}}})
	require.NoError(t, err)
	network := inventory.Components[1]
	require.Equal(t, "dev/network", network.Address.Component)
	request := &pluginv1.ComponentRequest{RootDirectory: root, Provider: "terragrunt", ProviderConfiguration: opts, Address: network.Address, Source: network.Source, Configuration: network.Configuration, Operation: &pluginv1.ComponentOperation{Mode: "native", Command: "plan"}, Invocation: &pluginv1.Invocation{Environment: map[string]string{"ATMOS_TERRAFORM_BINARY": "must-not-execute"}}}
	state, err := provider.Resolve(t.Context(), request)
	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(root, "live/.terragrunt-stack"))
	request.Effects = &pluginv1.ComponentEffects{Commands: true, Network: true, Secrets: true, Writes: true, StateReads: true}
	prepared, err := provider.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.DirExists(t, filepath.Join(root, "live/.terragrunt-stack"))
	require.Equal(t, state.State.WorkingDirectory, prepared.State.WorkingDirectory)
	var before, after map[string]any
	require.NoError(t, decode(state.State.Backend, &before))
	require.NoError(t, decode(prepared.State.Backend, &after))
	require.Equal(t, before, after)
	require.Equal(t, filepath.Join(root, "state/dev/network.tfstate"), after["path"])
	require.True(t, prepared.BackendGenerated)
	require.Len(t, prepared.Files, 1)
	require.Equal(t, "backend.tf", prepared.Files[0].Path)
	require.Contains(t, string(prepared.Files[0].Content), `backend "local"`)
	require.True(t, prepared.Files[0].Sensitive, "the host keeps generated backend bytes out of saved-plan records")
	require.NotNil(t, prepared.Inputs)
}
