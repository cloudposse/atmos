// Package git registers the built-in `git` hook kind, which publishes
// component artifacts to a Git repository on Atmos lifecycle events
// (e.g. after.terraform.apply). See docs/prd/git-ops.md, "Terraform
// Lifecycle Git Publishing".
//
// The kind targets the current repository by default; setting `repository`
// targets a managed repository under the top-level git.repositories config
// (cloned/reconciled via the shared Git service). It inherits standard hooks
// semantics: event binding, --skip-hooks, and on_failure modes. All Git
// operations go through the pkg/git provider registry (safety rules, auth
// env composition, and push retry included).
package git

import (
	"github.com/cloudposse/atmos/pkg/hooks"

	// Register the default "cli" Git provider so the kind works wherever the
	// hooks engine runs, independent of the cmd/git command wiring.
	_ "github.com/cloudposse/atmos/pkg/git/providers/cli"
)

// kindName is the registered identifier users select with `kind: git`.
const kindName = "git"

func init() {
	if err := hooks.RegisterKind(&hooks.Kind{
		Name: kindName,
		// Publishing is a state-changing operation: a silently dropped commit
		// or push loses artifacts, so the default is "fail" (matching the
		// effective behavior of the store kind), unlike the advisory tool
		// kinds that default to "warn". Users may override with on_failure.
		OnFailure: hooks.OnFailureFail,
		Engine:    NewEngine(),
	}); err != nil {
		panic("failed to register built-in git kind: " + err.Error())
	}
}
