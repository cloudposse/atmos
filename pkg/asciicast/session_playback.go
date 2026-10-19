package asciicast

import (
	"context"
	"fmt"
	"io"

	errUtils "github.com/cloudposse/atmos/errors"
	durationpkg "github.com/cloudposse/atmos/pkg/duration"
)

func runPlaybackAction(ctx context.Context, input io.Writer, state *sessionState, action *SessionAction, opts *SessionOptions) error {
	duration, err := durationpkg.ParseString(action.Duration)
	if err != nil || duration <= 0 || len(action.Actions) == 0 {
		return fmt.Errorf("%w: expected positive duration and nested actions", errUtils.ErrInvalidCastPlayback)
	}
	return opts.Recorder.WithPlaybackDuration(duration, func() error {
		for i := range action.Actions {
			if err := runAction(ctx, input, state, &action.Actions[i], opts); err != nil {
				return err
			}
		}
		return nil
	})
}
