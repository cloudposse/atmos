package describe

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	"testing"
)

func TestDescribeComponent(t *testing.T) {
	component := "test/test-component-override-3"
	stack := "tenant1-ue2-dev"

	componentSection, err := e.ExecuteDescribeComponent(component, stack)
	assert.Nil(t, err)

	componentSectionYaml, err := yaml.Marshal(componentSection)
	assert.Nil(t, err)
	t.Log(string(componentSectionYaml))
}

func TestDescribeTemplatedComponent(t *testing.T) {
	componentSection, err := e.ExecuteDescribeComponent("eks-blue/cluster", "tenant1-uw2-test-1")
	assert.Nil(t, err)
	assert.Equal(t, "eks-blue", componentSection["vars"].(map[any]any)["name"])

	componentSection, err = e.ExecuteDescribeComponent("eks-green/cluster", "tenant1-uw2-test-1")
	assert.Nil(t, err)
	assert.Equal(t, "eks-green", componentSection["vars"].(map[any]any)["name"])
}
