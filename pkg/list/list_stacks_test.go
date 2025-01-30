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
