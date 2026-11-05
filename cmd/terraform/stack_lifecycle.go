package terraform

import (
	"strconv"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/scheduler/adapters"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
	"github.com/cloudposse/atmos/pkg/terraform/plugin"
)

func init() {
	plan := terraformStackHandler(lifecycle.PlanEvent)
	plan.HookEvents = []string{string(hooks.BeforeTerraformPlan), string(hooks.AfterTerraformPlan)}
	lifecycle.MustRegister(cfg.TerraformComponentType, lifecycle.PlanEvent, plan)
	test := terraformStackHandler(lifecycle.Test)
	test.HookEvents = []string{string(hooks.BeforeTerraformTest), string(hooks.AfterTerraformTest)}
	lifecycle.MustRegister(cfg.TerraformComponentType, lifecycle.Test, test)
	deploy := terraformStackHandler(lifecycle.Deploy)
	deploy.HookEvents = []string{string(hooks.BeforeTerraformDeploy), string(hooks.AfterTerraformDeploy)}
	lifecycle.MustRegister(cfg.TerraformComponentType, lifecycle.Deploy, deploy)
	destroy := terraformStackHandler(lifecycle.Destroy)
	destroy.ApprovalArgs = []string{"-auto-approve"}
	// Single-component destroy currently does not fire user lifecycle hooks.
	lifecycle.MustRegister(cfg.TerraformComponentType, lifecycle.Destroy, destroy)
}

func terraformStackHandler(event lifecycle.Event) *lifecycle.Handler {
	handler := lifecycle.NativeHandler(cfg.TerraformComponentType, string(event))
	handler.NewResourceGuard = func(config *schema.AtmosConfiguration) func(*dependency.Node) func() {
		return adapters.NewTerraformResourceGuard(config, &schema.ConfigAndStacksInfo{SubCommand: string(event)})
	}
	handler.WorkerEnvironment = func(concurrency int) []string {
		return []string{plugin.DisableCacheEnv + "=" + strconv.FormatBool(concurrency > 1)}
	}
	return handler
}
