package describe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/testhelper"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestDescribeStacks(t *testing.T) {
	testhelper.Run(t, func(t *testing.T) {
		configAndStacksInfo := schema.ConfigAndStacksInfo{}

		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		assert.Nil(t, err)

		stacks, err := ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false)
		assert.Nil(t, err)
		assert.NotNil(t, stacks)

		// Output will be automatically buffered and shown on failure
		stacksYaml, err := u.ConvertToYAML(stacks)
		assert.Nil(t, err)
		t.Log(stacksYaml)
	})
}

func TestDescribeStacksWithFilter1(t *testing.T) {
	testhelper.Run(t, func(t *testing.T) {
		configAndStacksInfo := schema.ConfigAndStacksInfo{}

		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		assert.Nil(t, err)

		stack := "tenant1-ue2-dev"

		stacks, err := ExecuteDescribeStacks(atmosConfig, stack, nil, nil, nil, false, false)
		assert.Nil(t, err)
		assert.NotNil(t, stacks)

		stacksYaml, err := u.ConvertToYAML(stacks)
		assert.Nil(t, err)
		t.Log(stacksYaml)
	})
}

func TestDescribeStacksWithFilter2(t *testing.T) {
	testhelper.Run(t, func(t *testing.T) {
		configAndStacksInfo := schema.ConfigAndStacksInfo{}

		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		assert.Nil(t, err)

		stack := "tenant1-ue2-dev"
		components := []string{"infra/vpc"}

		stacks, err := ExecuteDescribeStacks(atmosConfig, stack, components, nil, nil, false, false)
		assert.Nil(t, err)
		assert.NotNil(t, stacks)

		stacksYaml, err := u.ConvertToYAML(stacks)
		assert.Nil(t, err)
		t.Log(stacksYaml)
	})
}

func TestDescribeStacksWithFilter3(t *testing.T) {
	testhelper.Run(t, func(t *testing.T) {
		configAndStacksInfo := schema.ConfigAndStacksInfo{}

		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		assert.Nil(t, err)

		stack := "tenant1-ue2-dev"
		sections := []string{"vars"}

		stacks, err := ExecuteDescribeStacks(atmosConfig, stack, nil, nil, sections, false, false)
		assert.Nil(t, err)
		assert.NotNil(t, stacks)

		stacksYaml, err := u.ConvertToYAML(stacks)
		assert.Nil(t, err)
		t.Log(stacksYaml)
	})
}

func TestDescribeStacksWithFilter4(t *testing.T) {
	testhelper.Run(t, func(t *testing.T) {
		configAndStacksInfo := schema.ConfigAndStacksInfo{}

		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		assert.Nil(t, err)

		componentTypes := []string{"terraform"}
		sections := []string{"none"}

		stacks, err := ExecuteDescribeStacks(atmosConfig, "", nil, componentTypes, sections, false, false)
		assert.Nil(t, err)
		assert.NotNil(t, stacks)

		stacksYaml, err := u.ConvertToYAML(stacks)
		assert.Nil(t, err)
		t.Log(stacksYaml)
	})
}

func TestDescribeStacksWithFilter5(t *testing.T) {
	testhelper.Run(t, func(t *testing.T) {
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
		t.Log(stacksYaml)
	})
}

func TestDescribeStacksWithFilter6(t *testing.T) {
	testhelper.Run(t, func(t *testing.T) {
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
		t.Log(stacksYaml)
	})
}

func TestDescribeStacksWithFilter7(t *testing.T) {
	testhelper.Run(t, func(t *testing.T) {
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
		t.Log(stacksYaml)
	})
}

func TestDescribeStacksWithEmptyStacks(t *testing.T) {
	testhelper.Run(t, func(t *testing.T) {
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
	})
}

func TestDescribeStacksWithVariousEmptyStacks(t *testing.T) {
	testhelper.Run(t, func(t *testing.T) {
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
	})
}
