package step

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RunGrouped runs a single step's execution (fn) wrapped in a collapsible CI
// log group when grouping is active for the current run (see ci.Group). The
// group label is the step `name` when set, falling back to the resolved
// `command`. Outside CI — or when grouping is disabled — fn runs unchanged.
//
// This is the one place the step abstraction owns per-step CI log grouping, so
// both the workflow executor and the custom-command runner get identical
// behavior across every step type by routing their per-step dispatch through it.
func RunGrouped(atmosConfig *schema.AtmosConfiguration, name, command string, fn func() error) error {
	defer perf.Track(nil, "step.RunGrouped")()

	return ci.Group(atmosConfig, ci.DimensionStep, groupLabel(name, command), fn)
}

// groupLabel picks the human-facing group label: the step name when present,
// otherwise the resolved command.
func groupLabel(name, command string) string {
	if label := strings.TrimSpace(name); label != "" {
		return label
	}
	return strings.TrimSpace(command)
}
