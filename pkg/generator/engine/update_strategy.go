package engine

import (
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// UpdateStrategy controls where a 3-way merge's "base" content comes from
// during --update. Named distinctly from merge.ConflictStrategy, which
// instead picks how a real ours/theirs divergence is resolved once base has
// already been determined -- an unrelated axis.
type UpdateStrategy int

const (
	// UpdateStrategyTracked is the default (zero value): base is read from
	// the target's own git history at a pinned commit (see SetupGitStorage).
	UpdateStrategyTracked UpdateStrategy = iota
	// UpdateStrategyRendered: base is a pristine re-render of the template at
	// the ref that produced what's currently on disk, using the answers
	// recorded from that generation (see SetupRenderedBaseStorage). Has no
	// dependency on the target's git history.
	UpdateStrategyRendered
)

// ParseUpdateStrategy parses a --update-strategy flag value. An empty string
// (flag not set) maps to the default UpdateStrategyTracked.
func ParseUpdateStrategy(s string) (UpdateStrategy, error) {
	defer perf.Track(nil, "engine.ParseUpdateStrategy")()

	switch s {
	case "", "tracked":
		return UpdateStrategyTracked, nil
	case "rendered":
		return UpdateStrategyRendered, nil
	default:
		return UpdateStrategyTracked, errUtils.Build(errUtils.ErrUnknownUpdateStrategy).
			WithExplanationf("Invalid --update-strategy value: `%s`", s).
			WithHint("Valid values are: tracked, rendered").
			WithExitCode(2).
			Err()
	}
}
