package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	stackexec "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/naming"
	"github.com/cloudposse/atmos/pkg/schema"
)

func namingContractInputs(t *testing.T, metadata map[string]any) stackexec.StaticStackInputs {
	t.Helper()
	return staticInputsFromMaps(t, map[string]map[string]any{"dev.yaml": {
		"vars":       map[string]any{"namespace": "acme", "tenant": "core", "environment": "ue1", "stage": "dev", "region": "us-east-1", "attributes": []any{"one", "two"}},
		"components": map[string]any{"terraform": map[string]any{"service/app": map[string]any{"metadata": metadata}}},
	}})
}

func TestDoctorWorkspaceRecommendationsBeforeAfter(t *testing.T) {
	for _, token := range []string{"{namespace}", "{tenant}", "{environment}", "{stage}", "{region}", "{attributes}", "{component}"} {
		t.Run(strings.Trim(token, "{}"), func(t *testing.T) {
			config := &schema.AtmosConfiguration{Stacks: schema.Stacks{NamePattern: "{stage}"}}
			before, err := staticNamingResults(context.Background(), config, namingContractInputs(t, map[string]any{"terraform_workspace_pattern": token}))
			require.NoError(t, err)
			replacement, _, unsupported := naming.Upgrade(token)
			require.Empty(t, unsupported)
			after, err := staticNamingResults(context.Background(), config, namingContractInputs(t, map[string]any{"terraform_workspace_template": replacement}))
			require.NoError(t, err)
			require.Len(t, before, 1)
			require.Equal(t, before, after)
			require.NotEmpty(t, after[naming.Identity{Manifest: "dev", ComponentType: "terraform", Component: "service/app"}].Workspace)
		})
	}
}

func TestDoctorMixedTemplateChangesWorkspace(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	original := "{{ .vars.environment }}-{{ .vars.stage }}-{component}"
	replacement, _, _ := naming.Upgrade(original)
	before, err := staticNamingResults(context.Background(), config, namingContractInputs(t, map[string]any{"terraform_workspace_template": original}))
	require.NoError(t, err)
	after, err := staticNamingResults(context.Background(), config, namingContractInputs(t, map[string]any{"terraform_workspace_template": replacement}))
	require.NoError(t, err)
	id := naming.Identity{Manifest: "dev", ComponentType: "terraform", Component: "service/app"}
	require.Equal(t, "ue1-dev-{component}", before[id].Workspace)
	require.Equal(t, "ue1-dev-service-app", after[id].Workspace)
}

func TestDoctorMixedPatternPreservesWorkspace(t *testing.T) {
	for _, scope := range []string{"global", "component"} {
		t.Run(scope, func(t *testing.T) {
			original := "{{ .vars.environment }}-{{ .vars.stage }}-{component}"
			replacement, _, _ := naming.Upgrade(original)
			resolve := func(field, value string) map[naming.Identity]naming.Result {
				metadata := map[string]any{field: value}
				stack := map[string]any{"vars": map[string]any{"environment": "ue1", "stage": "dev"}, "components": map[string]any{"terraform": map[string]any{"service/app": map[string]any{}}}}
				if scope == "global" {
					stack["metadata"] = metadata
				} else {
					stack["components"].(map[string]any)["terraform"].(map[string]any)["service/app"].(map[string]any)["metadata"] = metadata
				}
				config := &schema.AtmosConfiguration{Stacks: schema.Stacks{NamePattern: "{stage}"}}
				config.Templates.Settings.Enabled = true
				result, err := staticNamingResults(context.Background(), config, staticInputsFromMaps(t, map[string]map[string]any{"dev.yaml": stack}))
				require.NoError(t, err)
				return result
			}
			before := resolve("terraform_workspace_pattern", original)
			after := resolve("terraform_workspace_template", replacement)
			require.Equal(t, before, after)
			require.Equal(t, "ue1-dev-service-app", after[naming.Identity{Manifest: "dev", ComponentType: "terraform", Component: "service/app"}].Workspace)
		})
	}
}

func TestDoctorNamingInheritedAliasesAndOverrides(t *testing.T) {
	for _, token := range []string{"{component}", "{base-component}"} {
		t.Run(strings.Trim(token, "{}"), func(t *testing.T) {
			template, _, _ := naming.Upgrade(token)
			resolve := func(field, value string) map[naming.Identity]naming.Result {
				dir := writeStackFixture(t, map[string]string{
					"stacks/catalog/base.yaml": "metadata:\n  " + field + ": 'global-{component}'\ncomponents:\n  terraform:\n    base:\n      metadata:\n        type: abstract\n        component: physical/base\n        " + field + ": '" + value + "'\n",
					"stacks/dev.yaml":          "import: [catalog/*]\nvars:\n  stage: dev\ncomponents:\n  terraform:\n    alias/app:\n      metadata:\n        component: physical/base\n        inherits: [base]\n    override:\n      metadata:\n        inherits: [base]\n        terraform_workspace_template: custom\n",
				})
				result, err := staticNamingResults(context.Background(), staticProjectConfig(dir), stackexec.StaticStackInputs{Files: []string{"dev.yaml"}})
				require.NoError(t, err)
				return result
			}
			before := resolve("terraform_workspace_pattern", token)
			after := resolve("terraform_workspace_template", template)
			require.Equal(t, before, after)
			want := map[string]string{"{component}": "alias-app", "{base-component}": "physical-base"}[token]
			require.Equal(t, want, after[naming.Identity{Manifest: "dev", ComponentType: "terraform", Component: "alias/app"}].Workspace)
			require.Equal(t, "custom", after[naming.Identity{Manifest: "dev", ComponentType: "terraform", Component: "override"}].Workspace)
		})
	}
}

func TestDoctorStackAndClusterRecommendationsBeforeAfter(t *testing.T) {
	for _, token := range []string{"{namespace}", "{tenant}", "{environment}", "{stage}"} {
		t.Run("stack/"+token, func(t *testing.T) {
			config := &schema.AtmosConfiguration{Stacks: schema.Stacks{NamePattern: token}}
			inputs := namingContractInputs(t, nil)
			before, err := staticNamingResults(context.Background(), config, inputs)
			require.NoError(t, err)
			config.Stacks.NameTemplate, _, _ = naming.Upgrade(token)
			config.Stacks.NamePattern = ""
			after, err := staticNamingResults(context.Background(), config, inputs)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
	for _, token := range []string{"{namespace}", "{tenant}", "{environment}", "{stage}", "{region}", "{attributes}"} {
		t.Run("cluster/"+token, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			config.Components.Helmfile.ClusterNamePattern = token
			inputs := staticInputsFromMaps(t, map[string]map[string]any{"dev.yaml": {
				"vars":       map[string]any{"namespace": "acme", "tenant": "core", "environment": "ue1", "stage": "dev", "region": "us-east-1", "attributes": []any{"one", "two"}},
				"components": map[string]any{"helmfile": map[string]any{"app": map[string]any{}}},
			}})
			before, err := staticNamingResults(context.Background(), config, inputs)
			require.NoError(t, err)
			config.Components.Helmfile.ClusterNameTemplate, _, _ = naming.Upgrade(token)
			config.Components.Helmfile.ClusterNamePattern = ""
			after, err := staticNamingResults(context.Background(), config, inputs)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestDoctorGlobalWorkspaceTemplateInheritance(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	inputs := staticInputsFromMaps(t, map[string]map[string]any{"dev.yaml": {
		"metadata": map[string]any{"terraform_workspace_template": "global-{{ .atmos_component }}"},
		"components": map[string]any{"terraform": map[string]any{
			"app":      map[string]any{},
			"override": map[string]any{"metadata": map[string]any{"terraform_workspace_template": "custom"}},
		}},
	}})
	result, err := staticNamingResults(context.Background(), config, inputs)
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.Equal(t, "global-app", result[naming.Identity{Manifest: "dev", ComponentType: "terraform", Component: "app"}].Workspace)
	require.Equal(t, "custom", result[naming.Identity{Manifest: "dev", ComponentType: "terraform", Component: "override"}].Workspace)
}
