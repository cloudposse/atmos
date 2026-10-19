package stack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

const inventoryAtmosYAML = `base_path: .
stacks:
  base_path: stacks
  included_paths: ["**/*"]
components:
  terraform:
    base_path: components/terraform
commands:
  - name: inventorykind
    description: Operate on inventorykind components
    commands:
      - name: deploy
        description: Deploy an inventorykind component
        component:
          type: inventorykind
          base_path: components/custom-inventory
        arguments:
          - name: component
            provides: component
            required: true
        flags:
          - name: stack
            provides: stack
            required: true
        steps:
          - echo deployed
      - name: smoke
        description: A second command declaring the same kind without a base path
        component:
          type: inventorykind
        steps:
          - echo smoke
`

const inventoryStackYAML = `components:
  terraform:
    network:
      vars:
        region: us-east-2
  inventorykind:
    smoke:
      dependencies:
        components:
          - kind: terraform
            component: network
`

func inventoryFixture(t *testing.T) (*cobra.Command, *schema.AtmosConfiguration) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	require.NoError(t, os.WriteFile("atmos.yaml", []byte(inventoryAtmosYAML), 0o644))
	require.NoError(t, os.MkdirAll("stacks", 0o755))
	require.NoError(t, os.WriteFile(filepath.Join("stacks", "dev.yaml"), []byte(inventoryStackYAML), 0o644))
	e.ClearFindStacksMapCache()
	t.Cleanup(e.ClearFindStacksMapCache)

	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	root := &cobra.Command{Use: "atmos"}
	group := &cobra.Command{Use: "inventorykind"}
	group.AddCommand(&cobra.Command{Use: "deploy", Annotations: map[string]string{"customCommand": "true"}})
	root.AddCommand(group)
	return group, &config
}

func TestLoadLifecycleInventoryIncludesCustomKinds(t *testing.T) {
	group, config := inventoryFixture(t)

	stacks, handlers, err := loadLifecycleInventory(group, config, "deploy", nil)
	require.NoError(t, err)

	// The custom kind was registered and its handler discovered before stacks were described.
	_, registered := component.GetProvider("inventorykind")
	assert.True(t, registered)
	require.Contains(t, handlers, "inventorykind")
	assert.Equal(t, []string{"inventorykind", "deploy"}, handlers["inventorykind"].Path)

	components := stacks["dev"].(map[string]any)["components"].(map[string]any)
	require.Contains(t, components, "terraform")
	smoke := components["inventorykind"].(map[string]any)["smoke"].(map[string]any)
	deps := smoke["dependencies"].(map[string]any)["components"].([]any)
	require.Len(t, deps, 1)
	assert.Equal(t, "network", deps[0].(map[string]any)["component"])

	// The plan orders the custom component after the terraform component it depends on.
	left := "{{"
	handlers["terraform"] = lifecycle.NativeHandler("terraform", "deploy")
	plan, err := lifecycle.BuildPlan(stacks, handlers, lifecycle.Selection{}, left)
	require.NoError(t, err)
	order, err := plan.Graph.TopologicalSort()
	require.NoError(t, err)
	labels := make([]string, 0, len(order))
	for i := range order {
		labels = append(labels, lifecycle.Label(&order[i]))
	}
	assert.Equal(t, []string{"terraform/dev/network", "inventorykind/dev/smoke"}, labels)
}

func TestRegisterCustomComponentKindsBasePaths(t *testing.T) {
	config := &schema.AtmosConfiguration{
		Commands: []schema.Command{
			{Name: "a", Commands: []schema.Command{
				{Name: "bare", Component: &schema.CommandComponent{Type: "kindfirst"}},
				{Name: "declared", Component: &schema.CommandComponent{Type: "kindfirst", BasePath: "custom/first"}},
			}},
			{Name: "b", Component: &schema.CommandComponent{Type: "kindsecond"}},
		},
	}
	declared := map[string]string{}
	collectCustomComponentKinds(config.Commands, declared)
	assert.Equal(t, map[string]string{"kindfirst": "custom/first", "kindsecond": ""}, declared)
	assert.Equal(t, "custom/first", customKindBasePath(config, "kindfirst", declared["kindfirst"]))
	assert.Equal(t, filepath.ToSlash(filepath.Join("components", "kindsecond")), customKindBasePath(config, "kindsecond", ""))

	config.Components.Plugins = map[string]any{"kindsecond": map[string]any{"base_path": "plugins/second"}}
	assert.Equal(t, "plugins/second", customKindBasePath(config, "kindsecond", ""))
}
