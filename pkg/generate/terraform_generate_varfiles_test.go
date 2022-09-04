package vender

import (
	e "github.com/cloudposse/atmos/internal/exec"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTerraformGenerateVarfiles(t *testing.T) {
	err := c.InitConfig()
	assert.Nil(t, err)

	var stacks []string
	var components []string
	filePattern := "./varfiles/{tenant}/{environment}/{stage}/{component}.tfvars.json"

	err = e.ExecuteTerraformGenerateVarfiles(filePattern, stacks, components)
	assert.Nil(t, err)
}
