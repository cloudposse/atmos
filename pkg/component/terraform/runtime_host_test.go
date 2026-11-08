package terraform_test

import (
	host "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/component/terraform"
)

func init() {
	terraform.NewTestExecutor = host.NewTerraformExecutor
}
