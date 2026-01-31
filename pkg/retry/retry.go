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

// MaxElapsedTimeError is returned when the retry timeout is exceeded.
type MaxElapsedTimeError struct {
	MaxElapsedTime time.Duration
}

func (e MaxElapsedTimeError) Error() string {
	return fmt.Sprintf("retry timeout exceeded after %v", e.MaxElapsedTime)
}

// UnexpectedError is returned when the retry loop ends unexpectedly.
var UnexpectedError = errors.New("unexpected end of retry loop")

// Validation errors.
var (
	ErrMaxAttemptsMustBePositive    = errors.New("max_attempts must be greater than zero")
	ErrMaxElapsedTimeMustBePositive = errors.New("max_elapsed_time must be greater than zero")
	ErrMultiplierMustBePositive     = errors.New("multiplier must be greater than zero")
	ErrMaxDelayMustBePositive       = errors.New("max_delay must be greater than zero")
	ErrInitialDelayCannotBeNegative = errors.New("initial_delay cannot be negative")
	ErrRandomJitterOutOfRange       = errors.New("random_jitter must be between 0.0 and 1.0")
)

// Validate checks that explicitly set values are valid.
// Returns an error if any field is explicitly set to an invalid value (e.g., zero or negative).
// Nil values are valid and mean "disabled" or "unlimited".
func Validate(config *schema.RetryConfig) error {
	if config == nil {
		return nil
	}
	if err := validatePositiveInt(config.MaxAttempts, ErrMaxAttemptsMustBePositive); err != nil {
		return err
	}
	if err := validatePositiveDuration(config.MaxElapsedTime, ErrMaxElapsedTimeMustBePositive); err != nil {
		return err
	}
	if err := validatePositiveFloat(config.Multiplier, ErrMultiplierMustBePositive); err != nil {
		return err
	}
	if err := validatePositiveDuration(config.MaxDelay, ErrMaxDelayMustBePositive); err != nil {
		return err
	}
	if err := validateNonNegativeDuration(config.InitialDelay, ErrInitialDelayCannotBeNegative); err != nil {
		return err
	}
	return validateJitter(config.RandomJitter)
}

func validatePositiveInt(val *int, err error) error {
	if val != nil && *val <= 0 {
		return err
	}
	return nil
}

func validatePositiveDuration(val *time.Duration, err error) error {
	if val != nil && *val <= 0 {
		return err
	}
	return nil
}

func validatePositiveFloat(val *float64, err error) error {
	if val != nil && *val <= 0 {
		return err
	}
	return nil
}

func validateNonNegativeDuration(val *time.Duration, err error) error {
	if val != nil && *val < 0 {
		return err
	}
	return nil
}

func validateJitter(val *float64) error {
	if val != nil && (*val < 0 || *val > 1) {
		return ErrRandomJitterOutOfRange
	}
	return nil
}

// ExecuteWithPredicate runs the function with retry logic, using the predicate to determine
// if an error should trigger a retry.
func (e *Executor) ExecuteWithPredicate(ctx context.Context, fn Func, shouldRetry func(error) bool) error {
	startTime := time.Now()

	// Determine effective max attempts (nil = unlimited, use MaxInt)
	maxAttempts := math.MaxInt
	if e.config.MaxAttempts != nil {
		maxAttempts = *e.config.MaxAttempts
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Check if we've exceeded max elapsed time (only if MaxElapsedTime is set)
		if e.config.MaxElapsedTime != nil && time.Since(startTime) > *e.config.MaxElapsedTime {
			return MaxElapsedTimeError{MaxElapsedTime: *e.config.MaxElapsedTime}
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
		if attempt == maxAttempts {
			return fmt.Errorf("max attempts (%d) exceeded, last error: %w", maxAttempts, err)
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
	// Get initial delay (nil = 0, no delay)
	var initialDelay time.Duration
	if e.config.InitialDelay != nil {
		initialDelay = *e.config.InitialDelay
	}

	var delay time.Duration

	switch e.config.BackoffStrategy {
	case schema.BackoffConstant:
		delay = initialDelay
	case schema.BackoffLinear:
		delay = time.Duration(float64(initialDelay) * float64(attempt))
	case schema.BackoffExponential:
		// Get multiplier (nil = default 2.0)
		multiplier := 2.0
		if e.config.Multiplier != nil {
			multiplier = *e.config.Multiplier
		}
		delay = time.Duration(float64(initialDelay) * math.Pow(multiplier, float64(attempt-1)))
	default:
		delay = initialDelay
	}

	// Apply max delay limit (only if MaxDelay is set)
	if e.config.MaxDelay != nil && delay > *e.config.MaxDelay {
		delay = *e.config.MaxDelay
	}

	// Apply random jitter if configured
	var jitterFactor float64
	if e.config.RandomJitter != nil {
		jitterFactor = *e.config.RandomJitter
	}
	if jitterFactor > 0 {
		jitter := time.Duration(e.rand.Float64() * float64(delay) * jitterFactor)
		if e.rand.Float64() < jitterFlipChance {
			delay += jitter
		} else {
			delay -= jitter
		}

		// Ensure delay is not negative
		if delay < 0 {
			delay = 0
		}
	}

	return delay
}

// Do is a convenience function that creates an executor and runs the function.
// If config is nil, a default single-attempt config is used to ensure consistent
// error messages (e.g., "max attempts (1) exceeded, last error: ...").
func Do(ctx context.Context, config *schema.RetryConfig, fn Func) error {
	effectiveConfig := config
	if effectiveConfig == nil {
		// No explicit retry config = single attempt with consistent error messages.
		effectiveConfig = &schema.RetryConfig{
			MaxAttempts: intPtr(1),
		}
	}
	if err := Validate(effectiveConfig); err != nil {
		return err
	}
	executor := New(*effectiveConfig)
	return executor.Execute(ctx, fn)
}

// With7Params is a convenience function for retrying functions with 7 parameters.
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
// If config is nil, the function is executed once without retry.
func WithPredicate(ctx context.Context, config *schema.RetryConfig, fn Func, shouldRetry func(error) bool) error {
	if config == nil {
		// No retry config = run once without retry
		return fn()
	}
	if err := Validate(config); err != nil {
		return err
	}
	executor := New(*config)
	return executor.ExecuteWithPredicate(ctx, fn, shouldRetry)
}

// Helper functions for creating pointer values (useful in tests and config).
func intPtr(i int) *int                          { return &i }
func durationPtr(d time.Duration) *time.Duration { return &d }
func float64Ptr(f float64) *float64              { return &f }

// DefaultConfig returns a sensible default configuration for a single attempt.
// Most fields are nil (disabled/unlimited).
func DefaultConfig() schema.RetryConfig {
	return schema.RetryConfig{
		MaxAttempts:     intPtr(1), // Run once by default
		BackoffStrategy: schema.BackoffExponential,
		// All other fields nil = disabled/unlimited
	}
}
