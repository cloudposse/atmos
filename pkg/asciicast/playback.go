package asciicast

import (
	"errors"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

type playbackEvent struct {
	time    time.Duration
	stream  string
	content string
}

type playbackBuffer struct {
	start    time.Duration
	previous time.Duration
	events   []playbackEvent
}

// WithPlaybackDuration scales all recorded events produced by run into duration.
// Live output is unchanged. Groups cannot overlap or nest; only the active group's
// events are buffered, and subsequent recording resumes at its new end time.
func (r *Recorder) WithPlaybackDuration(duration time.Duration, run func() error) (err error) {
	defer perf.Track(nil, "asciicast.Recorder.WithPlaybackDuration")()

	if r == nil {
		return fmt.Errorf("%w: requires an active cast recorder", errUtils.ErrInvalidCastPlayback)
	}
	if err = r.beginPlayback(duration); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, r.endPlayback(duration)) }()
	return run()
}

func (r *Recorder) recordingElapsed() time.Duration {
	if r.elapsed != nil {
		return r.elapsed()
	}
	return time.Since(r.started)
}

func (r *Recorder) beginPlayback(duration time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if duration <= 0 {
		return fmt.Errorf("%w: duration must be positive", errUtils.ErrInvalidCastPlayback)
	}
	if r.closed || r.playback != nil {
		return fmt.Errorf("%w: recorder is closed or a playback group is already active", errUtils.ErrInvalidCastPlayback)
	}
	r.playback = &playbackBuffer{
		start:    maxDuration(r.recordingElapsed()+r.timeOffset, r.lastEventTime),
		previous: r.lastEventTime,
	}
	return nil
}

func (r *Recorder) endPlayback(duration time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	buffer := r.playback
	r.playback = nil
	elapsed := r.recordingElapsed()
	span := maxDuration(elapsed+r.timeOffset, r.lastEventTime) - buffer.start
	r.lastEventTime = buffer.previous
	end := buffer.start + duration
	for _, event := range buffer.events {
		eventTime := buffer.start
		if span > 0 {
			eventTime += time.Duration(float64(event.time-buffer.start) / float64(span) * float64(duration))
		}
		if err := r.writeRelativeEvent(eventTime, event.stream, event.content); err != nil {
			return err
		}
	}
	// An empty event preserves the endpoint even for silent groups and trailing idle time.
	r.timeOffset = end - elapsed
	return r.writeRelativeEvent(end, "o", "")
}
