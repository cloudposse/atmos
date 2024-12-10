package list

import (
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	testStack = "tenant1-ue2-dev"
)

func TestListComponents(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(cliConfig, "", nil, nil,
		nil, false, false, false)
	assert.Nil(t, err)

	listConfig := schema.ListConfig{
		Columns: []schema.ListColumnConfig{
			{Name: "Component", Value: "{{ .atmos_component }}"},
			{Name: "Stack", Value: "{{ .atmos_stack }}"},
			{Name: "Folder", Value: "{{ .vars.tenant }}"},
		},
	}
	output, err := FilterAndListComponents("", stacksMap, listConfig)
	assert.Nil(t, err)
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.Nil(t, err)
	// Add assertions to validate the output structure
	assert.NotNil(t, dependentsYaml)
	assert.Greater(t, len(dependentsYaml), 0)
	t.Log(dependentsYaml)
}

func TestListComponentsWithStack(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(cliConfig, testStack, nil, nil,
		nil, false, false, false)
	assert.Nil(t, err)

	output, err := FilterAndListStacks(stacksMap, testStack)
	assert.Nil(t, err)
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.Nil(t, err)
	assert.NotNil(t, dependentsYaml)
	assert.Greater(t, len(dependentsYaml), 0)
	assert.Contains(t, dependentsYaml, testStack)
	t.Log(dependentsYaml)
}
