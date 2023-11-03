package describe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeDependents(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	component := "test/test-component"
	stack := "tenant1-ue2-test-1"

	dependents, err := e.ExecuteDescribeDependents(cliConfig, component, stack)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(dependents))

	dependentsYaml, err := yaml.Marshal(dependents)
	assert.Nil(t, err)
	t.Log(string(dependentsYaml))
}

func TestDescribeDependents2(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	component := "test/test-component"
	stack := "tenant1-ue2-dev"

	dependents, err := e.ExecuteDescribeDependents(cliConfig, component, stack)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(dependents))

	dependentsYaml, err := yaml.Marshal(dependents)
	assert.Nil(t, err)
	t.Log(string(dependentsYaml))
}
