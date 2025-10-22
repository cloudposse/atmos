package describe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestDescribeStacks(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacks, err := ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false)
	assert.Nil(t, err)

	stacksYaml, err := u.ConvertToYAML(stacks)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if stacksYaml != "" {
				t.Logf("Stacks:\n%s", stacksYaml)
			} else {
				t.Logf("Stacks (raw): %+v", stacks)
			}
		}
	})
}

func TestDescribeStacksWithFilter1(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"

	stacks, err := ExecuteDescribeStacks(atmosConfig, stack, nil, nil, nil, false, false)
	assert.Nil(t, err)

	stacksYaml, err := u.ConvertToYAML(stacks)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if stacksYaml != "" {
				t.Logf("Stacks:\n%s", stacksYaml)
			} else {
				t.Logf("Stacks (raw): %+v", stacks)
			}
		}
	})
}

func TestDescribeStacksWithFilter2(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"
	components := []string{"infra/vpc"}

	stacks, err := ExecuteDescribeStacks(atmosConfig, stack, components, nil, nil, false, false)
	assert.Nil(t, err)

	stacksYaml, err := u.ConvertToYAML(stacks)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if stacksYaml != "" {
				t.Logf("Stacks:\n%s", stacksYaml)
			} else {
				t.Logf("Stacks (raw): %+v", stacks)
			}
		}
	})
}

func TestDescribeStacksWithFilter3(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"
	sections := []string{"vars"}

	stacks, err := ExecuteDescribeStacks(atmosConfig, stack, nil, nil, sections, false, false)
	assert.Nil(t, err)

	stacksYaml, err := u.ConvertToYAML(stacks)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if stacksYaml != "" {
				t.Logf("Stacks:\n%s", stacksYaml)
			} else {
				t.Logf("Stacks (raw): %+v", stacks)
			}
		}
	})
}

func TestDescribeStacksWithFilter4(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	componentTypes := []string{"terraform"}
	sections := []string{"none"}

	stacks, err := ExecuteDescribeStacks(atmosConfig, "", nil, componentTypes, sections, false, false)
	assert.Nil(t, err)

	stacksYaml, err := u.ConvertToYAML(stacks)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if stacksYaml != "" {
				t.Logf("Stacks:\n%s", stacksYaml)
			} else {
				t.Logf("Stacks (raw): %+v", stacks)
			}
		}
	})
}

func TestDescribeStacksWithFilter5(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	componentTypes := []string{"terraform"}
	components := []string{"top-level-component1"}
	sections := []string{"vars"}

	stacks, err := ExecuteDescribeStacks(atmosConfig, "", components, componentTypes, sections, false, false)
	assert.Nil(t, err)
	assert.Equal(t, 8, len(stacks))

	tenant1Ue2DevStack := stacks["tenant1-ue2-dev"].(map[string]any)
	tenant1Ue2DevStackComponents := tenant1Ue2DevStack["components"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraform := tenant1Ue2DevStackComponents["terraform"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraformComponent := tenant1Ue2DevStackComponentsTerraform["top-level-component1"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraformComponentVars := tenant1Ue2DevStackComponentsTerraformComponent["vars"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraformComponentVarsTenant := tenant1Ue2DevStackComponentsTerraformComponentVars["tenant"].(string)
	tenant1Ue2DevStackComponentsTerraformComponentVarsStage := tenant1Ue2DevStackComponentsTerraformComponentVars["stage"].(string)
	tenant1Ue2DevStackComponentsTerraformComponentVarsEnvironment := tenant1Ue2DevStackComponentsTerraformComponentVars["environment"].(string)
	assert.Equal(t, "tenant1", tenant1Ue2DevStackComponentsTerraformComponentVarsTenant)
	assert.Equal(t, "ue2", tenant1Ue2DevStackComponentsTerraformComponentVarsEnvironment)
	assert.Equal(t, "dev", tenant1Ue2DevStackComponentsTerraformComponentVarsStage)

	stacksYaml, err := u.ConvertToYAML(stacks)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if stacksYaml != "" {
				t.Logf("Stacks:\n%s", stacksYaml)
			} else {
				t.Logf("Stacks (raw): %+v", stacks)
			}
		}
	})
}

func TestDescribeStacksWithFilter6(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"
	componentTypes := []string{"terraform"}
	components := []string{"top-level-component1"}
	sections := []string{"workspace"}

	stacks, err := ExecuteDescribeStacks(atmosConfig, "tenant1-ue2-dev", components, componentTypes, sections, false, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(stacks))

	tenant1Ue2DevStack := stacks[stack].(map[string]any)
	tenant1Ue2DevStackComponents := tenant1Ue2DevStack["components"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraform := tenant1Ue2DevStackComponents["terraform"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraformComponent := tenant1Ue2DevStackComponentsTerraform["top-level-component1"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraformWorkspace := tenant1Ue2DevStackComponentsTerraformComponent["workspace"].(string)
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevStackComponentsTerraformWorkspace)

	stacksYaml, err := u.ConvertToYAML(stacks)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if stacksYaml != "" {
				t.Logf("Stacks:\n%s", stacksYaml)
			} else {
				t.Logf("Stacks (raw): %+v", stacks)
			}
		}
	})
}

func TestDescribeStacksWithFilter7(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"
	componentTypes := []string{"terraform"}
	components := []string{"test/test-component-override-3"}
	sections := []string{"workspace"}

	stacks, err := ExecuteDescribeStacks(atmosConfig, stack, components, componentTypes, sections, false, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(stacks))

	tenant1Ue2DevStack := stacks[stack].(map[string]any)
	tenant1Ue2DevStackComponents := tenant1Ue2DevStack["components"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraform := tenant1Ue2DevStackComponents["terraform"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraformComponent := tenant1Ue2DevStackComponentsTerraform["test/test-component-override-3"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraformWorkspace := tenant1Ue2DevStackComponentsTerraformComponent["workspace"].(string)
	assert.Equal(t, "test-component-override-3-workspace", tenant1Ue2DevStackComponentsTerraformWorkspace)

	stacksYaml, err := u.ConvertToYAML(stacks)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if stacksYaml != "" {
				t.Logf("Stacks:\n%s", stacksYaml)
			} else {
				t.Logf("Stacks (raw): %+v", stacks)
			}
		}
	})
}

func TestDescribeStacksWithFilter8(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	componentTypes := []string{"helmfile"}
	sections := []string{"none"}

	stacks, err := ExecuteDescribeStacks(atmosConfig, "", nil, componentTypes, sections, false, false)
	assert.Nil(t, err)

	stacksYaml, err := u.ConvertToYAML(stacks)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if stacksYaml != "" {
				t.Logf("Stacks:\n%s", stacksYaml)
			} else {
				t.Logf("Stacks (raw): %+v", stacks)
			}
		}
	})
}

func TestDescribeStacksWithEmptyStacks(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacks, err := ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false)
	assert.Nil(t, err)

	initialStackCount := len(stacks)

	stacksWithEmpty, err := ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true)
	assert.Nil(t, err)

	assert.Greater(t, len(stacksWithEmpty), initialStackCount, "Should include more stacks when empty stacks are included")

	foundEmptyStack := false
	for _, stackContent := range stacksWithEmpty {
		if components, ok := stackContent.(map[string]any)["components"].(map[string]any); ok {
			if len(components) == 0 {
				foundEmptyStack = true
				break
			}
			if len(components) == 1 {
				if terraformComps, hasTerraform := components["terraform"].(map[string]any); hasTerraform {
					if len(terraformComps) == 0 {
						foundEmptyStack = true
						break
					}
				}
			}
		}
	}
	assert.True(t, foundEmptyStack, "Should find at least one empty stack")
}

func TestDescribeStacksWithVariousEmptyStacks(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksWithoutEmpty, err := ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false)
	assert.Nil(t, err)
	initialCount := len(stacksWithoutEmpty)

	stacksWithEmpty, err := ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true)
	assert.Nil(t, err)

	assert.Greater(t, len(stacksWithEmpty), initialCount, "Should have more stacks when including empty ones")

	var (
		emptyStacks    []string
		nonEmptyStacks []string
	)

	for stackName, stackContent := range stacksWithEmpty {
		if stack, ok := stackContent.(map[string]any); ok {
			if components, hasComponents := stack["components"].(map[string]any); hasComponents {
				// Check for completely empty components
				if len(components) == 0 {
					emptyStacks = append(emptyStacks, stackName)
					continue
				}

				// Check if only terraform exists and is empty
				if len(components) == 1 {
					if terraformComps, hasTerraform := components["terraform"].(map[string]any); hasTerraform {
						if len(terraformComps) == 0 {
							emptyStacks = append(emptyStacks, stackName)
							continue
						}
					}
				}

				// If we have any components at all, consider it non-empty
				for _, compType := range components {
					if compMap, ok := compType.(map[string]any); ok && len(compMap) > 0 {
						nonEmptyStacks = append(nonEmptyStacks, stackName)
						break
					}
				}
			}
		}
	}

	// Verify we found both types of stacks
	assert.NotEmpty(t, emptyStacks, "Should find at least one empty stack")
	assert.NotEmpty(t, nonEmptyStacks, "Should find at least one non-empty stack")
}

// Helper function to count sections across all stacks.
func countSections(stacks map[string]any) int {
	totalSections := 0
	for _, stackContent := range stacks {
		stack, ok := stackContent.(map[string]any)
		if !ok {
			continue
		}

		components, ok := stack["components"].(map[string]any)
		if !ok {
			continue
		}

		terraform, ok := components["terraform"].(map[string]any)
		if !ok {
			continue
		}

		for _, component := range terraform {
			compMap, ok := component.(map[string]any)
			if !ok {
				continue
			}
			totalSections += len(compMap)
		}
	}
	return totalSections
}

// Helper function to count sections for a specific component in a stack.
func countComponentSections(stackData map[string]any, stack, component string) int {
	stackMap, ok := stackData[stack].(map[string]any)
	if !ok {
		return 0
	}

	components, ok := stackMap["components"].(map[string]any)
	if !ok {
		return 0
	}

	if terraform, ok := components["terraform"].(map[string]any); ok {
		if comp, ok := terraform[component].(map[string]any); ok {
			return len(comp)
		}
	}

	if helmfile, ok := components["helmfile"].(map[string]any); ok {
		if comp, ok := helmfile[component].(map[string]any); ok {
			return len(comp)
		}
	}

	return 0
}

// Helper function to setup test configuration.
func setupEmptySectionFilteringTest(includeEmpty bool) (schema.AtmosConfiguration, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return atmosConfig, err
	}

	atmosConfig.Describe.Settings.IncludeEmpty = &includeEmpty
	return atmosConfig, nil
}

// Test filtering empty sections at the stack level.
func TestDescribeStacksWithEmptySectionFilteringAllStacks(t *testing.T) {
	// Setup with includeEmpty = false
	atmosConfigFiltered, err := setupEmptySectionFilteringTest(false)
	assert.Nil(t, err)

	stacksWithFilteredSections, err := ExecuteDescribeStacks(atmosConfigFiltered, "", nil, nil, nil, false, false)
	assert.Nil(t, err)

	atmosConfigAll, err := setupEmptySectionFilteringTest(true)
	assert.Nil(t, err)

	stacksWithAllSections, err := ExecuteDescribeStacks(atmosConfigAll, "", nil, nil, nil, false, false)
	assert.Nil(t, err)

	filteredSectionCount := countSections(stacksWithFilteredSections)
	allSectionCount := countSections(stacksWithAllSections)

	assert.LessOrEqual(t, filteredSectionCount, allSectionCount,
		"When includeEmpty is false, there should be fewer or equal number of sections")
}

// Test filtering empty sections at the component level.
func TestDescribeStacksWithEmptySectionFilteringComponent(t *testing.T) {
	stack := "tenant1-ue2-dev"
	component := "infra/vpc"

	atmosConfigFiltered, err := setupEmptySectionFilteringTest(false)
	assert.Nil(t, err)

	filteredStack, err := ExecuteDescribeStacks(atmosConfigFiltered, stack, []string{component}, nil, nil, false, false)
	assert.Nil(t, err)

	atmosConfigAll, err := setupEmptySectionFilteringTest(true)
	assert.Nil(t, err)

	completeStack, err := ExecuteDescribeStacks(atmosConfigAll, stack, []string{component}, nil, nil, false, false)
	assert.Nil(t, err)

	filteredCompSections := countComponentSections(filteredStack, stack, component)
	allCompSections := countComponentSections(completeStack, stack, component)

	assert.LessOrEqual(t, filteredCompSections, allCompSections,
		"When includeEmpty is false, the component should have fewer or equal sections")
}

// Test filtering empty sections at the stack and component levels.
func TestDescribeStacksWithEmptySectionFiltering(t *testing.T) {
	TestDescribeStacksWithEmptySectionFilteringAllStacks(t)
	TestDescribeStacksWithEmptySectionFilteringComponent(t)
}
