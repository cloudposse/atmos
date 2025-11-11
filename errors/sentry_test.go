package errors

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestInitializeSentry_Disabled(t *testing.T) {
	config := &schema.SentryConfig{
		Enabled: false,
	}

	err := InitializeSentry(config)
	assert.NoError(t, err)
}

func TestInitializeSentry_MissingDSN(t *testing.T) {
	config := &schema.SentryConfig{
		Enabled: true,
		DSN:     "", // Missing DSN.
	}

	err := InitializeSentry(config)
	// Sentry should initialize but may have issues.
	// We just verify no panic occurs.
	_ = err
}

func TestInitializeSentry_WithConfiguration(t *testing.T) {
	config := &schema.SentryConfig{
		Enabled:             false, // Disabled to avoid actual Sentry calls in tests.
		DSN:                 "https://examplePublicKey@o0.ingest.sentry.io/0",
		Environment:         "test",
		Release:             "1.0.0",
		SampleRate:          1.0,
		Debug:               false,
		CaptureStackContext: true,
	}

	err := InitializeSentry(config)
	assert.NoError(t, err)
}

func TestInitializeSentry_DefaultSampleRate(t *testing.T) {
	config := &schema.SentryConfig{
		Enabled:    false, // Disabled for testing.
		DSN:        "https://examplePublicKey@o0.ingest.sentry.io/0",
		SampleRate: 0, // Should default to 1.0.
	}

	err := InitializeSentry(config)
	assert.NoError(t, err)
}

func TestCloseSentry(t *testing.T) {
	// Initialize Sentry first (disabled).
	config := &schema.SentryConfig{
		Enabled: false,
	}
	_ = InitializeSentry(config)

	// Close should not panic.
	assert.NotPanics(t, func() {
		CloseSentry()
	})
}

func TestCaptureError_Nil(t *testing.T) {
	// Should not panic on nil error.
	assert.NotPanics(t, func() {
		CaptureError(nil)
	})
}

func TestCaptureError_SimpleError(t *testing.T) {
	// Initialize Sentry (disabled).
	config := &schema.SentryConfig{
		Enabled: false,
	}
	_ = InitializeSentry(config)

	err := errors.New("test error")

	// Should not panic.
	assert.NotPanics(t, func() {
		CaptureError(err)
	})
}

func TestCaptureError_WithHints(t *testing.T) {
	// Initialize Sentry (disabled).
	config := &schema.SentryConfig{
		Enabled: false,
	}
	_ = InitializeSentry(config)

	err := Build(errors.New("test error")).
		WithHint("hint 1").
		WithHint("hint 2").
		Err()

	// Should not panic.
	assert.NotPanics(t, func() {
		CaptureError(err)
	})
}

func TestCaptureError_WithExitCode(t *testing.T) {
	// Initialize Sentry (disabled).
	config := &schema.SentryConfig{
		Enabled: false,
	}
	_ = InitializeSentry(config)

	err := Build(errors.New("test error")).
		WithExitCode(42).
		Err()

	// Should not panic.
	assert.NotPanics(t, func() {
		CaptureError(err)
	})
}

func TestCaptureErrorWithContext_Nil(t *testing.T) {
	// Should not panic on nil error.
	assert.NotPanics(t, func() {
		CaptureErrorWithContext(nil, nil)
	})
}

func TestCaptureErrorWithContext_WithContext(t *testing.T) {
	// Initialize Sentry (disabled).
	config := &schema.SentryConfig{
		Enabled: false,
	}
	_ = InitializeSentry(config)

	err := errors.New("test error")
	context := map[string]string{
		"component": "vpc",
		"stack":     "prod",
		"region":    "us-east-1",
	}

	// Should not panic.
	assert.NotPanics(t, func() {
		CaptureErrorWithContext(err, context)
	})
}

func TestCaptureErrorWithContext_CompleteExample(t *testing.T) {
	// Initialize Sentry (disabled).
	config := &schema.SentryConfig{
		Enabled: false,
	}
	_ = InitializeSentry(config)

	err := Build(errors.New("database connection failed")).
		WithHint("Check database credentials").
		WithHint("Verify network connectivity").
		WithContext("operation", "connect").
		WithExitCode(2).
		Err()

	context := map[string]string{
		"component": "vpc",
		"stack":     "prod",
		"region":    "us-east-1",
	}

	// Should not panic.
	assert.NotPanics(t, func() {
		CaptureErrorWithContext(err, context)
	})
}
