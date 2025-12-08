package generate

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTerraformGenerateVarfiles(t *testing.T) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	tempDir := t.TempDir()

	var stacks []string
	var components []string
	filePattern := filepath.Join(tempDir, "varfiles/{tenant}-{environment}-{stage}-{component}.tfvars")
	format := "hcl"

	err = e.ExecuteTerraformGenerateVarfiles(&atmosConfig, filePattern, format, stacks, components)
	assert.Nil(t, err)
}
