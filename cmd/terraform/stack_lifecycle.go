package terraform

import (
	"strconv"

	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/scheduler/adapters"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
	"github.com/cloudposse/atmos/pkg/terraform/plugin"
)

func init() {
	deploy := terraformStackHandler(lifecycle.Deploy)
	deploy.HookEvents = []string{string(hooks.BeforeTerraformDeploy), string(hooks.AfterTerraformDeploy)}
	lifecycle.MustRegister("terraform", lifecycle.Deploy, deploy)
	destroy := terraformStackHandler(lifecycle.Destroy)
	destroy.ApprovalArgs = []string{"-auto-approve"}
	// Single-component destroy currently does not fire user lifecycle hooks.
	lifecycle.MustRegister("terraform", lifecycle.Destroy, destroy)
}

func terraformStackHandler(event lifecycle.Event) *lifecycle.Handler {
	handler := lifecycle.NativeHandler("terraform", string(event))
	handler.NewResourceGuard = func(config *schema.AtmosConfiguration) func(*dependency.Node) func() {
		return adapters.NewTerraformResourceGuard(config, &schema.ConfigAndStacksInfo{SubCommand: string(event)})
	}
	handler.WorkerEnvironment = func(concurrency int) []string {
		return []string{plugin.DisableCacheEnv + "=" + strconv.FormatBool(concurrency > 1)}
	}
	return handler
}
