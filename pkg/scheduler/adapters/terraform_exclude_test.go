package adapters

import (
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard: the ownership marker these tests write is the stack processor's key.
var _ = schema.ConfigurationOwnedKey

// freeFormExclude is user data that happens to be named `settings.terragrunt.exclude`, for
// example migration notes. It sets `if: false`, which only the Terragrunt provider evaluates.
func freeFormExclude() map[string]any {
	return map[string]any{"if": false, "actions": []any{"plan", "apply", "all"}, "exclude_dependencies": true}
}

// settingsWithExclude returns component settings holding an exclude block and the given
// `settings.configuration` section (nil leaves the section out).
func settingsWithExclude(configuration map[string]any) map[string]any {
	settings := map[string]any{"terragrunt": map[string]any{"exclude": freeFormExclude()}}
	if configuration != nil {
		settings["configuration"] = configuration
	}
	return settings
}

func TestExecuteTerraformIgnoresExcludeBlockOnComponentsTheProviderDoesNotOwn(t *testing.T) {
	for _, test := range []struct {
		name          string
		configuration map[string]any
	}{
		{"plain component", nil},
		{"provider named without the ownership marker", map[string]any{"provider": terraformExcludeProvider}},
		{"marker set to a non-boolean value", map[string]any{"provider": terraformExcludeProvider, schema.ConfigurationOwnedKey: "true"}},
		{"owned by another configuration provider", map[string]any{"provider": "other", schema.ConfigurationOwnedKey: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			stacks := terraformAdapterTestStacks()
			components := stacks["dev"].(map[string]any)[cfg.ComponentsSectionName].(map[string]any)[cfg.TerraformSectionName].(map[string]any)
			components["database"].(map[string]any)[cfg.SettingsSectionName] = settingsWithExclude(test.configuration)
			for _, subCommand := range []string{"plan", "apply", "destroy"} {
				executed := runExcluded(t, stacks, &schema.ConfigAndStacksInfo{All: true, SubCommand: subCommand, AdditionalArgsAndFlags: []string{"-auto-approve"}})
				require.ElementsMatch(t, []string{"vpc", "database", "app"}, executed,
					"%s --all runs a component whose free-form settings contain terragrunt.exclude", subCommand)
			}
		})
	}
}

func TestTerraformExcludeBlockRequiresTerragruntOwnership(t *testing.T) {
	block := freeFormExclude()
	owned := map[string]any{"provider": terraformExcludeProvider, schema.ConfigurationOwnedKey: true}
	node := func(settings map[string]any) *dependency.Node {
		return &dependency.Node{Metadata: map[string]any{cfg.SettingsSectionName: settings}}
	}
	require.Equal(t, block, terraformExcludeBlock(node(map[string]any{"terragrunt": map[string]any{"exclude": block}, "configuration": owned})),
		"a unit the Terragrunt provider owns keeps its exclude block")
	require.Nil(t, terraformExcludeBlock(node(settingsWithExclude(nil))), "a plain component has no provider exclude block")
	require.Nil(t, terraformExcludeBlock(node(settingsWithExclude(map[string]any{"provider": "other", schema.ConfigurationOwnedKey: true}))))
	require.Nil(t, terraformExcludeBlock(node(map[string]any{"configuration": owned})), "an owned unit without a block has none")
	require.Nil(t, terraformExcludeBlock(node(nil)))
	require.Nil(t, terraformExcludeBlock(nil))
}
