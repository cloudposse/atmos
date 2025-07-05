package exec

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// GetTerraformBackendLocal returns the Terraform state from the local backend.
func GetTerraformBackendLocal(
	atmosConfig *schema.AtmosConfiguration,
	backendInfo *TerraformBackendInfo,
) (map[string]any, error) {
	tfStateFilePath := filepath.Join(
		atmosConfig.TerraformDirAbsolutePath,
		backendInfo.TerraformComponent,
		"terraform.tfstate.d",
		backendInfo.Workspace,
		"terraform.tfstate",
	)

	if !u.FileExists(tfStateFilePath) {
		return nil, fmt.Errorf("%w.\npath: `%s`", errUtils.ErrMissingTerraformStateFile, tfStateFilePath)
	}

	content, err := os.ReadFile(tfStateFilePath)
	if err != nil {
		return nil, fmt.Errorf("%w.\npath: `%s`\nerror: %v", errUtils.ErrReadFile, tfStateFilePath, err)
	}

	data, err := ProcessTerraformStateFile(content)
	if err != nil {
		return nil, fmt.Errorf("%w.\npath: `%s`\nerror: %v", errUtils.ErrProcessTerraformStateFile, tfStateFilePath, err)
	}

	return data, nil
}
