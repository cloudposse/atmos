package terraform_backend

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ReadTerraformBackendLocal reads the Terraform state file from the local backend.
func ReadTerraformBackendLocal(
	atmosConfig *schema.AtmosConfiguration,
	backendInfo *TerraformBackendInfo,
) ([]byte, error) {
	tfStateFilePath := filepath.Join(
		atmosConfig.TerraformDirAbsolutePath,
		backendInfo.TerraformComponent,
		"terraform.tfstate.d",
		backendInfo.Workspace,
		"terraform.tfstate",
	)

	// If the state file does not exist (the component in the stack has not been provisioned yet),
	// return a `nil` result and no error.
	if !u.FileExists(tfStateFilePath) {
		return nil, nil
	}

	content, err := os.ReadFile(tfStateFilePath)
	if err != nil {
		return nil, fmt.Errorf("%w.\npath: `%s`\nerror: %v", errUtils.ErrReadFile, tfStateFilePath, err)
	}

	return content, nil
}
