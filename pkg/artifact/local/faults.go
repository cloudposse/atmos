package local

import (
	"errors"

	"github.com/cloudposse/atmos/pkg/perf"
)

// faultPoint names a point in the publication lifecycle at which tests simulate a crash.
type faultPoint string

// Fault points, in lifecycle order.
const (
	// The before-payload point fires after the pending state was recorded and before any payload
	// byte is staged.
	faultBeforePayload faultPoint = "before-payload"

	// The during-payload point fires while the first payload blob is being staged.
	faultDuringPayload faultPoint = "during-payload"

	// The after-metadata point fires after the Record, payload, and representation are in place
	// and before the reference is completed.
	faultAfterMetadata faultPoint = "after-metadata"

	// The after-commit point fires after the reference was completed, before cleanup and before
	// the caller sees the response.
	faultAfterCommit faultPoint = "after-commit"
)

// faultHook returns a non-nil error to simulate a crash at the point.
type faultHook func(point faultPoint) error

// crashError marks an injected crash. A crashed process cannot clean up after itself, so the
// publication leaves its staging and journal exactly as a real interruption would.
type crashError struct{ err error }

func (c *crashError) Error() string {
	defer perf.Track(nil, "local.crashError.Error")()

	return c.err.Error()
}

func (c *crashError) Unwrap() error {
	defer perf.Track(nil, "local.crashError.Unwrap")()

	return c.err
}

func (s *settings) fault(point faultPoint) error {
	if s.faults == nil {
		return nil
	}
	if err := s.faults(point); err != nil {
		return &crashError{err: err}
	}
	return nil
}

func isCrash(err error) bool {
	var crash *crashError
	return errors.As(err, &crash)
}
