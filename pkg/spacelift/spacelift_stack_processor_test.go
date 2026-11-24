package spacelift

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestSpaceliftStackProcessor(t *testing.T) {
	processStackDeps := true
	processComponentDeps := true
	processImports := true
	stackConfigPathTemplate := "stacks/%s.yaml"

	spaceliftStacks, err := CreateSpaceliftStacks(
		"",
		"",
		"",
		"",
		"",
		nil,
		processStackDeps,
		processComponentDeps,
		processImports,
		stackConfigPathTemplate,
	)

	assert.Nil(t, err)
	assert.Equal(t, 47, len(spaceliftStacks))

	tenant1Ue2DevInfraVpcStack := spaceliftStacks["tenant1-ue2-dev-infra-vpc"].(map[string]any)
	tenant1Ue2DevInfraVpcStackInfrastructureStackName := tenant1Ue2DevInfraVpcStack["stack"].(string)
	tenant1Ue2DevInfraVpcStackBackend := tenant1Ue2DevInfraVpcStack["backend"].(map[string]any)
	tenant1Ue2DevInfraVpcStackBackendWorkspaceKeyPrefix := tenant1Ue2DevInfraVpcStackBackend["workspace_key_prefix"].(string)
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevInfraVpcStackInfrastructureStackName)
	assert.Equal(t, "infra-vpc", tenant1Ue2DevInfraVpcStackBackendWorkspaceKeyPrefix)

	// Test having a dash `-` in tenant/environment/stage names
	tenant1Ue2Test1InfraVpcStack := spaceliftStacks["tenant1-ue2-test-1-infra-vpc"].(map[string]any)
	tenant1Ue2Test1InfraVpcStackInfrastructureStackName := tenant1Ue2Test1InfraVpcStack["stack"].(string)
	tenant1Ue2Test1InfraVpcStackBackend := tenant1Ue2Test1InfraVpcStack["backend"].(map[string]any)
	tenant1Ue2Test1InfraVpcStackBackendWorkspaceKeyPrefix := tenant1Ue2Test1InfraVpcStackBackend["workspace_key_prefix"].(string)
	assert.Equal(t, "tenant1-ue2-test-1", tenant1Ue2Test1InfraVpcStackInfrastructureStackName)
	assert.Equal(t, "infra-vpc", tenant1Ue2Test1InfraVpcStackBackendWorkspaceKeyPrefix)

	tenant1Ue2DevTestTestComponentOverrideComponent := spaceliftStacks["tenant1-ue2-dev-test-test-component-override"].(map[string]any)
	tenant1Ue2DevTestTestComponentOverrideComponentInfrastructureStackName := tenant1Ue2DevTestTestComponentOverrideComponent["stack"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentBackend := tenant1Ue2DevTestTestComponentOverrideComponent["backend"].(map[string]any)
	tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent := tenant1Ue2DevTestTestComponentOverrideComponent["base_component"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentOverrideComponentBackend["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentDeps := tenant1Ue2DevTestTestComponentOverrideComponent["deps"].([]string)
	tenant1Ue2DevTestTestComponentOverrideComponentLabels := tenant1Ue2DevTestTestComponentOverrideComponent["labels"].([]string)
	tenant1Ue2DevTestTestComponentOverrideTerraformWorkspace := tenant1Ue2DevTestTestComponentOverrideComponent["workspace"]

	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevTestTestComponentOverrideComponentInfrastructureStackName)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "test/test-component", tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent)
	assert.Equal(t, "test-component-override-workspace-override", tenant1Ue2DevTestTestComponentOverrideTerraformWorkspace)

	assert.Equal(t, 10, len(tenant1Ue2DevTestTestComponentOverrideComponentDeps))
	assert.Equal(t, "catalog/terraform/services/service-1-override", tenant1Ue2DevTestTestComponentOverrideComponentDeps[0])
	assert.Equal(t, "catalog/terraform/services/service-2-override", tenant1Ue2DevTestTestComponentOverrideComponentDeps[1])
	assert.Equal(t, "catalog/terraform/spacelift-and-backend-override-1", tenant1Ue2DevTestTestComponentOverrideComponentDeps[2])
	assert.Equal(t, "catalog/terraform/test-component", tenant1Ue2DevTestTestComponentOverrideComponentDeps[3])
	assert.Equal(t, "catalog/terraform/test-component-override", tenant1Ue2DevTestTestComponentOverrideComponentDeps[4])
	assert.Equal(t, "mixins/region/us-east-2", tenant1Ue2DevTestTestComponentOverrideComponentDeps[5])
	assert.Equal(t, "mixins/stage/dev", tenant1Ue2DevTestTestComponentOverrideComponentDeps[6])
	assert.Equal(t, "orgs/cp/_defaults", tenant1Ue2DevTestTestComponentOverrideComponentDeps[7])
	assert.Equal(t, "orgs/cp/tenant1/_defaults", tenant1Ue2DevTestTestComponentOverrideComponentDeps[8])
	assert.Equal(t, "orgs/cp/tenant1/dev/us-east-2", tenant1Ue2DevTestTestComponentOverrideComponentDeps[9])

	assert.Equal(t, 39, len(tenant1Ue2DevTestTestComponentOverrideComponentLabels))
	assert.Equal(t, "deps:stacks/catalog/terraform/services/service-1-override.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[27])
	assert.Equal(t, "deps:stacks/catalog/terraform/services/service-2-override.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[28])
	assert.Equal(t, "deps:stacks/catalog/terraform/spacelift-and-backend-override-1.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[29])
	assert.Equal(t, "deps:stacks/catalog/terraform/test-component.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[30])
	assert.Equal(t, "deps:stacks/catalog/terraform/test-component-override.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[31])
	assert.Equal(t, "deps:stacks/mixins/region/us-east-2.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[32])
	assert.Equal(t, "deps:stacks/mixins/stage/dev.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[33])
	assert.Equal(t, "deps:stacks/orgs/cp/_defaults.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[34])
	assert.Equal(t, "deps:stacks/orgs/cp/tenant1/_defaults.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[35])
	assert.Equal(t, "deps:stacks/orgs/cp/tenant1/dev/us-east-2.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[36])
	assert.Equal(t, "folder:component/test/test-component-override", tenant1Ue2DevTestTestComponentOverrideComponentLabels[37])
	assert.Equal(t, "folder:tenant1/ue2/dev", tenant1Ue2DevTestTestComponentOverrideComponentLabels[38])

	newTenant1Ue2DevTestTestComponentOverrideComponent2 := spaceliftStacks["tenant1-ue2-dev-new-component"].(map[string]any)
	newTenant1Ue2DevTestTestComponentOverrideComponent2InfrastructureStackName := newTenant1Ue2DevTestTestComponentOverrideComponent2["stack"].(string)
	assert.Equal(t, "tenant1-ue2-dev", newTenant1Ue2DevTestTestComponentOverrideComponent2InfrastructureStackName)

	// Test having a dash `-` in tenant/environment/stage names
	newTenant1Ue2Test1TestTestComponentOverrideComponent2 := spaceliftStacks["tenant1-ue2-test-1-new-component"].(map[string]any)
	newTenant1Ue2Test1TestTestComponentOverrideComponent2InfrastructureStackName := newTenant1Ue2Test1TestTestComponentOverrideComponent2["stack"].(string)
	assert.Equal(t, "tenant1-ue2-test-1", newTenant1Ue2Test1TestTestComponentOverrideComponent2InfrastructureStackName)

	// Test `settings.depends_on`
	tenant1Ue2ProdTopLevelComponent1 := spaceliftStacks["tenant1-ue2-prod-top-level-component1"].(map[string]any)
	tenant1Ue2ProdTopLevelComponent1Labels := tenant1Ue2ProdTopLevelComponent1["labels"].([]string)
	assert.Equal(t, "depends-on:tenant1-ue2-dev-test-test-component", tenant1Ue2ProdTopLevelComponent1Labels[35])
	assert.Equal(t, "depends-on:tenant1-ue2-prod-test-test-component-override", tenant1Ue2ProdTopLevelComponent1Labels[36])

	yamlSpaceliftStacks, err := u.ConvertToYAML(spaceliftStacks)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if yamlSpaceliftStacks != "" {
				t.Logf("Spacelift stacks:\n%s", yamlSpaceliftStacks)
			} else {
				t.Logf("Spacelift stacks (raw): %+v", spaceliftStacks)
			}
		}
	})
}

func TestLegacySpaceliftStackProcessor(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/complete/stacks"
	terraformComponentsBasePath := "../../tests/fixtures/scenarios/complete/components/terraform"
	helmfileComponentsBasePath := "../../tests/fixtures/scenarios/complete/components/helmfile"

	filePaths := []string{
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant1/dev/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant1/prod/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant1/staging/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant1/test1/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant2/dev/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant2/prod/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant2/staging/us-east-2.yaml",
	}

	processStackDeps := true
	processComponentDeps := true
	processImports := true
	stackConfigPathTemplate := "stacks/%s.yaml"

	spaceliftStacks, err := CreateSpaceliftStacks(
		stacksBasePath,
		terraformComponentsBasePath,
		helmfileComponentsBasePath,
		"",
		"",
		filePaths,
		processStackDeps,
		processComponentDeps,
		processImports,
		stackConfigPathTemplate,
	)

	assert.Nil(t, err)
	assert.Equal(t, 44, len(spaceliftStacks))

	tenant1Ue2DevInfraVpcStack := spaceliftStacks["orgs-cp-tenant1-dev-us-east-2-infra-vpc"].(map[string]any)
	tenant1Ue2DevInfraVpcStackBackend := tenant1Ue2DevInfraVpcStack["backend"].(map[string]any)
	tenant1Ue2DevInfraVpcStackBackendWorkspaceKeyPrefix := tenant1Ue2DevInfraVpcStackBackend["workspace_key_prefix"].(string)
	assert.Equal(t, "infra-vpc", tenant1Ue2DevInfraVpcStackBackendWorkspaceKeyPrefix)

	tenant1Ue2DevTestTestComponentOverrideComponent := spaceliftStacks["orgs-cp-tenant1-dev-us-east-2-test-test-component-override"].(map[string]any)
	tenant1Ue2DevTestTestComponentOverrideComponentBackend := tenant1Ue2DevTestTestComponentOverrideComponent["backend"].(map[string]any)
	tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent := tenant1Ue2DevTestTestComponentOverrideComponent["base_component"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentOverrideComponentBackend["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentDeps := tenant1Ue2DevTestTestComponentOverrideComponent["deps"].([]string)
	tenant1Ue2DevTestTestComponentOverrideComponentLabels := tenant1Ue2DevTestTestComponentOverrideComponent["labels"].([]string)
	tenant1Ue2DevTestTestComponentOverrideTerraformWorkspace := tenant1Ue2DevTestTestComponentOverrideComponent["workspace"]

	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "test/test-component", tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent)
	assert.Equal(t, "test-component-override-workspace-override", tenant1Ue2DevTestTestComponentOverrideTerraformWorkspace)

	assert.Equal(t, 10, len(tenant1Ue2DevTestTestComponentOverrideComponentDeps))
	assert.Equal(t, "catalog/terraform/services/service-1-override", tenant1Ue2DevTestTestComponentOverrideComponentDeps[0])
	assert.Equal(t, "catalog/terraform/services/service-2-override", tenant1Ue2DevTestTestComponentOverrideComponentDeps[1])
	assert.Equal(t, "catalog/terraform/spacelift-and-backend-override-1", tenant1Ue2DevTestTestComponentOverrideComponentDeps[2])
	assert.Equal(t, "catalog/terraform/test-component", tenant1Ue2DevTestTestComponentOverrideComponentDeps[3])
	assert.Equal(t, "catalog/terraform/test-component-override", tenant1Ue2DevTestTestComponentOverrideComponentDeps[4])
	assert.Equal(t, "mixins/region/us-east-2", tenant1Ue2DevTestTestComponentOverrideComponentDeps[5])
	assert.Equal(t, "mixins/stage/dev", tenant1Ue2DevTestTestComponentOverrideComponentDeps[6])
	assert.Equal(t, "orgs/cp/_defaults", tenant1Ue2DevTestTestComponentOverrideComponentDeps[7])
	assert.Equal(t, "orgs/cp/tenant1/_defaults", tenant1Ue2DevTestTestComponentOverrideComponentDeps[8])
	assert.Equal(t, "orgs/cp/tenant1/dev/us-east-2", tenant1Ue2DevTestTestComponentOverrideComponentDeps[9])

	assert.Equal(t, 39, len(tenant1Ue2DevTestTestComponentOverrideComponentLabels))
	assert.Equal(t, "deps:stacks/catalog/terraform/services/service-1-override.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[27])
	assert.Equal(t, "deps:stacks/catalog/terraform/services/service-2-override.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[28])
	assert.Equal(t, "deps:stacks/catalog/terraform/spacelift-and-backend-override-1.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[29])
	assert.Equal(t, "deps:stacks/catalog/terraform/test-component.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[30])
	assert.Equal(t, "deps:stacks/catalog/terraform/test-component-override.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[31])
	assert.Equal(t, "deps:stacks/mixins/region/us-east-2.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[32])
	assert.Equal(t, "deps:stacks/mixins/stage/dev.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[33])
	assert.Equal(t, "deps:stacks/orgs/cp/_defaults.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[34])
	assert.Equal(t, "deps:stacks/orgs/cp/tenant1/_defaults.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[35])
	assert.Equal(t, "deps:stacks/orgs/cp/tenant1/dev/us-east-2.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[36])
	assert.Equal(t, "folder:component/test/test-component-override", tenant1Ue2DevTestTestComponentOverrideComponentLabels[37])

	yamlSpaceliftStacks, err := u.ConvertToYAML(spaceliftStacks)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if yamlSpaceliftStacks != "" {
				t.Logf("Spacelift stacks:\n%s", yamlSpaceliftStacks)
			} else {
				t.Logf("Spacelift stacks (raw): %+v", spaceliftStacks)
			}
		}
	})
}

func TestTransformStackConfigToSpaceliftStacks_DuplicateStackName(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Two different stacks, each with a component whose `settings.spacelift.stack_name`
	// explicitly resolves to the same literal name -- this must be rejected rather than
	// silently overwriting the first stack's config.
	stacks := map[string]any{
		"stackA": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"compA": map[string]any{
						"vars": map[string]any{},
						"settings": map[string]any{
							"spacelift": map[string]any{
								"workspace_enabled": true,
								"stack_name":        "dup-stack",
							},
						},
					},
				},
			},
		},
		"stackB": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"compB": map[string]any{
						"vars": map[string]any{},
						"settings": map[string]any{
							"spacelift": map[string]any{
								"workspace_enabled": true,
								"stack_name":        "dup-stack",
							},
						},
					},
				},
			},
		},
	}

	_, err := TransformStackConfigToSpaceliftStacks(atmosConfig, stacks, "stacks/%s.yaml", "", "", false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Duplicate Spacelift stack name")
	assert.Contains(t, err.Error(), "dup-stack")
}

func TestTransformStackConfigToSpaceliftStacks_DependsOnNameTemplateResolvesRegion(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	stacks := map[string]any{
		"orgs/cp/tenant1/dev/us-east-2": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"comp1": map[string]any{
						"vars": map[string]any{
							"tenant":      "tenant1",
							"environment": "ue2",
							"stage":       "dev",
							"region":      "us-east-2",
						},
						"settings": map[string]any{
							"spacelift": map[string]any{"workspace_enabled": true},
							"depends_on": map[string]any{
								"1": map[string]any{"component": "comp2"},
							},
						},
					},
					// comp2 deliberately uses a DIFFERENT region than comp1. If the
					// depends_on resolution path used comp2's own vars instead of
					// defaulting from comp1's context, the computed depends-on name
					// would use "us-west-2" and coincidentally match comp2's own
					// resolved name -- masking the bug. Using distinct regions forces
					// the depends-on name to only match if it's genuinely derived from
					// comp1's context (region "us-east-2"), which comp2 doesn't share,
					// so the lookup correctly fails and the error content proves which
					// region was actually used.
					"comp2": map[string]any{
						"vars": map[string]any{
							"tenant":      "tenant1",
							"environment": "ue2",
							"stage":       "dev",
							"region":      "us-west-2",
						},
						"settings": map[string]any{
							"spacelift": map[string]any{"workspace_enabled": true},
						},
					},
				},
			},
		},
	}

	// name_template references `.vars.region`. The `settings.depends_on` resolution path
	// defaults namespace/tenant/environment/stage/region from the *depending* component's
	// own context (comp1) when the depends_on entry doesn't declare them explicitly, so the
	// template renders successfully (no missing-key error) for the depends_on label too,
	// not just the component's own stack name.
	stackNameTemplate := "{{.vars.tenant}}-{{.vars.region}}"

	_, err := TransformStackConfigToSpaceliftStacks(atmosConfig, stacks, "stacks/%s.yaml", stackNameTemplate, "", false, nil)

	// comp2's own resolved name is "tenant1-us-west-2-comp2" (its own region), which does
	// not match "tenant1-us-east-2-comp2" (comp1's region) -- so the depends_on lookup
	// fails to find a match. That failure, naming the comp1-region-derived stack, is the
	// proof that region was defaulted from comp1's context and not silently pulled from
	// comp2's own vars (which would have rendered fine and hidden the distinction).
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant1-us-east-2")
	assert.NotContains(t, err.Error(), "tenant1-us-west-2")
}

// dependsOnTestStacks builds two stacks (`tenant1-ue1-dev` and `tenant1-uw2-dev`) with a `vpc` component in each and an
// `app` component in `tenant1-ue1-dev` whose dependency sections are supplied by the caller.
func dependsOnTestStacks(appSettings, appDependencies map[string]any) map[string]any {
	vars := func(environment string) map[string]any {
		return map[string]any{"tenant": "tenant1", "environment": environment, "stage": "dev"}
	}
	enabled := map[string]any{"spacelift": map[string]any{"workspace_enabled": true}}

	app := map[string]any{"vars": vars("ue1"), "settings": enabled}
	for k, v := range appSettings {
		app["settings"].(map[string]any)[k] = v
	}
	if appDependencies != nil {
		app["dependencies"] = appDependencies
	}

	return map[string]any{
		"orgs/tenant1/dev/ue1": map[string]any{"components": map[string]any{"terraform": map[string]any{
			"vpc": map[string]any{"vars": vars("ue1"), "settings": enabled},
			"app": app,
		}}},
		"orgs/tenant1/dev/uw2": map[string]any{"components": map[string]any{"terraform": map[string]any{
			"vpc": map[string]any{"vars": vars("uw2"), "settings": enabled},
		}}},
	}
}

func dependsOnLabels(t *testing.T, atmosConfig *schema.AtmosConfiguration, stacks map[string]any) []string {
	t.Helper()

	res, err := TransformStackConfigToSpaceliftStacks(atmosConfig, stacks, "stacks/%s.yaml", "{{.vars.tenant}}-{{.vars.environment}}-{{.vars.stage}}", "", false, nil)
	require.NoError(t, err)

	app, ok := res["tenant1-ue1-dev-app"].(map[string]any)
	require.True(t, ok, "expected the Spacelift stack tenant1-ue1-dev-app, got: %v", res)

	var labels []string
	for _, label := range app["labels"].([]string) {
		if strings.HasPrefix(label, "depends-on:") {
			labels = append(labels, label)
		}
	}
	return labels
}

func TestTransformStackConfigToSpaceliftStacks_DependenciesComponents(t *testing.T) {
	templatesEnabled := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true}}}

	t.Run("omitted stack depends on the component in the same stack", func(t *testing.T) {
		stacks := dependsOnTestStacks(nil, map[string]any{
			"components": []any{map[string]any{"name": "vpc"}},
		})
		assert.Equal(t, []string{"depends-on:tenant1-ue1-dev-vpc"}, dependsOnLabels(t, templatesEnabled, stacks))
	})

	t.Run("literal stack depends on the component in another stack", func(t *testing.T) {
		stacks := dependsOnTestStacks(nil, map[string]any{
			"components": []any{map[string]any{"component": "vpc", "stack": "tenant1-uw2-dev"}},
		})
		assert.Equal(t, []string{"depends-on:tenant1-uw2-dev-vpc"}, dependsOnLabels(t, templatesEnabled, stacks))
	})

	t.Run("stack template is rendered from the component's own vars", func(t *testing.T) {
		stacks := dependsOnTestStacks(nil, map[string]any{
			"components": []any{map[string]any{"name": "vpc", "stack": "{{ .vars.tenant }}-uw2-{{ .vars.stage }}"}},
		})
		assert.Equal(t, []string{"depends-on:tenant1-uw2-dev-vpc"}, dependsOnLabels(t, templatesEnabled, stacks))
	})

	t.Run("dependencies.components takes precedence over settings.depends_on", func(t *testing.T) {
		stacks := dependsOnTestStacks(
			map[string]any{"depends_on": map[string]any{"1": map[string]any{"component": "vpc", "environment": "uw2"}}},
			map[string]any{"components": []any{map[string]any{"name": "vpc"}}},
		)
		assert.Equal(t, []string{"depends-on:tenant1-ue1-dev-vpc"}, dependsOnLabels(t, templatesEnabled, stacks))
	})

	t.Run("settings.depends_on still works when dependencies.components is not declared", func(t *testing.T) {
		stacks := dependsOnTestStacks(
			map[string]any{"depends_on": map[string]any{"1": map[string]any{"component": "vpc", "environment": "uw2"}}},
			nil,
		)
		assert.Equal(t, []string{"depends-on:tenant1-uw2-dev-vpc"}, dependsOnLabels(t, templatesEnabled, stacks))
	})

	t.Run("settings.depends_on is used when dependencies.components has no Terraform component entries", func(t *testing.T) {
		stacks := dependsOnTestStacks(
			map[string]any{"depends_on": map[string]any{"1": map[string]any{"component": "vpc", "environment": "uw2"}}},
			map[string]any{"files": []any{"configs/app.yaml"}, "components": []any{map[string]any{"name": "chart", "kind": "helmfile"}}},
		)
		assert.Equal(t, []string{"depends-on:tenant1-uw2-dev-vpc"}, dependsOnLabels(t, templatesEnabled, stacks))
	})

	t.Run("settings.spacelift.depends_on is additive", func(t *testing.T) {
		stacks := dependsOnTestStacks(
			map[string]any{"spacelift": map[string]any{"workspace_enabled": true, "depends_on": []any{"tenant1-uw2-dev-vpc"}}},
			map[string]any{"components": []any{map[string]any{"name": "vpc"}}},
		)
		assert.Equal(
			t,
			[]string{"depends-on:tenant1-ue1-dev-vpc", "depends-on:tenant1-uw2-dev-vpc"},
			sortedCopy(dependsOnLabels(t, templatesEnabled, stacks)),
		)
	})

	t.Run("non-Terraform kinds are ignored", func(t *testing.T) {
		stacks := dependsOnTestStacks(nil, map[string]any{
			"components": []any{
				map[string]any{"name": "vpc"},
				map[string]any{"name": "chart", "kind": "helmfile"},
			},
		})
		assert.Equal(t, []string{"depends-on:tenant1-ue1-dev-vpc"}, dependsOnLabels(t, templatesEnabled, stacks))
	})

	t.Run("a missing required target is an error naming dependencies.components", func(t *testing.T) {
		stacks := dependsOnTestStacks(nil, map[string]any{
			"components": []any{map[string]any{"name": "vpc", "stack": "tenant1-ue2-dev"}},
		})
		_, err := TransformStackConfigToSpaceliftStacks(templatesEnabled, stacks, "stacks/%s.yaml", "{{.vars.tenant}}-{{.vars.environment}}-{{.vars.stage}}", "", false, nil)
		require.ErrorIs(t, err, errUtils.ErrInvalidDependenciesComponents)
		assert.Contains(t, err.Error(), "tenant1-ue2-dev")
	})

	t.Run("a missing target with required false is skipped", func(t *testing.T) {
		stacks := dependsOnTestStacks(nil, map[string]any{
			"components": []any{
				map[string]any{"name": "vpc", "stack": "tenant1-ue2-dev", "required": false},
				map[string]any{"name": "vpc"},
			},
		})
		assert.Equal(t, []string{"depends-on:tenant1-ue1-dev-vpc"}, dependsOnLabels(t, templatesEnabled, stacks))
	})

	// setVpc replaces the settings/metadata of the `vpc` target in the same stack as `app`.
	setVpc := func(stacks map[string]any, key string, value any) {
		vpc := stacks["orgs/tenant1/dev/ue1"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["vpc"].(map[string]any)
		vpc[key] = value
	}
	depOnVpc := map[string]any{"components": []any{map[string]any{"name": "vpc"}}}

	t.Run("a target with a stack_name override is labeled with its real Spacelift stack name", func(t *testing.T) {
		stacks := dependsOnTestStacks(nil, depOnVpc)
		setVpc(stacks, "settings", map[string]any{"spacelift": map[string]any{"workspace_enabled": true, "stack_name": "network-prod"}})
		assert.Equal(t, []string{"depends-on:network-prod"}, dependsOnLabels(t, templatesEnabled, stacks))
	})

	t.Run("a target with a stack_name_pattern override is labeled with its real Spacelift stack name", func(t *testing.T) {
		stacks := dependsOnTestStacks(nil, depOnVpc)
		setVpc(stacks, "settings", map[string]any{"spacelift": map[string]any{"workspace_enabled": true, "stack_name_pattern": "net-{environment}-{component}"}})
		assert.Equal(t, []string{"depends-on:net-ue1-vpc"}, dependsOnLabels(t, templatesEnabled, stacks))
	})

	t.Run("a required target without workspace_enabled is an error", func(t *testing.T) {
		stacks := dependsOnTestStacks(nil, depOnVpc)
		setVpc(stacks, "settings", map[string]any{"spacelift": map[string]any{"workspace_enabled": false}})
		_, err := TransformStackConfigToSpaceliftStacks(templatesEnabled, stacks, "stacks/%s.yaml", "{{.vars.tenant}}-{{.vars.environment}}-{{.vars.stage}}", "", false, nil)
		require.ErrorIs(t, err, errUtils.ErrInvalidDependenciesComponents)
	})

	t.Run("an optional target without workspace_enabled is skipped", func(t *testing.T) {
		stacks := dependsOnTestStacks(nil, map[string]any{"components": []any{map[string]any{"name": "vpc", "required": false}}})
		setVpc(stacks, "settings", map[string]any{})
		assert.Empty(t, dependsOnLabels(t, templatesEnabled, stacks))
	})

	t.Run("a required abstract target is an error", func(t *testing.T) {
		stacks := dependsOnTestStacks(nil, depOnVpc)
		setVpc(stacks, "metadata", map[string]any{"type": "abstract"})
		_, err := TransformStackConfigToSpaceliftStacks(templatesEnabled, stacks, "stacks/%s.yaml", "{{.vars.tenant}}-{{.vars.environment}}-{{.vars.stage}}", "", false, nil)
		require.ErrorIs(t, err, errUtils.ErrInvalidDependenciesComponents)
	})

	t.Run("a non-map dependencies section is an error", func(t *testing.T) {
		stacks := dependsOnTestStacks(nil, nil)
		stacks["orgs/tenant1/dev/ue1"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any)["dependencies"] = "vpc"
		_, err := TransformStackConfigToSpaceliftStacks(templatesEnabled, stacks, "stacks/%s.yaml", "{{.vars.tenant}}-{{.vars.environment}}-{{.vars.stage}}", "", false, nil)
		require.ErrorIs(t, err, errUtils.ErrInvalidDependenciesSection)
	})
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// TestCreateSpaceliftStacks_DependenciesComponentsFromManifests runs real manifests through the whole pipeline (imports,
// inheritance, deep merge, name template), proving `dependencies.components` survives the merge with its `stack`
// template unrendered and still resolves to the right Spacelift stacks.
func TestCreateSpaceliftStacks_DependenciesComponentsFromManifests(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"atmos.yaml": `
base_path: "."
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths: ["deploy/**/*"]
  excluded_paths: ["catalog/**/*"]
  name_template: "{{ .vars.tenant }}-{{ .vars.environment }}-{{ .vars.stage }}"
templates:
  settings:
    enabled: true
`,
		"components/terraform/vpc/main.tf": "",
		"components/terraform/app/main.tf": "",
		"stacks/catalog/app.yaml": `
components:
  terraform:
    app:
      settings:
        spacelift:
          workspace_enabled: true
      dependencies:
        components:
          - name: vpc
          - name: vpc
            stack: "{{ .vars.tenant }}-uw2-{{ .vars.stage }}"
`,
		"stacks/deploy/ue1.yaml": `
vars:
  tenant: tenant1
  environment: ue1
  stage: dev
import:
  - catalog/app
components:
  terraform:
    vpc:
      settings:
        spacelift:
          workspace_enabled: true
`,
		"stacks/deploy/uw2.yaml": `
vars:
  tenant: tenant1
  environment: uw2
  stage: dev
components:
  terraform:
    vpc:
      settings:
        spacelift:
          workspace_enabled: true
`,
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	t.Chdir(dir)

	spaceliftStacks, err := CreateSpaceliftStacks("", "", "", "", "", nil, false, true, true, "stacks/%s.yaml")
	require.NoError(t, err)

	app, ok := spaceliftStacks["tenant1-ue1-dev-app"].(map[string]any)
	require.True(t, ok, "expected tenant1-ue1-dev-app in %v", spaceliftStacks)

	var dependsOn []string
	for _, label := range app["labels"].([]string) {
		if strings.HasPrefix(label, "depends-on:") {
			dependsOn = append(dependsOn, label)
		}
	}
	assert.Equal(t, []string{"depends-on:tenant1-ue1-dev-vpc", "depends-on:tenant1-uw2-dev-vpc"}, dependsOn)
}
