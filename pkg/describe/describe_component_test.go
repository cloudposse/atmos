package describe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestDescribeComponent(t *testing.T) {
	component := "test/test-component-override-3"
	stack := "tenant1-ue2-dev"

	componentSection, err := e.ExecuteDescribeComponent(component, stack, true, true, nil)
	assert.Nil(t, err)

	componentSectionYaml, err := u.ConvertToYAML(componentSection)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if componentSectionYaml != "" {
				t.Logf("Component section:\n%s", componentSectionYaml)
			} else {
				t.Logf("Component section (raw): %+v", componentSection)
			}
		}
	})
}

func TestDescribeTemplatedComponent(t *testing.T) {
	componentSection, err := e.ExecuteDescribeComponent("eks-blue/cluster", "tenant1-uw2-test-1", true, true, nil)
	assert.Nil(t, err)
	assert.Equal(t, "eks-blue", componentSection["vars"].(map[string]any)["name"])

	componentSection, err = e.ExecuteDescribeComponent("eks-green/cluster", "tenant1-uw2-test-1", true, true, nil)
	assert.Nil(t, err)
	assert.Equal(t, "eks-green", componentSection["vars"].(map[string]any)["name"])
}

func TestDescribeComponent2(t *testing.T) {
	component := "derived-component-3"
	stack := "tenant1-ue2-test-1"

	componentSection, err := e.ExecuteDescribeComponent(component, stack, true, true, nil)
	assert.Nil(t, err)

	componentSectionYaml, err := u.ConvertToYAML(componentSection)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if componentSectionYaml != "" {
				t.Logf("Component section:\n%s", componentSectionYaml)
			} else {
				t.Logf("Component section (raw): %+v", componentSection)
			}
		}
	})
}

func TestDescribeComponent3(t *testing.T) {
	component := "service-iam-role/webservices/prod"
	stack := "tenant2-ue2-prod"

	componentSection, err := e.ExecuteDescribeComponent(component, stack, true, true, nil)
	assert.Nil(t, err)

	componentSectionYaml, err := u.ConvertToYAML(componentSection)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if componentSectionYaml != "" {
				t.Logf("Component section:\n%s", componentSectionYaml)
			} else {
				t.Logf("Component section (raw): %+v", componentSection)
			}
		}
	})
}

func TestDescribeComponent5(t *testing.T) {
	component := "test/test-component"
	stack := "tenant1-uw2-dev"

	componentSection, err := e.ExecuteDescribeComponent(component, stack, true, true, nil)
	assert.Nil(t, err)

	componentSectionYaml, err := u.ConvertToYAML(componentSection)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if componentSectionYaml != "" {
				t.Logf("Component section:\n%s", componentSectionYaml)
			} else {
				t.Logf("Component section (raw): %+v", componentSection)
			}
		}
	})
}

func TestDescribeComponent6(t *testing.T) {
	component := "infra/vpc"
	stack := "tenant1-ue2-dev"

	componentSection, err := e.ExecuteDescribeComponent(component, stack, true, true, nil)
	assert.Nil(t, err)

	componentSectionYaml, err := u.ConvertToYAML(componentSection)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if componentSectionYaml != "" {
				t.Logf("Component section:\n%s", componentSectionYaml)
			} else {
				t.Logf("Component section (raw): %+v", componentSection)
			}
		}
	})
}

func TestDescribeComponent7(t *testing.T) {
	component := "infra/vpc"
	stack := "tenant2-ue2-dev"

	componentSection, err := ExecuteDescribeComponent(component, stack, true, true, nil)
	assert.Nil(t, err)

	componentSectionYaml, err := u.ConvertToYAML(componentSection)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if componentSectionYaml != "" {
				t.Logf("Component section:\n%s", componentSectionYaml)
			} else {
				t.Logf("Component section (raw): %+v", componentSection)
			}
		}
	})
}
