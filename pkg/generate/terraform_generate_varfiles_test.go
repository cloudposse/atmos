package vender

import (
	e "github.com/cloudposse/atmos/internal/exec"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/stretchr/testify/assert"
	"os"
	"path"
	"strconv"
	"testing"
	"time"
)

func TestTerraformGenerateVarfiles(t *testing.T) {
	err := c.InitConfig()
	assert.Nil(t, err)

	tempDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
	assert.Nil(t, err)

	defer func(path string) {
		err := os.RemoveAll(path)
		assert.Nil(t, err)
	}(tempDir)

	var stacks []string
	var components []string
	filePattern := path.Join(tempDir, "varfiles/{tenant}-{environment}-{stage}-{component}.tfvars.json")
	format := "hcl"

	err = e.ExecuteTerraformGenerateVarfiles(filePattern, format, stacks, components)
	assert.Nil(t, err)
}
