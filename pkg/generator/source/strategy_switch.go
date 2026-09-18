package source

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// CheckNotSwitchedFromRendered errors if targetDir's recorded project state
// shows it was last managed with --update-strategy=rendered (spec.renderedRef
// set, spec.baseRef empty), so a --update-strategy=tracked run doesn't
// silently attempt a 3-way merge against a target that was deliberately
// generated with no git-history dependency -- it would either read stale
// history from before rendered mode took over, or fail with the generic,
// less-helpful "requires a git repository" message instead of naming what
// actually happened.
//
// A missing or never-updated project record is not an error here: only a
// confirmed rendered-mode history is a real switch worth flagging.
func CheckNotSwitchedFromRendered(targetDir string) error {
	defer perf.Track(nil, "source.CheckNotSwitchedFromRendered")()

	record, err := config.LoadProjectRecord(targetDir)
	if err != nil {
		return fmt.Errorf("failed to load the existing project record: %w", err)
	}
	if record == nil {
		return nil
	}
	if record.Spec.RenderedRef != "" && record.Spec.BaseRef == "" {
		return errUtils.Build(errUtils.ErrUpdateStrategySwitchedToTracked).
			WithExplanationf("`%s` was last updated with `--update-strategy=rendered`", targetDir).
			WithHint("Rendered mode never records a git-history base ref, so tracked mode has nothing reliable to diff against").
			WithHint("Re-run with `--update-strategy=rendered`, or `--force` to fully regenerate and start fresh with `tracked`").
			WithContext("target_dir", targetDir).
			WithExitCode(2).
			Err()
	}
	return nil
}
