package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/engine"
	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// Scheduling edges must be available from discovery, before preparation can read outputs.
func TestDiscoveryOutputDependencyOrdering(t *testing.T) {
	for _, test := range []struct {
		name, blocks string
		kinds        []pluginv1.ComponentDependency_Kind
	}{
		{"output only", `dependency "network" { config_path = "../network" }`, []pluginv1.ComponentDependency_Kind{pluginv1.ComponentDependency_OUTPUT}},
		{"output and ordering", `dependency "network" { config_path = "../network" }
dependencies { paths = ["../network", "../network"] }`, []pluginv1.ComponentDependency_Kind{pluginv1.ComponentDependency_OUTPUT}},
		{"ordering only", `dependencies { paths = ["../network"] }`, []pluginv1.ComponentDependency_Kind{pluginv1.ComponentDependency_ORDER}},
		{"skip outputs", `dependency "network" {
 config_path = "../network"
 skip_outputs = true
}`, []pluginv1.ComponentDependency_Kind{pluginv1.ComponentDependency_ORDER}},
		{"disabled", `dependency "network" {
 config_path = "../network"
 enabled = false
}`, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			for _, dir := range []string{"module", "live/app", "live/network"} {
				require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o755))
			}
			config := "terraform { source = \"../../module\" }\n"
			require.NoError(t, os.WriteFile(filepath.Join(root, "live/network/terragrunt.hcl"), []byte(config), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "live/app/terragrunt.hcl"), []byte(config+test.blocks+"\ninputs = { deferred = run_cmd(\"must-not-execute\") }\n"), 0o600))
			opts, err := encode(map[string]any{"enabled": true, "mode": "native", "discovery": map[string]any{"enabled": true}})
			require.NoError(t, err)
			var phases []string
			p := New(func(ctx context.Context, req model.Request) (*model.Resolution, error) {
				phases = append(phases, req.Phase)
				return engine.Resolve(ctx, req)
			})
			response, err := p.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{
				RootDirectory: root, Provider: "terragrunt", ProviderConfiguration: opts,
				IncludePaths: []string{"live/**"}, Operation: &pluginv1.ComponentOperation{Mode: "native", Command: "plan"},
				Invocation: &pluginv1.Invocation{Environment: map[string]string{"ATMOS_TERRAFORM_BINARY": "must-not-execute"}},
			})
			require.NoError(t, err)
			require.Empty(t, response.Diagnostics)
			require.Equal(t, []string{"inventory"}, phases)
			require.Len(t, response.Components, 2)
			app := response.Components[0]
			network := response.Components[1]
			require.Equal(t, "app", app.Address.Component)
			require.Equal(t, "network", network.Address.Component)
			require.Len(t, app.Dependencies, len(test.kinds))
			for i, kind := range test.kinds {
				require.Equal(t, kind, app.Dependencies[i].Kind)
				require.Equal(t, network.Address, app.Dependencies[i].Target)
			}
			require.Empty(t, network.Dependencies)
			require.NoDirExists(t, filepath.Join(root, "live/app/.terragrunt-cache"))
		})
	}
}
