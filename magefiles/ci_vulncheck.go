//go:build mage

package main

import (
	"context"

	"github.com/cloudposse/atmos/internal/ci/vulncheck"
)

// Vulncheck scans every package of the module with govulncheck and writes its
// SARIF report to output. The tool must already be installed and on PATH. The
// report is normalized before it replaces output: exact duplicate stacks, which
// the SARIF schema forbids and GitHub code scanning rejects, are removed while
// every finding and distinct stack is kept. Scanner failures and malformed
// output fail the target without touching an existing report. See
// internal/ci/vulncheck.
func (CI) Vulncheck(output string) error {
	return vulncheck.Scan(context.Background(), &vulncheck.Params{Output: output})
}
