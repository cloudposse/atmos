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

	// includeEmptyStacks=true should return at least as many stacks as false.
	// If the fixture has import-only or component-less stacks, the count will be strictly greater.
	// If all stacks have components, the counts are equal — both behaviors are correct.
	assert.GreaterOrEqual(t, len(stacksWithEmpty), initialStackCount, "Should include at least as many stacks when empty stacks are included")
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

	// includeEmptyStacks=true should return at least as many stacks as false.
	assert.GreaterOrEqual(t, len(stacksWithEmpty), initialCount, "Should have at least as many stacks when including empty ones")

	// Verify we have at least some non-empty stacks in the output.
	var nonEmptyStacks []string
	for stackName, stackContent := range stacksWithEmpty {
		if stack, ok := stackContent.(map[string]any); ok {
			if components, hasComponents := stack["components"].(map[string]any); hasComponents {
				for _, compType := range components {
					if compMap, ok := compType.(map[string]any); ok && len(compMap) > 0 {
						nonEmptyStacks = append(nonEmptyStacks, stackName)
						break
					}
				}
			}
		}
	}
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

// TestDescribeStacksComponentDescription verifies that the description field on a component
// is included in the describe stacks output.
func TestDescribeStacksComponentDescription(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"
	components := []string{"top-level-component1"}
	componentTypes := []string{"terraform"}

	stacks, err := ExecuteDescribeStacks(atmosConfig, stack, components, componentTypes, nil, false, false)
	assert.Nil(t, err)
	assert.NotNil(t, stacks)

	stackMap, ok := stacks[stack].(map[string]any)
	assert.True(t, ok, "stack entry should be a map")

	componentsMap, ok := stackMap["components"].(map[string]any)
	assert.True(t, ok, "stack should have components section")

	terraformMap, ok := componentsMap["terraform"].(map[string]any)
	assert.True(t, ok, "components should have terraform section")

	component, ok := terraformMap["top-level-component1"].(map[string]any)
	assert.True(t, ok, "terraform should have top-level-component1")

	description, ok := component["description"].(string)
	assert.True(t, ok, "component should have description field")
	assert.Equal(t, "Top-level component for testing Atmos stack configuration.", description)
}

// TestDescribeStacksStackDescription verifies that the description field at the stack level
// is included in the describe stacks output.
func TestDescribeStacksStackDescription(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"

	stacks, err := ExecuteDescribeStacks(atmosConfig, stack, nil, nil, nil, false, false)
	assert.Nil(t, err)
	assert.NotNil(t, stacks)

	stackMap, ok := stacks[stack].(map[string]any)
	assert.True(t, ok, "stack entry should be a map")

	description, ok := stackMap["description"].(string)
	assert.True(t, ok, "stack should have description field")
	assert.Equal(t, "Tenant1 US East 2 development stack.", description)
}

// TestDescribeStacksDescriptionSectionFilter verifies that filtering by the description section
// includes the description field and excludes others.
func TestDescribeStacksDescriptionSectionFilter(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"
	components := []string{"top-level-component1"}
	componentTypes := []string{"terraform"}
	sections := []string{"description"}

	stacks, err := ExecuteDescribeStacks(atmosConfig, stack, components, componentTypes, sections, false, false)
	assert.Nil(t, err)
	assert.NotNil(t, stacks)

	stackMap, ok := stacks[stack].(map[string]any)
	assert.True(t, ok, "stack entry should be a map")

	// Stack-level description should be present.
	description, ok := stackMap["description"].(string)
	assert.True(t, ok, "stack should have description field when filtering by description section")
	assert.Equal(t, "Tenant1 US East 2 development stack.", description)

	componentsMap, ok := stackMap["components"].(map[string]any)
	assert.True(t, ok, "stack should have components section")

	terraformMap, ok := componentsMap["terraform"].(map[string]any)
	assert.True(t, ok, "components should have terraform section")

	component, ok := terraformMap["top-level-component1"].(map[string]any)
	assert.True(t, ok, "terraform should have top-level-component1")

	// Component description should be present.
	compDescription, ok := component["description"].(string)
	assert.True(t, ok, "component should have description field when filtering by description section")
	assert.Equal(t, "Top-level component for testing Atmos stack configuration.", compDescription)

	// No other sections like vars should be present.
	_, hasVars := component["vars"]
	assert.False(t, hasVars, "vars should not be present when filtering by description section only")
}

// TestDescribeStacksNoDescriptionField verifies that components without a description field
// don't have the description field in the output when it's empty and includeEmpty is false.
func TestDescribeStacksNoDescriptionField(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	includeEmpty := false
	atmosConfig.Describe.Settings.IncludeEmpty = &includeEmpty

	stack := "tenant1-ue2-dev"
	components := []string{"test/test-component"}
	componentTypes := []string{"terraform"}

	stacks, err := ExecuteDescribeStacks(atmosConfig, stack, components, componentTypes, nil, false, false)
	assert.Nil(t, err)
	assert.NotNil(t, stacks)

	stackMap, ok := stacks[stack].(map[string]any)
	assert.True(t, ok, "stack entry should be a map")

	componentsMap, ok := stackMap["components"].(map[string]any)
	assert.True(t, ok)

	terraformMap, ok := componentsMap["terraform"].(map[string]any)
	assert.True(t, ok)

	component, ok := terraformMap["test/test-component"].(map[string]any)
	assert.True(t, ok, "terraform should have test/test-component")

	// test/test-component has no description field - it should not appear as a non-empty string.
	desc := component["description"]
	if descStr, ok := desc.(string); ok {
		assert.Empty(t, descStr, "description should be empty if not set in YAML")
	}
}
