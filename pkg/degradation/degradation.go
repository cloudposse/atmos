// Package degradation supports graceful degradation for describe/list commands: when a
// recoverable YAML-function error occurs (e.g. a Terraform backend that has not been
// provisioned yet), the caller may substitute AtmosComputedValue for the unresolved value
// and continue instead of aborting. Warning tracks one such substitution, and Collector
// accumulates them across a single command run for an end-of-command summary.
package degradation

import (
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// AtmosComputedValue is substituted for a value that could not be resolved — most often
// because the underlying Terraform backend/state isn't accessible yet (not provisioned,
// not applied, not authenticated, or not the caller's most recent state). It is only
// substituted when a caller opts into lenient processing (list/describe
// --error-mode=warn|silent).
//
// AtmosComputedValue renders as the literal "(computed)" in every output format without
// any renderer needing to special-case it: text/template and fmt.Fprint call String()
// automatically for any fmt.Stringer, and encoding/json and gopkg.in/yaml.v3 call
// MarshalJSON/MarshalYAML automatically for any value implementing those interfaces.
type AtmosComputedValue struct{}

// String implements fmt.Stringer.
func (AtmosComputedValue) String() string {
	defer perf.Track(nil, "degradation.AtmosComputedValue.String")()

	return "(computed)"
}

// MarshalJSON implements json.Marshaler.
func (AtmosComputedValue) MarshalJSON() ([]byte, error) {
	defer perf.Track(nil, "degradation.AtmosComputedValue.MarshalJSON")()

	return []byte(`"(computed)"`), nil
}

// MarshalYAML implements yaml.Marshaler (gopkg.in/yaml.v3).
func (AtmosComputedValue) MarshalYAML() (interface{}, error) {
	defer perf.Track(nil, "degradation.AtmosComputedValue.MarshalYAML")()

	return "(computed)", nil
}

// Warning describes one value that could not be resolved and was substituted with
// AtmosComputedValue.
type Warning struct {
	Stack     string
	Component string
	Function  string
	Reason    string
}

// Collector accumulates Warnings during a single command run and can produce a one-line
// UI-facing summary. Not safe for concurrent use — matches the single-threaded
// describe-stacks processing pass.
type Collector struct {
	warnings []Warning
}

// Add records one degraded value. The full detail is always logged at debug level
// (visible with --logs-level=Debug); whether a user-facing summary is ever shown is up to
// the caller — see Summary.
func (c *Collector) Add(w Warning) {
	defer perf.Track(nil, "degradation.Collector.Add")()

	log.Debug("value could not be resolved; substituted (computed)",
		"stack", w.Stack, "component", w.Component, "function", w.Function, "reason", w.Reason)
	c.warnings = append(c.warnings, w)
}

// Count returns the number of warnings collected so far.
func (c *Collector) Count() int {
	defer perf.Track(nil, "degradation.Collector.Count")()

	if c == nil {
		return 0
	}
	return len(c.warnings)
}

// Summary prints one ui.Warningf line naming how many values could not be determined, if
// any were collected. No-op when nothing was collected (or c is nil).
func (c *Collector) Summary() {
	defer perf.Track(nil, "degradation.Collector.Summary")()

	if c.Count() == 0 {
		return
	}
	ui.Warningf("%d value(s) could not be determined and are shown as (computed). Run with --logs-level=Debug for details, or --error-mode=strict to fail immediately instead.", c.Count())
}
