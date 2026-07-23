package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func initTopLevelStackCompositionConfig(t *testing.T, fixture string) schema.AtmosConfiguration {
	t.Helper()
	t.Chdir("../../tests/fixtures/scenarios/" + fixture)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return atmosConfig
}

func TestTopLevelStackComposition(t *testing.T) {
	tests := []struct {
		name      string
		fixture   string
		stackName string
	}{
		{
			name:      "name template",
			fixture:   "top-level-stack-composition",
			stackName: "dev-shared",
		},
		{
			name:      "imperative name",
			fixture:   "top-level-stack-composition-explicit-name",
			stackName: "imperative-shared",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := initTopLevelStackCompositionConfig(t, tt.fixture)

			stacks, err := ExecuteDescribeStacks(
				&atmosConfig,
				"",
				nil,
				nil,
				nil,
				false,
				false,
				false,
				false,
				nil,
				nil,
			)
			require.NoError(t, err)
			require.Len(t, stacks, 1)

			stack, ok := stacks[tt.stackName].(map[string]any)
			require.True(t, ok, "both parents should compose one logical stack")
			components, ok := stack["components"].(map[string]any)
			require.True(t, ok)
			terraform, ok := components["terraform"].(map[string]any)
			require.True(t, ok)
			assert.Contains(t, terraform, "shared")
			assert.Contains(t, terraform, "network")
			assert.Contains(t, terraform, "platform")
			assert.Contains(t, terraform, "dns-primary")
			assert.Contains(t, terraform, "chatops")

			stacksMap, _, err := FindStacksMap(&atmosConfig, false)
			require.NoError(t, err)
			platformParent := stacksMap["parents/02-platform"].(map[string]any)
			platformComponents := platformParent["components"].(map[string]any)["terraform"].(map[string]any)
			assert.Equal(t, "primary.example.com", platformComponents["chatops"].(map[string]any)["vars"].(map[string]any)["hostname"])

			require.NoError(t, ValidateStacks(&atmosConfig))

			result, err := ExecuteDescribeComponentWithContext(DescribeComponentContextParams{
				AtmosConfig:      &atmosConfig,
				Component:        "shared",
				Stack:            tt.stackName,
				ProcessTemplates: false,
			})
			require.NoError(t, err)
			assert.Equal(t, "parents/01-network", result.StackFile)
			assert.Equal(t, "shared", result.ComponentSection["vars"].(map[string]any)["name"])

			inherited, err := ExecuteDescribeComponentWithContext(DescribeComponentContextParams{
				AtmosConfig:      &atmosConfig,
				Component:        "chatops",
				Stack:            tt.stackName,
				ProcessTemplates: false,
			})
			require.NoError(t, err)
			assert.Equal(t, "parents/02-platform", inherited.StackFile)
			assert.Equal(t, "chatops", inherited.ComponentSection["vars"].(map[string]any)["name"])
			assert.Equal(t, "primary.example.com", inherited.ComponentSection["vars"].(map[string]any)["hostname"])
		})
	}
}

func TestTopLevelStackCompositionRejectsConflictingDuplicates(t *testing.T) {
	atmosConfig := initTopLevelStackCompositionConfig(t, "top-level-stack-composition-conflict")

	err := ValidateStacks(&atmosConfig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "component 'conflicting'")
	assert.Contains(t, err.Error(), "more than one top-level stack manifest file")
	assert.Contains(t, err.Error(), "parents/01-network")
	assert.Contains(t, err.Error(), "parents/02-platform")
	_, err = ExecuteDescribeComponentWithContext(DescribeComponentContextParams{
		AtmosConfig:      &atmosConfig,
		Component:        "conflicting",
		Stack:            "dev-conflict",
		ProcessTemplates: false,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Found duplicate config")
}

func TestTopLevelStackCompositionPreservesParentScopes(t *testing.T) {
	atmosConfig := initTopLevelStackCompositionConfig(t, "top-level-stack-composition-parent-scope")

	dns, err := ExecuteDescribeComponentWithContext(DescribeComponentContextParams{
		AtmosConfig:      &atmosConfig,
		Component:        "dns-primary",
		Stack:            "dev-scoped",
		ProcessTemplates: false,
	})
	require.NoError(t, err)
	assert.Equal(t, "parents/01-network", dns.StackFile)
	assert.Equal(t, "network", dns.ComponentSection["vars"].(map[string]any)["owner"])

	chatops, err := ExecuteDescribeComponentWithContext(DescribeComponentContextParams{
		AtmosConfig:      &atmosConfig,
		Component:        "chatops",
		Stack:            "dev-scoped",
		ProcessTemplates: false,
	})
	require.NoError(t, err)
	assert.Equal(t, "parents/02-platform", chatops.StackFile)
	assert.Equal(t, "platform", chatops.ComponentSection["vars"].(map[string]any)["owner"])
	assert.Equal(t, "primary.example.com", chatops.ComponentSection["vars"].(map[string]any)["hostname"])
}

func TestTopLevelStackCompositionRejectsCrossLogicalStackInheritance(t *testing.T) {
	atmosConfig := initTopLevelStackCompositionConfig(t, "top-level-stack-composition-isolated-inheritance")

	_, _, err := FindStacksMap(&atmosConfig, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "component 'chatops'")
	assert.Contains(t, err.Error(), "inherits from 'dns-primary'")
}

func TestLogicalStackIdentityDefersMissingComponentContext(t *testing.T) {
	tests := []struct {
		name         string
		stacksConfig schema.Stacks
	}{
		{
			name:         "name template",
			stacksConfig: schema.Stacks{NameTemplate: "{{ .vars.stage }}"},
		},
		{
			name:         "name pattern",
			stacksConfig: schema.Stacks{NamePattern: "{stage}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{Stacks: tt.stacksConfig}
			identity, err := logicalStackIdentity(atmosConfig, "parents/component-scoped", map[string]any{})
			require.NoError(t, err)
			assert.Equal(t, "parents/component-scoped", identity)
		})
	}
}
