package generate

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTerraformGenerateVarfiles(t *testing.T) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	tempDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
	assert.Nil(t, err)

	defer func(path string) {
		err := os.RemoveAll(path)
		assert.Nil(t, err)
	}(tempDir)

	var stacks []string
	var components []string
	filePattern := filepath.Join(tempDir, "varfiles/{tenant}-{environment}-{stage}-{component}.tfvars")
	format := "hcl"

	err = e.ExecuteTerraformGenerateVarfiles(atmosConfig, filePattern, format, stacks, components)
	assert.Nil(t, err)
}
