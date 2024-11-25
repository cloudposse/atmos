package exec

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func checkTerraformConfig(cliConfig schema.CliConfiguration) error {
	if len(cliConfig.Components.Terraform.BasePath) < 1 {
		return errors.New("Base path to terraform components must be provided in 'components.terraform.base_path' config or " +
			"'ATMOS_COMPONENTS_TERRAFORM_BASE_PATH' ENV variable")
	}

	return nil
}

// cleanTerraformWorkspace deletes the `.terraform/environment` file from the component directory.
// The `.terraform/environment` file contains the name of the currently selected workspace,
// helping Terraform identify the active workspace context for managing your infrastructure.
// We delete the file to prevent the Terraform prompt asking to select the default or the
// previously used workspace. This happens when different backends are used for the same component.
func cleanTerraformWorkspace(cliConfig schema.CliConfiguration, componentPath string) {
	filePath := filepath.Join(componentPath, ".terraform", "environment")
	u.LogDebug(cliConfig, fmt.Sprintf("\nDeleting Terraform environment file:\n'%s'", filePath))
	_ = os.Remove(filePath)
}
