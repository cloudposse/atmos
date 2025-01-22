package cmd

import _ "embed"

//go:embed markdown/atmos_terraform_usage.md
var terraform string

//go:embed markdown/atmos_terraform_plan_usage.md
var terraformPlan string

//go:embed markdown/atmos_terraform_apply_usage.md
var terraformApply string

type ExampleContent struct {
	Content    string
	Suggestion string
}

var examples map[string]ExampleContent = map[string]ExampleContent{
	"atmos_terraform": {
		Content:    terraform,
		Suggestion: "https://atmos.tools/cli/commands/terraform/usage",
	},
	"atmos_terraform_plan": {
		Content: terraformPlan,
		// TODO: We should update this once we have a page for terraform plan
		Suggestion: "https://atmos.tools/cli/commands/terraform/usage",
	},
	"atmos_terraform_apply": {
		Content: terraformApply,
		// TODO: We should update this once we have a page for terraform plan
		Suggestion: "https://atmos.tools/cli/commands/terraform/usage",
	},
}
