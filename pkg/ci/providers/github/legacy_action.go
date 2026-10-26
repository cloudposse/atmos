package github

import (
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

// legacyActionRepos is the exact set of marketplace actions that predate
// Native CI and are documented as deprecated at
// https://atmos.tools/deprecated/github-actions. Kept as an explicit set
// (rather than a "cloudposse/github-action-atmos-*" prefix match) because
// github-action-terraform-plan-storage — also deprecated — doesn't match
// that prefix, while other cloudposse/github-action-* repos (e.g.
// github-action-setup-atmos) are still current and must not match.
var legacyActionRepos = map[string]bool{
	"cloudposse/github-action-atmos-terraform-plan":              true,
	"cloudposse/github-action-atmos-terraform-apply":             true,
	"cloudposse/github-action-atmos-affected-stacks":             true,
	"cloudposse/github-action-atmos-terraform-drift-detection":   true,
	"cloudposse/github-action-atmos-terraform-drift-remediation": true,
	"cloudposse/github-action-atmos-component-updater":           true,
	"cloudposse/github-action-terraform-plan-storage":            true,
	"cloudposse/github-action-atmos-get-setting":                 true,
	"cloudposse/github-action-atmos-terraform-select-components": true,
	"cloudposse/github-action-atmos-affected-trigger-spacelift":  true,
}

// LegacyActionRepo reports the owner/repo of the currently executing GitHub
// Action when it is one of the deprecated marketplace actions predating
// Native CI. GitHub Actions sets GITHUB_ACTION_REPOSITORY for the step
// currently running, including a composite action's own nested run: steps,
// so this also detects atmos invoked from inside such an action.
func LegacyActionRepo() (repo string, ok bool) {
	defer perf.Track(nil, "github.LegacyActionRepo")()

	repo = os.Getenv("GITHUB_ACTION_REPOSITORY")
	return repo, legacyActionRepos[repo]
}
