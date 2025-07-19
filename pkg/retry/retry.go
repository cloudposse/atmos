package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Func represents a function that can be retried.
type Func func() error

// Executor handles the retry logic.
type Executor struct {
	config schema.RetryConfig
	rand   *rand.Rand
}

// New creates a new retry executor with the given config.
func New(config schema.RetryConfig) *Executor {
	return &Executor{
		config: config,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Execute runs the function with retry logic.
func (e *Executor) Execute(ctx context.Context, fn Func) error {
	return e.ExecuteWithPredicate(ctx, fn, func(err error) bool {
		return true
	})
}

type MaxElapsedTimeError struct {
	MaxElapsedTime time.Duration
}

func (e MaxElapsedTimeError) Error() string {
	return fmt.Sprintf("retry timeout exceeded after %v", e.MaxElapsedTime)
}

var UnexpectedError = errors.New("unexpected end of retry loop")

func (e *Executor) ExecuteWithPredicate(ctx context.Context, fn Func, shouldRetry func(error) bool) error {
	startTime := time.Now()

	for attempt := 1; attempt <= e.config.MaxAttempts; attempt++ {
		// Check if we've exceeded max elapsed time
		if time.Since(startTime) > e.config.MaxElapsedTime {
			return MaxElapsedTimeError{MaxElapsedTime: e.config.MaxElapsedTime}
		}

		// Execute the function
		err := fn()
		if err == nil {
			return nil // Success!
		}

		if !shouldRetry(err) {
			return err
		}

		// If this was the last attempt, return the error
		if attempt == e.config.MaxAttempts {
			return fmt.Errorf("max attempts (%d) exceeded, last error: %w", e.config.MaxAttempts, err)
		}

		// Calculate delay for next attempt
		delay := e.calculateDelay(attempt)

		// Wait for the calculated delay or until context is cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}
	}
	return UnexpectedError
}

const jitterFlipChance = 0.5

// calculateDelay calculates the delay for the next retry attempt.
func (e *Executor) calculateDelay(attempt int) time.Duration {
	var delay time.Duration

	switch e.config.BackoffStrategy {
	case schema.BackoffConstant:
		delay = e.config.InitialDelay
	case schema.BackoffLinear:
		delay = time.Duration(float64(e.config.InitialDelay) * float64(attempt))
	case schema.BackoffExponential:
		delay = time.Duration(float64(e.config.InitialDelay) * math.Pow(e.config.Multiplier, float64(attempt-1)))
	default:
		delay = e.config.InitialDelay
	}

	// Apply max delay limit
	if delay > e.config.MaxDelay {
		delay = e.config.MaxDelay
	}

	// Apply random jitter if enabled
	if e.config.RandomJitter {
		jitter := time.Duration(e.rand.Float64() * float64(delay) * 0.1) // 10% jitter
		if e.rand.Float64() < jitterFlipChance {
			delay += jitter
		} else {
			delay -= jitter
		}

		// Ensure delay is not negative
		if delay < 0 {
			delay = time.Duration(0)
		}
	}

	return delay
}

// Do is a convenience function that creates an executor and runs the function.
func Do(ctx context.Context, config *schema.RetryConfig, fn Func) error {
	if config == nil {
		temp := DefaultConfig()
		config = &temp
	}
	executor := New(*config)
	return executor.Execute(ctx, fn)
}

func With7Params[T1 any, T2 any, T3 any, T4 any, T5 any, T6 any, T7 any](ctx context.Context,
	config *schema.RetryConfig,
	fn func(T1, T2, T3, T4, T5, T6, T7) error, a T1, b T2, c T3, d T4, e T5, f T6, g T7,
) error {
	err := Do(ctx, config, func() error {
		return fn(a, b, c, d, e, f, g)
	})
	return err
}

// WithPredicate allows you to specify which errors should trigger a retry.
func WithPredicate(ctx context.Context, config *schema.RetryConfig, fn Func, shouldRetry func(error) bool) error {
	if config == nil {
		temp := DefaultConfig()
		config = &temp
	}
	executor := New(*config)
	return executor.ExecuteWithPredicate(ctx, fn, shouldRetry)
}

const (
	defaultInitialDelay   = 100 * time.Millisecond
	defaultMaxDelay       = 5 * time.Second
	defaultMaxElapsedTime = 30 * time.Minute
)

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() schema.RetryConfig {
	return schema.RetryConfig{
		MaxAttempts:     1,
		BackoffStrategy: schema.BackoffExponential,
		InitialDelay:    defaultInitialDelay,
		MaxDelay:        defaultMaxDelay,
		RandomJitter:    true,
		Multiplier:      2.0,
		MaxElapsedTime:  defaultMaxElapsedTime,
	}
}

// Predefined common retry predicates.
var (
	// RetryOnAnyError retries on any error.
	RetryOnAnyError = func(err error) bool { return true }

	// RetryOnNetworkError retries on network-related errors.
	RetryOnNetworkError = func(err error) bool {
		// You can customize this based on your specific network error types
		return true // Placeholder - customize based on your error types
	}
)
