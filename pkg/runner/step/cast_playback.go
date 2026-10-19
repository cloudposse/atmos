package step

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	durationpkg "github.com/cloudposse/atmos/pkg/duration"
	"github.com/cloudposse/atmos/pkg/schema"
)

// castPlaybackStepType groups a segment of a cast into a fixed playback duration.
const castPlaybackStepType = "playback"

func validateCastPlaybackGroup(parent, group *schema.WorkflowStep, session bool) error {
	duration, err := durationpkg.ParseString(group.Duration)
	if err != nil || duration <= 0 || len(group.Steps) == 0 {
		return fmt.Errorf("%w: expected positive duration and nested steps", errUtils.ErrInvalidCastPlayback)
	}
	if containsPlaybackGroup(group.Steps) {
		return fmt.Errorf("%w: groups cannot nest", errUtils.ErrInvalidCastPlayback)
	}
	if session {
		for i := range group.Steps {
			if err := validateCastSessionAction(parent, &group.Steps[i]); err != nil {
				return err
			}
		}
		return nil
	}
	clone := *parent
	clone.Steps = group.Steps
	return validateCastStepsMode(&clone)
}

func containsPlaybackGroup(steps []schema.WorkflowStep) bool {
	for i := range steps {
		if steps[i].Type == castPlaybackStepType || containsPlaybackGroup(steps[i].Steps) {
			return true
		}
	}
	return false
}

func (r castChildStepRunner) runPlaybackGroup(group *schema.WorkflowStep) error {
	if err := validateCastPlaybackGroup(r.castStep, group, false); err != nil {
		return err
	}
	duration, _ := durationpkg.ParseString(group.Duration)
	clone := *r.castStep
	clone.Steps = group.Steps
	nested := r
	nested.castStep = &clone
	return r.recorder.WithPlaybackDuration(duration, nested.runSteps)
}
