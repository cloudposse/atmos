package describe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestDescribeDependents(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	component := "test/test-component"
	stack := "tenant1-ue2-test-1"

	dependents, err := e.ExecuteDescribeDependents(&atmosConfig, component, stack, false, true, true, nil)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(dependents))

	dependentsYaml, err := u.ConvertToYAML(dependents)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if dependentsYaml != "" {
				t.Logf("Dependents:\n%s", dependentsYaml)
			} else {
				t.Logf("Dependents (raw): %+v", dependents)
			}
		}
	})
}

func TestDescribeDependents2(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	component := "test/test-component"
	stack := "tenant1-ue2-dev"

	dependents, err := e.ExecuteDescribeDependents(&atmosConfig, component, stack, false, true, true, nil)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(dependents))

	dependentsYaml, err := u.ConvertToYAML(dependents)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if dependentsYaml != "" {
				t.Logf("Dependents:\n%s", dependentsYaml)
			} else {
				t.Logf("Dependents (raw): %+v", dependents)
			}
		}
	})
}
