package hooks

type HookEvent string

const (
	BeforeTerraformInit  HookEvent = "before.terraform.init"
	AfterTerraformApply  HookEvent = "after.terraform.apply"
	BeforeTerraformApply HookEvent = "before.terraform.apply"
	AfterTerraformPlan   HookEvent = "after.terraform.plan"
	BeforeTerraformPlan  HookEvent = "before.terraform.plan"
)
