package describe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeStacks(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacks, err := ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, false)
	assert.Nil(t, err)

	dependentsYaml, err := yaml.Marshal(stacks)
	assert.Nil(t, err)
	t.Log(string(dependentsYaml))
}

func TestDescribeStacksWithFilter1(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"

	stacks, err := ExecuteDescribeStacks(cliConfig, stack, nil, nil, nil, false)
	assert.Nil(t, err)

	dependentsYaml, err := yaml.Marshal(stacks)
	assert.Nil(t, err)
	t.Log(string(dependentsYaml))
}

func TestDescribeStacksWithFilter2(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"
	components := []string{"infra/vpc"}

	stacks, err := ExecuteDescribeStacks(cliConfig, stack, components, nil, nil, false)
	assert.Nil(t, err)

	dependentsYaml, err := yaml.Marshal(stacks)
	assert.Nil(t, err)
	t.Log(string(dependentsYaml))
}

func TestDescribeStacksWithFilter3(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"
	sections := []string{"vars"}

	stacks, err := ExecuteDescribeStacks(cliConfig, stack, nil, nil, sections, false)
	assert.Nil(t, err)

	dependentsYaml, err := yaml.Marshal(stacks)
	assert.Nil(t, err)
	t.Log(string(dependentsYaml))
}

func TestDescribeStacksWithFilter4(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	componentTypes := []string{"terraform"}
	sections := []string{"none"}

	stacks, err := ExecuteDescribeStacks(cliConfig, "", nil, componentTypes, sections, false)
	assert.Nil(t, err)

	dependentsYaml, err := yaml.Marshal(stacks)
	assert.Nil(t, err)
	t.Log(string(dependentsYaml))
}

func TestDescribeStacksWithFilter5(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	componentTypes := []string{"terraform"}
	components := []string{"top-level-component1"}
	sections := []string{"vars"}

	stacks, err := ExecuteDescribeStacks(cliConfig, "", components, componentTypes, sections, false)
	assert.Nil(t, err)
	assert.Equal(t, 8, len(stacks))

	tenant1Ue2DevStack := stacks["tenant1-ue2-dev"].(map[string]any)
	tenant1Ue2DevStackComponents := tenant1Ue2DevStack["components"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraform := tenant1Ue2DevStackComponents["terraform"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraformComponent := tenant1Ue2DevStackComponentsTerraform["top-level-component1"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraformComponentVars := tenant1Ue2DevStackComponentsTerraformComponent["vars"].(map[any]any)
	tenant1Ue2DevStackComponentsTerraformComponentVarsTenant := tenant1Ue2DevStackComponentsTerraformComponentVars["tenant"].(string)
	tenant1Ue2DevStackComponentsTerraformComponentVarsStage := tenant1Ue2DevStackComponentsTerraformComponentVars["stage"].(string)
	tenant1Ue2DevStackComponentsTerraformComponentVarsEnvironment := tenant1Ue2DevStackComponentsTerraformComponentVars["environment"].(string)
	assert.Equal(t, "tenant1", tenant1Ue2DevStackComponentsTerraformComponentVarsTenant)
	assert.Equal(t, "ue2", tenant1Ue2DevStackComponentsTerraformComponentVarsEnvironment)
	assert.Equal(t, "dev", tenant1Ue2DevStackComponentsTerraformComponentVarsStage)

	stacksYaml, err := yaml.Marshal(stacks)
	assert.Nil(t, err)
	t.Log(string(stacksYaml))
}

func TestDescribeStacksWithFilter6(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"
	componentTypes := []string{"terraform"}
	components := []string{"top-level-component1"}
	sections := []string{"workspace"}

	stacks, err := ExecuteDescribeStacks(cliConfig, "tenant1-ue2-dev", components, componentTypes, sections, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(stacks))

	tenant1Ue2DevStack := stacks[stack].(map[string]any)
	tenant1Ue2DevStackComponents := tenant1Ue2DevStack["components"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraform := tenant1Ue2DevStackComponents["terraform"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraformComponent := tenant1Ue2DevStackComponentsTerraform["top-level-component1"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraformWorkspace := tenant1Ue2DevStackComponentsTerraformComponent["workspace"].(string)
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevStackComponentsTerraformWorkspace)

	stacksYaml, err := yaml.Marshal(stacks)
	assert.Nil(t, err)
	t.Log(string(stacksYaml))
}

func TestDescribeStacksWithFilter7(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"
	componentTypes := []string{"terraform"}
	components := []string{"test/test-component-override-3"}
	sections := []string{"workspace"}

	stacks, err := ExecuteDescribeStacks(cliConfig, stack, components, componentTypes, sections, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(stacks))

	tenant1Ue2DevStack := stacks[stack].(map[string]any)
	tenant1Ue2DevStackComponents := tenant1Ue2DevStack["components"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraform := tenant1Ue2DevStackComponents["terraform"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraformComponent := tenant1Ue2DevStackComponentsTerraform["test/test-component-override-3"].(map[string]any)
	tenant1Ue2DevStackComponentsTerraformWorkspace := tenant1Ue2DevStackComponentsTerraformComponent["workspace"].(string)
	assert.Equal(t, "test-component-override-3-workspace", tenant1Ue2DevStackComponentsTerraformWorkspace)

	stacksYaml, err := yaml.Marshal(stacks)
	assert.Nil(t, err)
	t.Log(string(stacksYaml))
}
