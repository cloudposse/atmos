package install

import _ "embed"

// GitHub Actions workflow templates.

//go:embed templates/atmos-pro-terraform-plan.yaml
var planWorkflowTemplate string

//go:embed templates/atmos-pro-terraform-apply.yaml
var applyWorkflowTemplate string

//go:embed templates/atmos-pro-terraform-drift-detection.yaml
var driftDetectionWorkflowTemplate string

//go:embed templates/atmos-pro-terraform-drift-remediation.yaml
var driftRemediationWorkflowTemplate string

// Auth profile template.

//go:embed templates/github-profile-atmos.yaml
var githubProfileTemplate string

// Stack configuration templates.

//go:embed templates/atmos-pro-mixin.yaml
var proMixinTemplate string

//go:embed templates/defaults-snippet.yaml
var defaultsSnippetTemplate string
