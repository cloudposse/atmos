//go:build mage

package main

import "github.com/magefile/mage/mg"

// Changed runs go.mod validation, ensures custom-gcl is built (only if
// stale), then runs the patch-scoped golangci-lint pass against changes from
// origin/main. Equivalent to the previous `atmos lint changed` chain
// (gomodcheck -> custom-gcl -> run-custom-golangci-lint.sh).
func (Lint) Changed() error {
	mg.SerialDeps(Lint.GoModCheck, Lint.CustomGCL)
	return Lint{}.Precommit()
}
