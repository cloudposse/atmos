package list

import (
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestListStacks(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(cliConfig, "", nil, nil,
		nil, false, false, false)
	assert.Nil(t, err)

	output, err := FilterAndListStacks(stacksMap, "")
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.Nil(t, err)
	t.Log(dependentsYaml)
}

func TestListStacksWithComponent(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)
	component := "infra/vpc"

	stacksMap, err := e.ExecuteDescribeStacks(cliConfig, component, nil, nil,
		nil, false, false, false)
	assert.Nil(t, err)

	output, err := FilterAndListStacks(stacksMap, component)
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.Nil(t, err)
	t.Log(dependentsYaml)
}
