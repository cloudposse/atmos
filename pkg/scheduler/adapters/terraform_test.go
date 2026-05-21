package adapters

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteTerraformAllUsesGraphBackedSequentialOrder(t *testing.T) {
	stacks := terraformAdapterTestStacks()
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:        true,
			SubCommand: "plan",
		},
		Stacks: stacks,
		Executor: func(info schema.ConfigAndStacksInfo) error {
			executed = append(executed, info.Component+"@"+info.Stack)
			return nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"vpc@dev", "database@dev", "app@dev"}, executed)
}

func TestExecuteTerraformComponentsUsesGraphBackedSequentialOrder(t *testing.T) {
	stacks := terraformAdapterTestStacks()
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			Components: []string{"app", "database", "vpc"},
			Stack:      "dev",
			SubCommand: "plan",
		},
		Stacks: stacks,
		Executor: func(info schema.ConfigAndStacksInfo) error {
			executed = append(executed, info.Component+"@"+info.Stack)
			return nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"vpc@dev", "database@dev", "app@dev"}, executed)
}

func TestExecuteTerraformQueryUsesGraphBackedSequentialOrder(t *testing.T) {
	stacks := terraformAdapterTestStacks()
	var executed []string

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			Query:      ".vars.group == \"selected\"",
			Stack:      "dev",
			SubCommand: "plan",
		},
		Stacks: stacks,
		Executor: func(info schema.ConfigAndStacksInfo) error {
			executed = append(executed, info.Component+"@"+info.Stack)
			return nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"vpc@dev", "database@dev", "app@dev"}, executed)
}

func TestBuildTerraformGraphPrefersDependenciesComponentsOverSettingsDependsOn(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"vpc":      terraformAdapterComponent("selected", nil, nil),
					"database": terraformAdapterComponent("selected", nil, nil),
					"app": terraformAdapterComponent("selected",
						[]any{map[string]any{"component": "vpc"}},
						[]any{map[string]any{"component": "database"}},
					),
				},
			},
		},
	}

	graph, err := BuildTerraformGraph(stacks)
	require.NoError(t, err)

	app, ok := graph.GetNode("app-dev")
	require.True(t, ok)
	require.Equal(t, []string{"vpc-dev"}, app.Dependencies)
}

func TestBuildTerraformGraphFallsBackToSettingsDependsOn(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"vpc": terraformAdapterComponent("selected", nil, nil),
					"app": terraformAdapterComponent("selected",
						nil,
						[]any{map[string]any{"component": "vpc"}},
					),
				},
			},
		},
	}

	graph, err := BuildTerraformGraph(stacks)
	require.NoError(t, err)

	app, ok := graph.GetNode("app-dev")
	require.True(t, ok)
	require.Equal(t, []string{"vpc-dev"}, app.Dependencies)
}

func TestExecuteTerraformSerializesSharedPhysicalComponentPath(t *testing.T) {
	t.Setenv("ATMOS_EXPERIMENTAL_DAG_MAX_CONCURRENCY", "3")

	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"service-api":    terraformAdapterComponentWithPath("selected", "components/terraform/shared-service"),
					"service-worker": terraformAdapterComponentWithPath("selected", "components/terraform/shared-service"),
					"service-cron":   terraformAdapterComponentWithPath("selected", "components/terraform/shared-service"),
				},
			},
		},
	}

	var active atomic.Int32
	var maxActive atomic.Int32

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:        true,
			SubCommand: "plan",
		},
		Stacks: stacks,
		Executor: func(info schema.ConfigAndStacksInfo) error {
			current := active.Add(1)
			updateMaxActive(&maxActive, current)
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			return nil
		},
	})

	require.NoError(t, err)
	require.EqualValues(t, 1, maxActive.Load())
}

func TestExecuteTerraformAllowsParallelDifferentPhysicalComponentPaths(t *testing.T) {
	t.Setenv("ATMOS_EXPERIMENTAL_DAG_MAX_CONCURRENCY", "3")

	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"app":      terraformAdapterComponentWithPath("selected", "components/terraform/app"),
					"database": terraformAdapterComponentWithPath("selected", "components/terraform/database"),
					"vpc":      terraformAdapterComponentWithPath("selected", "components/terraform/vpc"),
				},
			},
		},
	}

	var active atomic.Int32
	var maxActive atomic.Int32

	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			All:        true,
			SubCommand: "plan",
		},
		Stacks: stacks,
		Executor: func(info schema.ConfigAndStacksInfo) error {
			current := active.Add(1)
			updateMaxActive(&maxActive, current)
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			return nil
		},
	})

	require.NoError(t, err)
	require.Greater(t, maxActive.Load(), int32(1))
}

func updateMaxActive(maxActive *atomic.Int32, current int32) {
	for {
		previous := maxActive.Load()
		if current <= previous {
			return
		}
		if maxActive.CompareAndSwap(previous, current) {
			return
		}
	}
}

func terraformAdapterTestStacks() map[string]any {
	return map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"app": terraformAdapterComponent("selected",
						[]any{map[string]any{"component": "database"}},
						nil,
					),
					"database": terraformAdapterComponent("selected",
						[]any{map[string]any{"component": "vpc"}},
						nil,
					),
					"vpc": terraformAdapterComponent("selected", nil, nil),
				},
			},
		},
	}
}

func terraformAdapterComponent(group string, dependenciesComponents []any, settingsDependsOn []any) map[string]any {
	component := map[string]any{
		cfg.MetadataSectionName: map[string]any{
			"component": "mock",
		},
		"vars": map[string]any{
			"group": group,
		},
	}
	if dependenciesComponents != nil {
		component[cfg.DependenciesSectionName] = map[string]any{
			"components": dependenciesComponents,
		}
	}
	if settingsDependsOn != nil {
		component[cfg.SettingsSectionName] = map[string]any{
			"depends_on": settingsDependsOn,
		}
	}
	return component
}

func terraformAdapterComponentWithPath(group string, componentPath string) map[string]any {
	component := terraformAdapterComponent(group, nil, nil)
	component["component_info"] = map[string]any{
		cfg.ComponentPathSectionName: componentPath,
	}
	return component
}
