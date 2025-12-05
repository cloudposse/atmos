package terraform_backend

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ReadTerraformBackendLocal reads the Terraform state file from the local backend.
// If the state file does not exist, the function returns `nil`.
func ReadTerraformBackendLocal(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	_ *schema.AuthContext, // Auth context not used for local backend.
) ([]byte, error) {
	defer perf.Track(atmosConfig, "terraform_backend.ReadTerraformBackendLocal")()

	tfStateFilePath := filepath.Join(
		atmosConfig.TerraformDirAbsolutePath,
		GetTerraformComponent(componentSections),
		"terraform.tfstate.d",
		GetTerraformWorkspace(componentSections),
		"terraform.tfstate",
	)

	// If the state file does not exist (the component in the stack has not been provisioned yet), return a `nil` result and no error.
	if !u.FileExists(tfStateFilePath) {
		return nil, nil
	}

	content, err := os.ReadFile(tfStateFilePath)
	if err != nil {
		return nil, fmt.Errorf("%w.\npath: `%s`\nerror:w%v", errUtils.ErrReadFile, tfStateFilePath, err)
	}

	return content, nil
}
