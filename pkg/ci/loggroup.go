package ci

import (
	"io"
	"os"
	"strings"
	"sync/atomic"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Grouping modes for ci.groups.mode. CI providers do not support nested groups,
// so the mode selects a single, mutually-exclusive granularity.
const (
	// GroupModeAuto emits the finest grouping that applies to each command:
	// one group per workflow/custom-command step (DimensionStep) and one group
	// per phase of a terraform/helmfile/packer invocation (DimensionPhase).
	// This is the default when the mode is unset.
	GroupModeAuto = "auto"

	// GroupModeInvocation emits one group around each whole top-level
	// `atmos <command>` run (DimensionInvocation); finer step/phase grouping is
	// suppressed.
	GroupModeInvocation = "invocation"

	// GroupModeOff disables CI log grouping entirely.
	GroupModeOff = "off"
)

// Dimension identifies the boundary at which a caller wants to open a CI log
// group. Each dimension is emitted only under the grouping mode(s) that select
// it (see dimensionActive), which keeps the dimensions mutually exclusive.
type Dimension int

const (
	// DimensionStep is one group per workflow/custom-command step.
	DimensionStep Dimension = iota

	// DimensionPhase is one group per phase (init/plan/apply, …) of a single
	// component command invocation.
	DimensionPhase

	// DimensionInvocation is one group around a whole top-level `atmos` command.
	DimensionInvocation
)

// logGroupSentinelEnvVar is set in a step/command subprocess's environment while
// a parent Atmos process has grouping enabled. A child Atmos process that sees
// it skips its own grouping so nested `atmos` invocations do not emit
// unsupported nested groups.
const logGroupSentinelEnvVar = "ATMOS_CI_LOG_GROUP_ACTIVE"

// logGroupOut is where Group writes log-group markers. It defaults to os.Stdout
// — the stream the CI runner reads workflow commands from, and the stream that
// carries forwarded subprocess output, which keeps the start/content/end
// ordering correct — and is overridable in tests.
//
//nolint:gochecknoglobals // test seam for stdout-bound workflow commands.
var logGroupOut io.Writer = os.Stdout

// logGroupDepth guards against in-process re-entry: only the outermost Group
// emits markers.
//
//nolint:gochecknoglobals // process-wide nesting guard.
var logGroupDepth int32

// LogGroupSentinelEnv returns the "KEY=VALUE" environment entry that
// orchestrators append to a step/command subprocess's environment while
// grouping is enabled, so nested `atmos` invocations skip re-grouping. Callers
// append it only when GroupingEnabled reports true.
func LogGroupSentinelEnv() string {
	defer perf.Track(nil, "ci.LogGroupSentinelEnv")()

	return logGroupSentinelEnvVar + "=1"
}

// resolveGroupMode returns the effective grouping mode for the run: GroupModeOff
// when CI integration is disabled or grouping is turned off, otherwise the
// configured mode (defaulting to GroupModeAuto). An unrecognized value is
// treated as GroupModeAuto so a typo degrades to the safe default rather than
// silently disabling grouping.
func resolveGroupMode(atmosConfig *schema.AtmosConfiguration) string {
	if atmosConfig == nil || !atmosConfig.CI.Enabled {
		return GroupModeOff
	}
	switch strings.ToLower(strings.TrimSpace(atmosConfig.CI.Groups.Mode)) {
	case "", GroupModeAuto:
		return GroupModeAuto
	case GroupModeInvocation:
		return GroupModeInvocation
	case GroupModeOff, "none", "false", "disabled":
		return GroupModeOff
	default:
		return GroupModeAuto
	}
}

// dimensionActive reports whether dim should emit a group under mode.
func dimensionActive(mode string, dim Dimension) bool {
	switch dim {
	case DimensionStep, DimensionPhase:
		return mode == GroupModeAuto
	case DimensionInvocation:
		return mode == GroupModeInvocation
	default:
		return false
	}
}

// grouper returns the detected provider's log-group capability, or (nil, false)
// when no grouping-capable provider is active or a parent Atmos process already
// has grouping open (env sentinel).
func grouper(atmosConfig *schema.AtmosConfiguration) (provider.LogGrouper, bool) {
	if resolveGroupMode(atmosConfig) == GroupModeOff {
		return nil, false
	}
	// A parent Atmos run already has grouping open — do not nest.
	if os.Getenv(logGroupSentinelEnvVar) != "" {
		return nil, false
	}
	p := Detect()
	if p == nil {
		return nil, false
	}
	lg, ok := p.(provider.LogGrouper)
	if !ok {
		return nil, false
	}
	return lg, true
}

// GroupingEnabled reports whether CI log grouping is active for this run in any
// dimension: a grouping mode other than "off" is configured, a grouping-capable
// provider is detected, and no parent Atmos process already has grouping open.
//
// Orchestrators use it to decide whether to append LogGroupSentinelEnv to a
// child subprocess's environment — which must happen whenever grouping is
// enabled, regardless of which dimension the current command emits, so that a
// nested `atmos` invocation never emits its own (nested) groups.
func GroupingEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	defer perf.Track(nil, "ci.GroupingEnabled")()

	_, ok := grouper(atmosConfig)
	return ok
}

// Group runs fn wrapped in the detected CI provider's log-group markers, named
// `name`, when the configured mode selects the given dimension; otherwise it
// simply calls fn. The group-end marker is always emitted (even when fn returns
// an error or panics), so a failing operation never leaves a group open. Nested
// calls within the same process do not emit nested groups — only the outermost
// Group emits markers.
//
// Markers are written through a masking writer so a secret resolved into the
// group label (for example a step command containing a `!secret` value) is not
// leaked into the CI log.
func Group(atmosConfig *schema.AtmosConfiguration, dim Dimension, name string, fn func() error) error {
	defer perf.Track(nil, "ci.Group")()

	lg, ok := grouper(atmosConfig)
	if !ok || !dimensionActive(resolveGroupMode(atmosConfig), dim) {
		return fn()
	}

	// Only the outermost group emits markers; inner re-entry runs fn bare.
	if atomic.AddInt32(&logGroupDepth, 1) > 1 {
		defer atomic.AddInt32(&logGroupDepth, -1)
		return fn()
	}
	defer atomic.AddInt32(&logGroupDepth, -1)

	out := iolib.MaskWriter(logGroupOut)
	lg.StartGroup(out, name)
	defer lg.EndGroup(out)

	return fn()
}
