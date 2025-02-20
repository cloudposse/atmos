package cmd

import _ "embed"

//go:embed markdown/atmos_terraform_usage.md
var terraformUsage string

//go:embed markdown/atmos_terraform_plan_usage.md
var terraformPlanUsage string

//go:embed markdown/atmos_terraform_apply_usage.md
var terraformApplyUsage string

//go:embed markdown/atmos_workflow_usage.md
var workflowUsage string

//go:embed markdown/atmos_about_usage.md
var atmosAboutUsage string

type ExampleContent struct {
	Content    string
	Suggestion string
}

const (
	doubleDashHint string = "Use double dashes to separate Atmos-specific options from native arguments and flags for the command."
	stackHint      string = "The `stack` flag specifies the environment or configuration set for deployment in Atmos CLI."
	componentHint  string = "The `component` flag specifies the name of the component to be managed or deployed in Atmos CLI."
)

var examples map[string]ExampleContent = map[string]ExampleContent{
	"atmos_terraform": {
		Content:    terraformUsage,
		Suggestion: "https://atmos.tools/cli/commands/terraform/usage",
	},
	"atmos_terraform_plan": {
		Content: terraformPlanUsage,
		// TODO: We should update this once we have a page for terraform plan
		Suggestion: "https://atmos.tools/cli/commands/terraform/usage",
	},
	"atmos_terraform_apply": {
		Content: terraformApplyUsage,
		// TODO: We should update this once we have a page for terraform plan
		Suggestion: "https://atmos.tools/cli/commands/terraform/usage",
	},
	"atmos_workflow": {
		Content:    workflowUsage,
		Suggestion: "https://atmos.tools/cli/commands/workflow/",
	},
	"atmos_about": {
		Content: atmosAboutUsage,
	},
}
