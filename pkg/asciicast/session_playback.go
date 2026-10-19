package asciicast

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
)

func runPlaybackAction(ctx context.Context, input *syncWriter, state *sessionState, action *SessionAction, opts *SessionOptions) error {
	duration, err := time.ParseDuration(action.Duration)
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
