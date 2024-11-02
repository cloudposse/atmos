package exec

import (
	"os"
	"path"

	"github.com/pkg/errors"

	"github.com/cloudposse/atmos/pkg/schema"
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
// We delete the file to prevent the Terraform prompt asking o select the default or the
// previously used workspace. This happens when different backends are used for the same component.
func cleanTerraformWorkspace(componentPath string) {
	_ = os.Remove(path.Join(componentPath, ".terraform", "environment"))
}
