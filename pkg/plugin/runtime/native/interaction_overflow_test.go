package native

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestInteractiveDeadlineSurvivesHugeCallTimeout guards the hard deadline of an
// interactive call against overflow. The call timeout has no upper bound, so a very
// large one plus the interaction limit must still produce a deadline in the future
// rather than wrapping to a negative duration, which would silently drop the deadline
// and leave the call unbounded.
func TestInteractiveDeadlineSurvivesHugeCallTimeout(t *testing.T) {
	handle := &Handle{callTimeout: time.Duration(math.MaxInt64) - time.Minute, interactionLimit: DefaultInteractionLimit}

	callCtx, work, cancel := handle.callContext(t.Context(), true)
	defer cancel()

	require.NotNil(t, work, "an interactive call with a timeout gets a work budget")
	deadline, ok := callCtx.Deadline()
	require.True(t, ok, "an interactive call with a timeout must keep a hard deadline")
	require.True(t, deadline.After(time.Now()), "the hard deadline must be in the future, got %s", deadline)
	require.NoError(t, callCtx.Err(), "the call must not be cancelled on creation")
}

// TestSaturatingDurationAdd verifies the capped addition used for the hard deadline.
func TestSaturatingDurationAdd(t *testing.T) {
	maxDuration := time.Duration(math.MaxInt64)
	cases := []struct {
		name string
		a, b time.Duration
		want time.Duration
	}{
		{"small values add normally", time.Minute, 15 * time.Minute, 16 * time.Minute},
		{"zero addend", time.Hour, 0, time.Hour},
		{"sum at the maximum", maxDuration - time.Second, time.Second, maxDuration},
		{"overflow is capped", maxDuration - time.Second, time.Hour, maxDuration},
		{"both at the maximum", maxDuration, maxDuration, maxDuration},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			require.Equal(t, testCase.want, saturatingDurationAdd(testCase.a, testCase.b))
		})
	}
}
