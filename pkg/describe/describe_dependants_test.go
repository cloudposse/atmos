package describe

import (
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	"testing"
)

func TestDescribeDependants(t *testing.T) {
	configAndStacksInfo := cfg.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	component := "test/test-component"
	stack := "tenant1-ue2-test-1"

	dependants, err := e.ExecuteDescribeDependants(cliConfig, component, stack)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(dependants))

	dependantsYaml, err := yaml.Marshal(dependants)
	assert.Nil(t, err)
	t.Log(string(dependantsYaml))
}
