package retry

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

type cooldownError struct{ delay time.Duration }

func (e *cooldownError) Error() string             { return "throttled" }
func (e *cooldownError) RetryAfter() time.Duration { return e.delay }

type cooldownClock struct {
	elapsed time.Duration
	waits   []time.Duration
}

func (c *cooldownClock) Now() time.Time { return time.Unix(0, 0).Add(c.elapsed) }
func (c *cooldownClock) After(d time.Duration) <-chan time.Time {
	c.elapsed += d
	c.waits = append(c.waits, d)
	ch := make(chan time.Time, 1)
	ch <- c.Now()
	return ch
}

func TestRetryAfterSurvivesWrappingCapAndJitter(t *testing.T) {
	clock := &cooldownClock{}
	config := schema.RetryConfig{MaxAttempts: new(2), InitialDelay: new(time.Second), MaxDelay: new(time.Second), RandomJitter: new(1.0)}
	attempts := 0
	err := New(config, WithClock(clock)).Execute(t.Context(), func() error {
		attempts++
		if attempts == 1 {
			return fmt.Errorf("transport: %w", &cooldownError{delay: time.Minute})
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.Equal(t, []time.Duration{time.Minute}, clock.waits)
}

func TestRetryAfterRespectsRemainingBudgetAndAttempts(t *testing.T) {
	for _, mode := range []string{"budget", "attempts"} {
		t.Run(mode, func(t *testing.T) {
			clock := &cooldownClock{}
			config := schema.RetryConfig{MaxAttempts: new(3), MaxElapsedTime: new(time.Minute)}
			if mode == "attempts" {
				config.MaxAttempts = new(1)
			}
			failure := &cooldownError{delay: 30 * time.Second}
			attempts := 0
			err := New(config, WithClock(clock)).Execute(t.Context(), func() error { attempts++; clock.elapsed += 30 * time.Second; return failure })
			require.ErrorIs(t, err, failure)
			var timeout MaxElapsedTimeError
			require.Equal(t, mode == "budget", errors.As(err, &timeout))
			require.Equal(t, 1, attempts)
			require.Empty(t, clock.waits)
		})
	}
}
