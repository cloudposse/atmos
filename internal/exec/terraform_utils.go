package exec

import (
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
