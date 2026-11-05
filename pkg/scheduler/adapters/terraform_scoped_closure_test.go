package adapters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	listdeps "github.com/cloudposse/atmos/pkg/list/dependencies"
	"github.com/cloudposse/atmos/pkg/schema"
)

// scopedClosureStacks builds base (core) <- middle (dev) <- leaf (dev) using the
// modern required dependencies.components declaration, with a per-component ci label.
func scopedClosureStacks(baseCI string) map[string]any {
	component := func(ci string, requires ...map[string]any) map[string]any {
		section := map[string]any{
			cfg.MetadataSectionName: map[string]any{
				"component": "mock",
				"labels":    map[string]any{"ci": ci},
			},
			"vars": map[string]any{},
		}
		if len(requires) > 0 {
			components := make([]any, 0, len(requires))
			for _, r := range requires {
				components = append(components, r)
			}
			section[cfg.DependenciesSectionName] = map[string]any{"components": components}
		}
		return section
	}
	stack := func(components map[string]any) map[string]any {
		return map[string]any{
			cfg.ComponentsSectionName: map[string]any{cfg.TerraformSectionName: components},
		}
	}
	return map[string]any{
		"core": stack(map[string]any{"base": component(baseCI)}),
		"dev": stack(map[string]any{
			"middle": component("manual", map[string]any{"component": "base", "stack": "core"}),
			"leaf":   component("auto", map[string]any{"component": "middle"}),
		}),
	}
}

// narrowStacks mimics the describe pipeline: it returns the requested stack
// (or all stacks) restricted to the requested components.
func narrowStacks(full map[string]any, stackName string, components []string) map[string]any {
	result := map[string]any{}
	for name, value := range full {
		if stackName != "" && name != stackName {
			continue
		}
		if len(components) == 0 {
			result[name] = value
			continue
		}
		terraform := value.(map[string]any)[cfg.ComponentsSectionName].(map[string]any)[cfg.TerraformSectionName].(map[string]any)
		narrowed := map[string]any{}
		for _, component := range components {
			if section, ok := terraform[component]; ok {
				narrowed[component] = section
			}
		}
		result[name] = map[string]any{
			cfg.ComponentsSectionName: map[string]any{cfg.TerraformSectionName: narrowed},
		}
	}
	return result
}

// TestExecuteTerraformScopedClosureDescribesEveryExecutedStack proves the
// stacks produced by the scoped describe pass (pkg/list/dependencies) contain
// everything the adapter needs to execute the same selection: the dropped
// intermediate and the dropped seed stay described so strict dependency
// validation and edge contraction succeed.
func TestExecuteTerraformScopedClosureDescribesEveryExecutedStack(t *testing.T) {
	tests := []struct {
		name       string
		baseCI     string
		subCommand string
		expected   []string
	}{
		{"matching seed orders leaf after base", "auto", "plan", []string{"base-core", "leaf-dev"}},
		{"matching seed destroys leaf before base", "auto", "destroy", []string{"leaf-dev", "base-core"}},
		{"non-matching seed still runs its matching dependent", "manual", "plan", []string{"leaf-dev"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			full := scopedClosureStacks(tt.baseCI)
			describe := func(stackName string, components []string, _, _ bool) (map[string]any, error) {
				return narrowStacks(full, stackName, components), nil
			}
			result, err := listdeps.ResolveScopedClosure(describe, &listdeps.ScopeRequest{
				Stack:            "core",
				Labels:           map[string]string{"ci": "auto"},
				Direction:        listdeps.DirectionReverse,
				ProcessTemplates: true,
			})
			require.NoError(t, err)

			var executed []string
			err = ExecuteTerraform(context.Background(), TerraformOptions{
				AtmosConfig: &schema.AtmosConfiguration{},
				Info: &schema.ConfigAndStacksInfo{
					SubCommand:        tt.subCommand,
					Stack:             "core",
					Labels:            map[string]string{"ci": "auto"},
					IncludeDependents: -1,
				},
				Stacks: result.Stacks,
				Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
					executed = append(executed, terraformNodeID(execution.Info.Component, execution.Info.Stack))
					return TerraformExecutionResult{}, nil
				},
			})
			require.NoError(t, err)
			require.Equal(t, tt.expected, executed)

			closureIDs := make([]string, 0, len(result.Closure.Nodes))
			for _, node := range result.Closure.Nodes {
				closureIDs = append(closureIDs, terraformNodeID(node.Component, node.Stack))
			}
			require.ElementsMatch(t, tt.expected, closureIDs, "the closure preview and the executed set must agree")
		})
	}
}
