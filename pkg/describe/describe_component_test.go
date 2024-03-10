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

func TestDescribeComponent2(t *testing.T) {
	component := "derived-component-3"
	stack := "tenant1-ue2-test-1"

	componentSection, err := e.ExecuteDescribeComponent(component, stack)
	assert.Nil(t, err)

	componentSectionYaml, err := yaml.Marshal(componentSection)
	assert.Nil(t, err)
	t.Log(string(componentSectionYaml))
}

func TestDescribeComponent3(t *testing.T) {
	component := "service-iam-role/webservices/prod"
	stack := "tenant2-ue2-prod"

	componentSection, err := e.ExecuteDescribeComponent(component, stack)
	assert.Nil(t, err)

	componentSectionYaml, err := yaml.Marshal(componentSection)
	assert.Nil(t, err)
	t.Log(string(componentSectionYaml))
}

func TestDescribeComponent5(t *testing.T) {
	component := "test/test-component"
	stack := "tenant1-uw2-dev"

	componentSection, err := e.ExecuteDescribeComponent(component, stack)
	assert.Nil(t, err)

	componentSectionYaml, err := yaml.Marshal(componentSection)
	assert.Nil(t, err)
	t.Log(string(componentSectionYaml))
}

func TestDescribeComponent6(t *testing.T) {
	component := "infra/vpc"
	stack := "tenant1-ue2-dev"

	componentSection, err := e.ExecuteDescribeComponent(component, stack)
	assert.Nil(t, err)

	componentSectionYaml, err := yaml.Marshal(componentSection)
	assert.Nil(t, err)
	t.Log(string(componentSectionYaml))
}
