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

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil,
		nil, false, false, false, false, nil)
	assert.Nil(t, err)

	output, err := FilterAndListComponents("", stacksMap)
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

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, testStack, nil, nil,
		nil, false, false, false, false, nil)
	assert.Nil(t, err)

	output, err := FilterAndListComponents(testStack, stacksMap)
	assert.Nil(t, err)
	assert.Nil(t, err)
	assert.NotNil(t, output)
	assert.Greater(t, len(output), 0)
	assert.ObjectsAreEqualValues([]string{"infra/vpc", "mixin/test-1", "mixin/test-2", "test/test-component", "test/test-component-override", "test/test-component-override-2", "test/test-component-override-3", "top-level-component1", "vpc", "vpc/new"}, output)
}
