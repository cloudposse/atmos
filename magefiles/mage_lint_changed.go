//go:build mage

package main

import "github.com/magefile/mage/mg"

// Changed runs go.mod validation, ensures custom-gcl is built (only if
// stale), then runs the patch-scoped golangci-lint pass against changes from
// origin/main. Equivalent to the previous `atmos lint changed` chain
// (gomodcheck -> custom-gcl -> run-custom-golangci-lint.sh).
//
// No dedicated test: this is a one-line composition with no branching logic
// of its own. Every target it calls (GoModCheck, CustomGCL, Precommit) is
// unit-tested independently; meaningfully testing Changed itself would mean
// re-running CustomGCL's real `golangci-lint custom` network fetch + full
// recompile, which is integration-test territory this package exists to
// make lazy/cached, not something worth re-running in the unit-test tier.
func (Lint) Changed() error {
	mg.SerialDeps(Lint.GoModCheck, Lint.CustomGCL)
	return Lint{}.Precommit()
}
