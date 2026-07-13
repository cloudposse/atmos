package list

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/degradation"
)

// describeStacksErrorOptions builds the DescribeStacksErrorOptions for a describe-stacks
// call from the --error-mode flag value ("strict", "warn", or "silent"), along with a
// Collector the caller should pass to printErrorModeSummary after the command's output is
// written. The returned Collector is nil when errorMode is "strict" (or any other
// unrecognized value), since nothing is ever degraded in that mode.
//
// "warn" and "silent" both enable lenient substitution (degradation.AtmosComputedValue for
// a recoverable YAML-function error) via the same Collector.Add callback; they differ only
// in whether the caller ends up printing a summary — silent mode intentionally never does,
// so no end-of-command warning is shown, while full detail remains available via
// --logs-level=Debug in both modes.
//
// A single Collector must be shared across every ExecuteDescribeStacksWithOptions call
// within one command invocation (e.g. describe affected's HEAD-side and BASE-side calls)
// so the end-of-command summary reports one combined count, not one per call site.
func describeStacksErrorOptions(errorMode string) (e.DescribeStacksErrorOptions, *degradation.Collector) {
	if errorMode != "warn" && errorMode != "silent" {
		return e.DescribeStacksErrorOptions{}, nil
	}
	collector := &degradation.Collector{}
	return e.DescribeStacksErrorOptions{
		OnError:   e.OnErrorWarn,
		OnWarning: collector.Add,
	}, collector
}

// printErrorModeSummary prints the collector's end-of-command summary only when errorMode
// is "warn". Safe to call with a nil collector (e.g. when errorMode is "strict"/"silent").
func printErrorModeSummary(errorMode string, collector *degradation.Collector) {
	if errorMode == "warn" {
		collector.Summary()
	}
}
