package describe

import (
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	"testing"
)

func TestDescribeComponent(t *testing.T) {
	_, err := cfg.InitCliConfig(cfg.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	component := "test/test-component-override-3"
	stack := "tenant1-ue2-dev"

	componentSection, err := e.ExecuteDescribeComponent(component, stack)
	assert.Nil(t, err)

	componentSectionYaml, err := yaml.Marshal(componentSection)
	assert.Nil(t, err)
	t.Log(string(componentSectionYaml))
}
