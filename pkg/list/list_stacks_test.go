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

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil,
		nil, false, true, true, false, nil, nil)
	assert.Nil(t, err)

	output, err := FilterAndListStacks(stacksMap, "")
	assert.Nil(t, err)
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.NotEmpty(t, dependentsYaml)
}

func TestListStacksWithComponent(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil,
		nil, false, true, true, false, nil, nil)
	assert.Nil(t, err)

	output, err := FilterAndListStacks(stacksMap, "eks-blue/cluster")
	assert.Nil(t, err)
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.Nil(t, err)

	// Verify the output structure
	assert.NotEmpty(t, dependentsYaml)
	// Verify that only stacks with the specified component are included
	assert.Contains(t, dependentsYaml, "tenant1-uw1-test-1")
	assert.Contains(t, dependentsYaml, "tenant1-uw2-test-1")
}

func TestFilterAndListStacks_NoMatchingComponent(t *testing.T) {
	stacksMap := map[string]any{
		"stack1": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"existing": map[string]any{},
				},
			},
		},
	}

	output, err := FilterAndListStacks(stacksMap, "missing")
	assert.NoError(t, err)
	assert.Nil(t, output)
}
