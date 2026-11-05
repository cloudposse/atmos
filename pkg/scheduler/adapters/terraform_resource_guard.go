package adapters

import (
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewTerraformResourceGuard shares Terraform's alias/workdir and deploy verification
// locks with schedulers dispatching isolated Atmos processes. One guard must be
// shared by every Terraform node in the invocation.
func NewTerraformResourceGuard(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) func(*dependency.Node) func() {
	defer perf.Track(config, "adapters.NewTerraformResourceGuard")()

	d := &TerraformDispatcher{
		info:                info,
		locks:               newTerraformResourceLocks(),
		lockDeploySourceDir: deployVerificationUsesSourceDir(config, info),
	}
	return d.lockTerraformResource
}
