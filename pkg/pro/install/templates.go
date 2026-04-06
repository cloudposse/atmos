package install

import _ "embed"

// GitHub Actions workflow templates.

//go:embed templates/workflows/atmos-pro-terraform-plan.yaml
var planWorkflowTemplate string

//go:embed templates/workflows/atmos-pro-terraform-apply.yaml
var applyWorkflowTemplate string

//go:embed templates/workflows/atmos-pro-affected-stacks.yaml
var affectedStacksWorkflowTemplate string

//go:embed templates/workflows/atmos-pro-list-instances.yaml
var listInstancesWorkflowTemplate string

// Auth profile templates.

//go:embed templates/profiles/github-plan.yaml
var githubPlanProfileTemplate string

//go:embed templates/profiles/github-apply.yaml
var githubApplyProfileTemplate string

//go:embed templates/profiles/README.md
var profilesReadmeTemplate string

// Stack configuration templates.

//go:embed templates/mixins/atmos-pro.yaml
var proMixinTemplate string

//go:embed templates/stacks/defaults-snippet.yaml
var defaultsSnippetTemplate string
