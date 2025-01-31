package list

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	testComponent = "infra/vpc"
)

func TestListStacks(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil,
		nil, false, false, false)
	assert.Nil(t, err)

	output, err := FilterAndListStacks(stacksMap, "", "")
	assert.Nil(t, err)
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.NotEmpty(t, dependentsYaml)
}

func TestListStacksWithComponent(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)
	component := testComponent

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, component, nil, nil,
		nil, false, false, false)
	assert.Nil(t, err)

	output, err := FilterAndListStacks(stacksMap, component, "")
	assert.Nil(t, err)
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.Nil(t, err)

	// Verify the output structure
	assert.NotEmpty(t, dependentsYaml)
	// Verify that only stacks with the specified component are included
	assert.Contains(t, dependentsYaml, testComponent)
}

func TestListStacksWithTemplate(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil,
		nil, false, false, false)
	assert.Nil(t, err)

	// Test case 1: List only stack names using 'keys' template
	output, err := FilterAndListStacks(stacksMap, "", "keys")
	assert.Nil(t, err)
	var stackNames []string
	err = json.Unmarshal([]byte(output), &stackNames)
	assert.Nil(t, err)
	assert.NotEmpty(t, stackNames)

	// Test case 2: Full JSON structure using '.' template
	output, err = FilterAndListStacks(stacksMap, "", ".")
	assert.Nil(t, err)
	var fullStructure map[string]interface{}
	err = json.Unmarshal([]byte(output), &fullStructure)
	assert.Nil(t, err)
	assert.Equal(t, stacksMap, fullStructure)

	// Test case 3: Custom mapping template
	template := `to_entries | map({stack: .key})`
	output, err = FilterAndListStacks(stacksMap, "", template)
	assert.Nil(t, err)
	var customMapping []map[string]string
	err = json.Unmarshal([]byte(output), &customMapping)
	assert.Nil(t, err)
	assert.NotEmpty(t, customMapping)
	assert.Contains(t, customMapping[0], "stack")
}

func TestListStacksWithComponentAndTemplate(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)
	component := testComponent

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, component, nil, nil,
		nil, false, false, false)
	assert.Nil(t, err)

	// Test filtering by component with template
	template := `to_entries | map({stack: .key, component: .value.components.terraform | keys})`
	output, err := FilterAndListStacks(stacksMap, component, template)
	assert.Nil(t, err)

	var result []map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	assert.Nil(t, err)
	assert.NotEmpty(t, result)

	// Verify that component exists in the output
	for _, item := range result {
		components, ok := item["component"].([]interface{})
		assert.True(t, ok)
		assert.Contains(t, components, testComponent)
	}
}
