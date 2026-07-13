package list

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ui"
)

// describeStacksErrorOptions builds the DescribeStacksErrorOptions for a describe-stacks
// call from the --on-error flag value ("strict" or "warn"). Any value other than "warn"
// (including "strict" and the empty string) reproduces ExecuteDescribeStacks's historical
// fail-fast behavior.
func describeStacksErrorOptions(onError string) e.DescribeStacksErrorOptions {
	if onError != "warn" {
		return e.DescribeStacksErrorOptions{}
	}
	return e.DescribeStacksErrorOptions{
		OnError: e.OnErrorWarn,
		OnWarning: func(w e.DegradationWarning) {
			ui.Warningf("%s in stack %s: %s could not resolve (%s) — value set to null", w.Component, w.Stack, w.Function, w.Reason)
		},
	}
}
