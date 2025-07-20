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
		MaxAttempts:     3,
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		RandomJitter:    false,
		Multiplier:      2.0,
		MaxElapsedTime:  1 * time.Second,
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
		MaxAttempts:     3,
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		RandomJitter:    false,
		Multiplier:      2.0,
		MaxElapsedTime:  1 * time.Second,
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
		MaxAttempts:     5,
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    50 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		RandomJitter:    false,
		Multiplier:      2.0,
		MaxElapsedTime:  1 * time.Second,
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
		MaxAttempts:     10,
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		RandomJitter:    false,
		Multiplier:      2.0,
		MaxElapsedTime:  20 * time.Millisecond,
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
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		RandomJitter:    false,
		Multiplier:      2.0,
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
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		RandomJitter:    false,
		Multiplier:      2.0,
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
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		RandomJitter:    false,
		Multiplier:      2.0,
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
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        300 * time.Millisecond,
		RandomJitter:    false,
		Multiplier:      2.0,
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
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		RandomJitter:    true,
		Multiplier:      2.0,
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
		MaxAttempts:     3,
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		RandomJitter:    false,
		Multiplier:      2.0,
		MaxElapsedTime:  1 * time.Second,
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
	config := schema.RetryConfig{
		MaxAttempts:     3,
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		RandomJitter:    false,
		Multiplier:      2.0,
		MaxElapsedTime:  1 * time.Second,
	}

	retryableError := errors.New("retryable error")
	nonRetryableError := errors.New("non-retryable error")

	shouldRetry := func(err error) bool {
		return err.Error() == "retryable error"
	}

	// Test retryable error
	attempts := 0
	fn := func() error {
		attempts++
		if attempts < 2 {
			return retryableError
		}
		return nil
	}

	err := WithPredicate(context.Background(), &config, fn, shouldRetry)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}

	// Test non-retryable error
	attempts = 0
	fn = func() error {
		attempts++
		return nonRetryableError
	}
	err = WithPredicate(context.Background(), &config, fn, shouldRetry)

	if err == nil {
		t.Error("Expected error, got nil")
	}

	if attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", attempts)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxAttempts != 1 {
		t.Errorf("Expected MaxAttempts to be 1, got %d", config.MaxAttempts)
	}

	if config.BackoffStrategy != schema.BackoffExponential {
		t.Errorf("Expected BackoffStrategy to be %q, got %q", schema.BackoffExponential, config.BackoffStrategy)
	}

	if config.InitialDelay != 100*time.Millisecond {
		t.Errorf("Expected InitialDelay to be 100ms, got %v", config.InitialDelay)
	}

	if config.MaxDelay != 5*time.Second {
		t.Errorf("Expected MaxDelay to be 5s, got %v", config.MaxDelay)
	}

	if !config.RandomJitter {
		t.Error("Expected RandomJitter to be true")
	}

	if config.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier to be 2.0, got %f", config.Multiplier)
	}

	if config.MaxElapsedTime != 30*time.Minute {
		t.Errorf("Expected MaxElapsedTime to be 30s, got %v", config.MaxElapsedTime)
	}
}

func TestRetryPredicates(t *testing.T) {
	if !RetryOnAnyError(errors.New("any error")) {
		t.Error("RetryOnAnyError should return true for any error")
	}

	if !RetryOnNetworkError(errors.New("network error")) {
		t.Error("RetryOnNetworkError should return true for network errors")
	}
}

func BenchmarkExecutor_Execute_Success(b *testing.B) {
	config := schema.RetryConfig{
		MaxAttempts:     3,
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    1 * time.Microsecond,
		MaxDelay:        100 * time.Microsecond,
		RandomJitter:    false,
		Multiplier:      2.0,
		MaxElapsedTime:  1 * time.Second,
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
