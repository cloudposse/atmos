package hooks

import "strings"

type HookEvent string

const (
	BeforeTerraformInit   HookEvent = "before.terraform.init"
	AfterTerraformApply   HookEvent = "after.terraform.apply"
	BeforeTerraformApply  HookEvent = "before.terraform.apply"
	AfterTerraformPlan    HookEvent = "after.terraform.plan"
	BeforeTerraformPlan   HookEvent = "before.terraform.plan"
	BeforeTerraformDeploy HookEvent = "before.terraform.deploy"
	AfterTerraformDeploy  HookEvent = "after.terraform.deploy"
)

// IsPostExecution reports whether the event fires after terraform has already run
// (and therefore after terraform init has already completed).
// Store hooks use this to decide whether to skip terraform init when reading outputs:
// after-events can safely skip init because the workdir is already initialized;
// before-events must run init because the workdir may not be initialized yet.
func (e HookEvent) IsPostExecution() bool {
	return strings.HasPrefix(string(e), "after.")
}
