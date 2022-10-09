package vender

import (
	"github.com/stretchr/testify/assert"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestTerraformGenerateVarfiles(t *testing.T) {
	cliConfig, err := cfg.InitCliConfig(cfg.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	tempDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
	assert.Nil(t, err)

	defer func(path string) {
		err := os.RemoveAll(path)
		assert.Nil(t, err)
	}(tempDir)

	var stacks []string
	var components []string
	filePattern := path.Join(tempDir, "varfiles/{tenant}-{environment}-{stage}-{component}.tfvars")
	format := "hcl"

	err = e.ExecuteTerraformGenerateVarfiles(cliConfig, filePattern, format, stacks, components)
	assert.Nil(t, err)
}
