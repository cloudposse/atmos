package retry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecutor_Execute_Success(t *testing.T) {
	config := schema.RetryConfig{
		MaxAttempts:     intPtr(3),
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(10 * time.Millisecond),
		MaxDelay:        durationPtr(100 * time.Millisecond),
		RandomJitter:    float64Ptr(0.0),
		Multiplier:      float64Ptr(2.0),
		MaxElapsedTime:  durationPtr(1 * time.Second),
	}

	executor := New(config)
	attempts := 0

	fn := func() error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	}

	err := executor.Execute(context.Background(), fn)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestExecutor_Execute_MaxAttemptsExceeded(t *testing.T) {
	config := schema.RetryConfig{
		MaxAttempts:     intPtr(3),
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(1 * time.Millisecond),
		MaxDelay:        durationPtr(100 * time.Millisecond),
		RandomJitter:    float64Ptr(0.0),
		Multiplier:      float64Ptr(2.0),
		MaxElapsedTime:  durationPtr(1 * time.Second),
	}

	executor := New(config)
	attempts := 0
	expectedError := errors.New("persistent error")

	fn := func() error {
		attempts++
		return expectedError
	}

	err := executor.Execute(context.Background(), fn)

	if err == nil {
		t.Error("Expected error, got nil")
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}

	if !strings.Contains(err.Error(), "max attempts (3) exceeded") {
		t.Errorf("Expected max attempts error, got: %v", err)
	}
}

func TestExecutor_Execute_ContextCancelled(t *testing.T) {
	config := schema.RetryConfig{
		MaxAttempts:     intPtr(5),
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(50 * time.Millisecond),
		MaxDelay:        durationPtr(100 * time.Millisecond),
		RandomJitter:    float64Ptr(0.0),
		Multiplier:      float64Ptr(2.0),
		MaxElapsedTime:  durationPtr(1 * time.Second),
	}

	executor := New(config)
	ctx, cancel := context.WithCancel(context.Background())

	attempts := 0
	fn := func() error {
		attempts++
		if attempts == 2 {
			cancel()
		}
		return errors.New("error")
	}

	err := executor.Execute(ctx, fn)

	if err == nil {
		t.Error("Expected error, got nil")
	}

	if !strings.Contains(err.Error(), "context cancelled") {
		t.Errorf("Expected context cancelled error, got: %v", err)
	}
}

func TestExecutor_Execute_MaxElapsedTimeExceeded(t *testing.T) {
	config := schema.RetryConfig{
		MaxAttempts:     intPtr(10),
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(1 * time.Millisecond),
		MaxDelay:        durationPtr(100 * time.Millisecond),
		RandomJitter:    float64Ptr(0.0),
		Multiplier:      float64Ptr(2.0),
		MaxElapsedTime:  durationPtr(20 * time.Millisecond),
	}

	executor := New(config)
	attempts := 0

	fn := func() error {
		attempts++
		time.Sleep(15 * time.Millisecond)
		return errors.New("error")
	}

	err := executor.Execute(context.Background(), fn)

	if err == nil {
		t.Error("Expected error, got nil")
	}

	if !strings.Contains(err.Error(), "retry timeout exceeded") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

func TestExecutor_CalculateDelay_Constant(t *testing.T) {
	config := schema.RetryConfig{
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(100 * time.Millisecond),
		MaxDelay:        durationPtr(1 * time.Second),
		RandomJitter:    float64Ptr(0.0),
		Multiplier:      float64Ptr(2.0),
	}

	executor := New(config)

	for attempt := 1; attempt <= 5; attempt++ {
		delay := executor.calculateDelay(attempt)
		expected := 100 * time.Millisecond

		if delay != expected {
			t.Errorf("Attempt %d: expected %v, got %v", attempt, expected, delay)
		}
	}
}

func TestExecutor_CalculateDelay_Linear(t *testing.T) {
	config := schema.RetryConfig{
		BackoffStrategy: schema.BackoffLinear,
		InitialDelay:    durationPtr(100 * time.Millisecond),
		MaxDelay:        durationPtr(1 * time.Second),
		RandomJitter:    float64Ptr(0.0),
		Multiplier:      float64Ptr(2.0),
	}

	executor := New(config)

	expectedDelays := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
		400 * time.Millisecond,
	}

	for i, expected := range expectedDelays {
		attempt := i + 1
		delay := executor.calculateDelay(attempt)

		if delay != expected {
			t.Errorf("Attempt %d: expected %v, got %v", attempt, expected, delay)
		}
	}
}

func TestExecutor_CalculateDelay_Exponential(t *testing.T) {
	config := schema.RetryConfig{
		BackoffStrategy: schema.BackoffExponential,
		InitialDelay:    durationPtr(100 * time.Millisecond),
		MaxDelay:        durationPtr(1 * time.Second),
		RandomJitter:    float64Ptr(0.0),
		Multiplier:      float64Ptr(2.0),
	}

	executor := New(config)

	expectedDelays := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
	}

	for i, expected := range expectedDelays {
		attempt := i + 1
		delay := executor.calculateDelay(attempt)

		if delay != expected {
			t.Errorf("Attempt %d: expected %v, got %v", attempt, expected, delay)
		}
	}
}

func TestExecutor_CalculateDelay_MaxDelayLimit(t *testing.T) {
	config := schema.RetryConfig{
		BackoffStrategy: schema.BackoffExponential,
		InitialDelay:    durationPtr(100 * time.Millisecond),
		MaxDelay:        durationPtr(300 * time.Millisecond),
		RandomJitter:    float64Ptr(0.0),
		Multiplier:      float64Ptr(2.0),
	}

	executor := New(config)

	delay := executor.calculateDelay(3)
	expected := 300 * time.Millisecond

	if delay != expected {
		t.Errorf("Expected delay to be capped at %v, got %v", expected, delay)
	}
}

func TestExecutor_CalculateDelay_WithJitter(t *testing.T) {
	config := schema.RetryConfig{
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(100 * time.Millisecond),
		MaxDelay:        durationPtr(1 * time.Second),
		RandomJitter:    float64Ptr(0.1),
		Multiplier:      float64Ptr(2.0),
	}

	executor := New(config)

	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = executor.calculateDelay(1)
	}

	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}

	if allSame {
		t.Error("Expected jitter to produce different delays, but all were the same")
	}
}

func TestDo_ConvenienceFunction(t *testing.T) {
	config := schema.RetryConfig{
		MaxAttempts:     intPtr(3),
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(1 * time.Millisecond),
		MaxDelay:        durationPtr(100 * time.Millisecond),
		RandomJitter:    float64Ptr(0.0),
		Multiplier:      float64Ptr(2.0),
		MaxElapsedTime:  durationPtr(1 * time.Second),
	}

	attempts := 0
	fn := func() error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	}

	err := Do(context.Background(), &config, fn)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestWithPredicate_RetryOnSpecificErrors(t *testing.T) {
	config := &schema.RetryConfig{
		MaxAttempts:     intPtr(3),
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(1 * time.Millisecond),
		MaxDelay:        durationPtr(100 * time.Millisecond),
		RandomJitter:    float64Ptr(0.0),
		Multiplier:      float64Ptr(2.0),
		MaxElapsedTime:  durationPtr(1 * time.Second),
	}

	retryableError := errors.New("retryable error")
	nonRetryableError := errors.New("non-retryable error")

	shouldRetry := func(err error) bool {
		return err.Error() == "retryable error"
	}

	tests := []struct {
		name         string
		fn           func(*int) func() error
		wantErr      bool
		wantAttempts int
	}{
		{
			name: "retryable error succeeds on second attempt",
			fn: func(attempts *int) func() error {
				return func() error {
					*attempts++
					if *attempts < 2 {
						return retryableError
					}
					return nil
				}
			},
			wantErr:      false,
			wantAttempts: 2,
		},
		{
			name: "non-retryable error stops immediately",
			fn: func(attempts *int) func() error {
				return func() error {
					*attempts++
					return nonRetryableError
				}
			},
			wantErr:      true,
			wantAttempts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			err := WithPredicate(context.Background(), config, tt.fn(&attempts), shouldRetry)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
			if attempts != tt.wantAttempts {
				t.Errorf("Expected %d attempts, got %d", tt.wantAttempts, attempts)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxAttempts == nil || *config.MaxAttempts != 1 {
		t.Errorf("Expected MaxAttempts to be 1, got %v", config.MaxAttempts)
	}

	if config.BackoffStrategy != schema.BackoffExponential {
		t.Errorf("Expected BackoffStrategy to be %q, got %q", schema.BackoffExponential, config.BackoffStrategy)
	}

	// Other fields should be nil (not set).
	if config.InitialDelay != nil {
		t.Errorf("Expected InitialDelay to be nil, got %v", config.InitialDelay)
	}

	if config.MaxDelay != nil {
		t.Errorf("Expected MaxDelay to be nil, got %v", config.MaxDelay)
	}

	if config.RandomJitter != nil {
		t.Errorf("Expected RandomJitter to be nil, got %v", config.RandomJitter)
	}

	if config.Multiplier != nil {
		t.Errorf("Expected Multiplier to be nil, got %v", config.Multiplier)
	}

	if config.MaxElapsedTime != nil {
		t.Errorf("Expected MaxElapsedTime to be nil, got %v", config.MaxElapsedTime)
	}
}

// =============================================================================
// Validation Tests
// =============================================================================

func TestValidate_NilConfig(t *testing.T) {
	err := Validate(nil)
	if err != nil {
		t.Errorf("Expected nil error for nil config, got: %v", err)
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	config := &schema.RetryConfig{
		MaxAttempts:    intPtr(3),
		MaxElapsedTime: durationPtr(30 * time.Second),
		Multiplier:     float64Ptr(2.0),
		MaxDelay:       durationPtr(10 * time.Second),
		InitialDelay:   durationPtr(1 * time.Second),
		RandomJitter:   float64Ptr(0.5),
	}
	err := Validate(config)
	if err != nil {
		t.Errorf("Expected nil error for valid config, got: %v", err)
	}
}

func TestValidate_ExplicitZeroErrors(t *testing.T) {
	tests := []struct {
		name   string
		config *schema.RetryConfig
		errMsg string
	}{
		{
			name:   "zero max_attempts",
			config: &schema.RetryConfig{MaxAttempts: intPtr(0)},
			errMsg: "max_attempts must be greater than zero",
		},
		{
			name:   "negative max_attempts",
			config: &schema.RetryConfig{MaxAttempts: intPtr(-1)},
			errMsg: "max_attempts must be greater than zero",
		},
		{
			name:   "zero max_elapsed_time",
			config: &schema.RetryConfig{MaxElapsedTime: durationPtr(0)},
			errMsg: "max_elapsed_time must be greater than zero",
		},
		{
			name:   "negative max_elapsed_time",
			config: &schema.RetryConfig{MaxElapsedTime: durationPtr(-1 * time.Second)},
			errMsg: "max_elapsed_time must be greater than zero",
		},
		{
			name:   "zero multiplier",
			config: &schema.RetryConfig{Multiplier: float64Ptr(0)},
			errMsg: "multiplier must be greater than zero",
		},
		{
			name:   "negative multiplier",
			config: &schema.RetryConfig{Multiplier: float64Ptr(-1.0)},
			errMsg: "multiplier must be greater than zero",
		},
		{
			name:   "zero max_delay",
			config: &schema.RetryConfig{MaxDelay: durationPtr(0)},
			errMsg: "max_delay must be greater than zero",
		},
		{
			name:   "negative max_delay",
			config: &schema.RetryConfig{MaxDelay: durationPtr(-1 * time.Second)},
			errMsg: "max_delay must be greater than zero",
		},
		{
			name:   "negative initial_delay",
			config: &schema.RetryConfig{InitialDelay: durationPtr(-1 * time.Second)},
			errMsg: "initial_delay cannot be negative",
		},
		{
			name:   "random_jitter below 0",
			config: &schema.RetryConfig{RandomJitter: float64Ptr(-0.1)},
			errMsg: "random_jitter must be between 0.0 and 1.0",
		},
		{
			name:   "random_jitter above 1",
			config: &schema.RetryConfig{RandomJitter: float64Ptr(1.5)},
			errMsg: "random_jitter must be between 0.0 and 1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.config)
			if err == nil {
				t.Errorf("Expected error containing %q, got nil", tt.errMsg)
				return
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Expected error containing %q, got: %v", tt.errMsg, err)
			}
		})
	}
}

// =============================================================================
// Nil (Unset) Field Tests - verifies "omit = unlimited/disabled" behavior
// =============================================================================

func TestExecutor_NilMaxElapsedTime_NoTimeout(t *testing.T) {
	// nil MaxElapsedTime = no time limit.
	config := schema.RetryConfig{
		MaxAttempts:  intPtr(3),
		InitialDelay: durationPtr(10 * time.Millisecond),
		// MaxElapsedTime: nil = no timeout.
	}

	executor := New(config)
	attempts := 0

	fn := func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	}

	err := executor.Execute(context.Background(), fn)
	if err != nil {
		t.Errorf("Expected success with nil MaxElapsedTime, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestExecutor_NilMaxAttempts_UnlimitedRetries(t *testing.T) {
	// Nil MaxAttempts = unlimited retries (test with context timeout).
	// Use generous timeout and low threshold to avoid flakiness on slow CI runners.
	config := schema.RetryConfig{
		InitialDelay: durationPtr(1 * time.Millisecond),
	}

	executor := New(config)
	attempts := 0

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	fn := func() error {
		attempts++
		return errors.New("keep failing")
	}

	_ = executor.Execute(ctx, fn)

	// Should have made at least 2 attempts before context timeout.
	if attempts < 2 {
		t.Errorf("Expected multiple attempts with nil MaxAttempts, got %d", attempts)
	}
}

func TestExecutor_NilInitialDelay_NoDelay(t *testing.T) {
	// nil InitialDelay = no delay between retries.
	config := schema.RetryConfig{
		MaxAttempts: intPtr(3),
		// InitialDelay: nil = no delay.
	}

	executor := New(config)
	delay := executor.calculateDelay(1)

	if delay != 0 {
		t.Errorf("Expected 0 delay with nil InitialDelay, got %v", delay)
	}
}

func TestExecutor_NilMaxDelay_NoCap(t *testing.T) {
	// nil MaxDelay = no delay cap.
	config := schema.RetryConfig{
		BackoffStrategy: schema.BackoffExponential,
		InitialDelay:    durationPtr(100 * time.Millisecond),
		Multiplier:      float64Ptr(2.0),
		// MaxDelay: nil = no cap.
	}

	executor := New(config)

	// Attempt 5: 100ms * 2^4 = 1600ms (should not be capped).
	delay := executor.calculateDelay(5)
	expected := 1600 * time.Millisecond

	if delay != expected {
		t.Errorf("Expected delay %v with nil MaxDelay (no cap), got %v", expected, delay)
	}
}

func TestExecutor_NilMultiplier_UsesDefault(t *testing.T) {
	// nil Multiplier = use default (2.0).
	config := schema.RetryConfig{
		BackoffStrategy: schema.BackoffExponential,
		InitialDelay:    durationPtr(100 * time.Millisecond),
		// Multiplier: nil = use default 2.0.
	}

	executor := New(config)

	// Attempt 2 should be 200ms (100ms * 2^1) with default multiplier.
	delay := executor.calculateDelay(2)
	expected := 200 * time.Millisecond

	if delay != expected {
		t.Errorf("Expected delay %v with default multiplier, got %v", expected, delay)
	}
}

func TestExecutor_NilRandomJitter_NoJitter(t *testing.T) {
	// nil RandomJitter = no jitter.
	config := schema.RetryConfig{
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(100 * time.Millisecond),
		// RandomJitter: nil = no jitter.
	}

	executor := New(config)

	// All delays should be exactly the same.
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = executor.calculateDelay(1)
	}

	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			t.Errorf("Expected all delays to be equal with nil RandomJitter, got varying delays")
			break
		}
	}
}

// =============================================================================
// Do and WithPredicate with nil config
// =============================================================================

func TestDo_NilConfig_RunsOnce(t *testing.T) {
	// nil config = run once without retry.
	attempts := 0
	fn := func() error {
		attempts++
		return errors.New("error")
	}

	err := Do(context.Background(), nil, fn)

	if err == nil {
		t.Error("Expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt with nil config, got %d", attempts)
	}
}

func TestWithPredicate_NilConfig_RunsOnce(t *testing.T) {
	// nil config = run once without retry.
	attempts := 0
	fn := func() error {
		attempts++
		return errors.New("error")
	}

	err := WithPredicate(context.Background(), nil, fn, func(error) bool { return true })

	if err == nil {
		t.Error("Expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt with nil config, got %d", attempts)
	}
}

// =============================================================================
// Validation error paths in Do and WithPredicate
// =============================================================================

func TestDo_InvalidConfig_ReturnsValidationError(t *testing.T) {
	// Config with zero max_attempts should fail validation.
	config := &schema.RetryConfig{
		MaxAttempts: intPtr(0),
	}

	err := Do(context.Background(), config, func() error {
		t.Error("Function should not be called with invalid config")
		return nil
	})

	if err == nil {
		t.Error("Expected validation error, got nil")
	}
	if !errors.Is(err, ErrMaxAttemptsMustBePositive) {
		t.Errorf("Expected ErrMaxAttemptsMustBePositive, got: %v", err)
	}
}

func TestWithPredicate_InvalidConfig_ReturnsValidationError(t *testing.T) {
	// Config with negative multiplier should fail validation.
	config := &schema.RetryConfig{
		Multiplier: float64Ptr(-1.0),
	}

	err := WithPredicate(context.Background(), config, func() error {
		t.Error("Function should not be called with invalid config")
		return nil
	}, func(error) bool { return true })

	if err == nil {
		t.Error("Expected validation error, got nil")
	}
	if !errors.Is(err, ErrMultiplierMustBePositive) {
		t.Errorf("Expected ErrMultiplierMustBePositive, got: %v", err)
	}
}

// =============================================================================
// Coverage gap tests - default strategy, jitter subtraction, negative clamping
// =============================================================================

func TestExecutor_CalculateDelay_UnknownStrategy(t *testing.T) {
	// Unknown backoff strategy should fall back to initial delay.
	config := schema.RetryConfig{
		BackoffStrategy: schema.BackoffStrategy("unknown_strategy"),
		InitialDelay:    durationPtr(100 * time.Millisecond),
		RandomJitter:    float64Ptr(0.0),
	}

	executor := New(config)
	delay := executor.calculateDelay(3)

	if delay != 100*time.Millisecond {
		t.Errorf("Expected initial delay for unknown strategy, got %v", delay)
	}
}

func TestExecutor_CalculateDelay_JitterSubtraction(t *testing.T) {
	// Use a seeded rand to force the subtraction branch (rand >= 0.5).
	config := schema.RetryConfig{
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(100 * time.Millisecond),
		RandomJitter:    float64Ptr(0.5),
	}

	executor := New(config)

	// Try many attempts to cover both addition and subtraction branches.
	var sawBelow, sawAbove bool
	for i := 0; i < 100; i++ {
		delay := executor.calculateDelay(1)
		if delay < 100*time.Millisecond {
			sawBelow = true
		}
		if delay > 100*time.Millisecond {
			sawAbove = true
		}
		if sawBelow && sawAbove {
			break
		}
	}

	if !sawBelow {
		t.Error("Expected jitter to produce delays below initial delay (subtraction branch)")
	}
	if !sawAbove {
		t.Error("Expected jitter to produce delays above initial delay (addition branch)")
	}
}

func TestExecutor_CalculateDelay_NegativeDelayClamped(t *testing.T) {
	// Use jitter factor > 1.0 (bypassing validation) to force negative delay clamping.
	// With jitterFactor > 1.0 and the subtraction branch, delay can go negative.
	config := schema.RetryConfig{
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(1 * time.Millisecond),
		RandomJitter:    float64Ptr(5.0), // Intentionally high to force negative.
	}

	executor := New(config)

	// Run many times; at least some should hit the clamping path.
	for i := 0; i < 100; i++ {
		delay := executor.calculateDelay(1)
		if delay < 0 {
			t.Errorf("Delay should never be negative, got %v", delay)
		}
	}
}

// =============================================================================
// With7Params tests
// =============================================================================

func TestWith7Params_Success(t *testing.T) {
	config := &schema.RetryConfig{
		MaxAttempts:     intPtr(3),
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(1 * time.Millisecond),
	}

	attempts := 0
	fn := func(a string, b int, c bool, d float64, e string, f int, g string) error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		// Verify all params passed through correctly.
		if a != "hello" || b != 42 || !c || d != 3.14 || e != "world" || f != 7 || g != "!" {
			return errors.New("params not passed correctly")
		}
		return nil
	}

	err := With7Params(context.Background(), config, fn, "hello", 42, true, 3.14, "world", 7, "!")
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestWith7Params_NilConfig_RunsOnce(t *testing.T) {
	attempts := 0
	fn := func(a, b, c, d, e, f, g string) error {
		attempts++
		return errors.New("always fails")
	}

	err := With7Params(context.Background(), nil, fn, "a", "b", "c", "d", "e", "f", "g")
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt with nil config, got %d", attempts)
	}
}

func TestWith7Params_InvalidConfig_ReturnsValidationError(t *testing.T) {
	config := &schema.RetryConfig{
		MaxAttempts: intPtr(0), // Invalid: zero.
	}

	fn := func(a, b, c, d, e, f, g int) error {
		t.Error("Function should not be called with invalid config")
		return nil
	}

	err := With7Params(context.Background(), config, fn, 1, 2, 3, 4, 5, 6, 7)
	if err == nil {
		t.Error("Expected validation error, got nil")
	}
	if !errors.Is(err, ErrMaxAttemptsMustBePositive) {
		t.Errorf("Expected ErrMaxAttemptsMustBePositive, got: %v", err)
	}
}

func TestWith7Params_MaxAttemptsExceeded(t *testing.T) {
	config := &schema.RetryConfig{
		MaxAttempts:     intPtr(2),
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(1 * time.Millisecond),
	}

	attempts := 0
	fn := func(a, b, c, d, e, f, g int) error {
		attempts++
		return errors.New("persistent failure")
	}

	err := With7Params(context.Background(), config, fn, 1, 2, 3, 4, 5, 6, 7)
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
	if !strings.Contains(err.Error(), "max attempts (2) exceeded") {
		t.Errorf("Expected max attempts error, got: %v", err)
	}
}

// =============================================================================
// WithPredicate success path test
// =============================================================================

func TestWithPredicate_NilConfig_Success(t *testing.T) {
	// nil config with successful function should return nil.
	fn := func() error {
		return nil
	}

	err := WithPredicate(context.Background(), nil, fn, func(error) bool { return true })
	if err != nil {
		t.Errorf("Expected nil error for successful function with nil config, got: %v", err)
	}
}

// =============================================================================
// Edge case: zero initial delay validation
// =============================================================================

func TestValidate_ZeroInitialDelay_IsValid(t *testing.T) {
	// Zero initial delay is valid (means "no delay"), unlike zero max_attempts.
	config := &schema.RetryConfig{
		InitialDelay: durationPtr(0),
	}
	err := Validate(config)
	if err != nil {
		t.Errorf("Expected zero initial_delay to be valid, got: %v", err)
	}
}

func TestValidate_AllNilFields_IsValid(t *testing.T) {
	// All-nil config is valid (everything disabled/unlimited).
	config := &schema.RetryConfig{}
	err := Validate(config)
	if err != nil {
		t.Errorf("Expected all-nil config to be valid, got: %v", err)
	}
}

func TestValidate_BoundaryJitter(t *testing.T) {
	// Jitter at exact boundaries (0.0 and 1.0) should be valid.
	tests := []struct {
		name   string
		jitter float64
	}{
		{"zero jitter", 0.0},
		{"max jitter", 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &schema.RetryConfig{RandomJitter: float64Ptr(tt.jitter)}
			err := Validate(config)
			if err != nil {
				t.Errorf("Expected jitter %v to be valid, got: %v", tt.jitter, err)
			}
		})
	}
}

// =============================================================================
// Do convenience function edge cases
// =============================================================================

func TestDo_NilConfig_Success(t *testing.T) {
	// nil config with successful function.
	err := Do(context.Background(), nil, func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected nil for successful function with nil config, got: %v", err)
	}
}

// =============================================================================
// Benchmark
// =============================================================================

func BenchmarkExecutor_Execute_Success(b *testing.B) {
	config := schema.RetryConfig{
		MaxAttempts:     intPtr(3),
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    durationPtr(1 * time.Microsecond),
		MaxDelay:        durationPtr(100 * time.Microsecond),
		RandomJitter:    float64Ptr(0.0),
		Multiplier:      float64Ptr(2.0),
		MaxElapsedTime:  durationPtr(1 * time.Second),
	}

	executor := New(config)
	fn := func() error {
		return nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = executor.Execute(context.Background(), fn)
	}
}
