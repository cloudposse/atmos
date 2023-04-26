package describe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeStacks(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacks, err := ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, false)
	assert.Nil(t, err)

	dependentsYaml, err := yaml.Marshal(stacks)
	assert.Nil(t, err)
	t.Log(string(dependentsYaml))
}

func TestDescribeStacksWithFilter1(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"

	stacks, err := ExecuteDescribeStacks(cliConfig, stack, nil, nil, nil, false)
	assert.Nil(t, err)

	dependentsYaml, err := yaml.Marshal(stacks)
	assert.Nil(t, err)
	t.Log(string(dependentsYaml))
}

func TestDescribeStacksWithFilter2(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"
	components := []string{"infra/vpc"}

	stacks, err := ExecuteDescribeStacks(cliConfig, stack, components, nil, nil, false)
	assert.Nil(t, err)

	dependentsYaml, err := yaml.Marshal(stacks)
	assert.Nil(t, err)
	t.Log(string(dependentsYaml))
}

func TestDescribeStacksWithFilter3(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stack := "tenant1-ue2-dev"
	sections := []string{"vars"}

	stacks, err := ExecuteDescribeStacks(cliConfig, stack, nil, nil, sections, false)
	assert.Nil(t, err)

	dependentsYaml, err := yaml.Marshal(stacks)
	assert.Nil(t, err)
	t.Log(string(dependentsYaml))
}

func TestDescribeStacksWithFilter4(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	componentTypes := []string{"terraform"}
	sections := []string{"none"}

	stacks, err := ExecuteDescribeStacks(cliConfig, "", nil, componentTypes, sections, false)
	assert.Nil(t, err)

	dependentsYaml, err := yaml.Marshal(stacks)
	assert.Nil(t, err)
	t.Log(string(dependentsYaml))
}
