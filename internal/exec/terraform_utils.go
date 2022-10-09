package exec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/pkg/errors"
)

func checkTerraformConfig(cliConfig cfg.CliConfiguration) error {
	if len(cliConfig.Components.Terraform.BasePath) < 1 {
		return errors.New("Base path to terraform components must be provided in 'components.terraform.base_path' config or " +
			"'ATMOS_COMPONENTS_TERRAFORM_BASE_PATH' ENV variable")
	}

	return nil
}
