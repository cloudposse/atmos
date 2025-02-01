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

	output, err := FilterAndListStacks(stacksMap, "", "", "", "")
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

	output, err := FilterAndListStacks(stacksMap, component, "", "", "")
	assert.Nil(t, err)
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.Nil(t, err)

	// Verify the output structure
	assert.NotEmpty(t, dependentsYaml)
	// Verify that only stacks with the specified component are included
	assert.Contains(t, dependentsYaml, testComponent)
}

func TestListStacksWithJQ(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil,
		nil, false, false, false)
	assert.Nil(t, err)

	// Test case 1: List only stack names using 'keys' query
	output, err := FilterAndListStacks(stacksMap, "", "", "keys", "")
	assert.Nil(t, err)
	var stackNames []string
	err = json.Unmarshal([]byte(output), &stackNames)
	assert.Nil(t, err)
	assert.NotEmpty(t, stackNames)

	// Test case 2: Full JSON structure using '.' query
	output, err = FilterAndListStacks(stacksMap, "", "", ".", "")
	assert.Nil(t, err)
	var fullStructure map[string]interface{}
	err = json.Unmarshal([]byte(output), &fullStructure)
	assert.Nil(t, err)
	assert.Equal(t, stacksMap, fullStructure)

	// Test case 3: Custom mapping query
	jqQuery := `to_entries | map({stack: .key})`
	output, err = FilterAndListStacks(stacksMap, "", "", jqQuery, "")
	assert.Nil(t, err)
	var customMapping []map[string]string
	err = json.Unmarshal([]byte(output), &customMapping)
	assert.Nil(t, err)
	assert.NotEmpty(t, customMapping)
	assert.Contains(t, customMapping[0], "stack")
}

func TestListStacksWithGoTemplate(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil,
		nil, false, false, false)
	assert.Nil(t, err)

	// Test case 1: Simple range template
	template := `{{range $stack, $_ := .}}{{$stack}}{{"\n"}}{{end}}`
	output, err := FilterAndListStacks(stacksMap, "", "", "", template)
	assert.Nil(t, err)
	assert.NotEmpty(t, output)

	// Test case 2: Table format with autocolor
	template = `{{range $stack, $data := .}}{{tablerow (autocolor $stack) ($data.components.terraform | keys | join ", ")}}{{end}}`
	output, err = FilterAndListStacks(stacksMap, "", "", "", template)
	assert.Nil(t, err)
	assert.NotEmpty(t, output)
}

func TestListStacksWithComponentAndTemplate(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)
	component := testComponent

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, component, nil, nil,
		nil, false, false, false)
	assert.Nil(t, err)

	// Test filtering by component with JQ template
	jqQuery := `to_entries | map({stack: .key, component: .value.components.terraform | keys})`
	output, err := FilterAndListStacks(stacksMap, component, "", jqQuery, "")
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

	// Test filtering by component with Go template
	template := `{{range $stack, $data := .}}{{tablerow $stack ($data.components.terraform | keys | join ", ")}}{{end}}`
	output, err = FilterAndListStacks(stacksMap, component, "", "", template)
	assert.Nil(t, err)
	assert.NotEmpty(t, output)
	assert.Contains(t, output, testComponent)
}
