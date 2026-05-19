package schema

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeRetryConfig_NilOrEmpty(t *testing.T) {
	got, err := DecodeRetryConfig(nil)
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = DecodeRetryConfig(map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDecodeRetryConfig_FullShape(t *testing.T) {
	raw := map[string]any{
		"max_attempts":     5,
		"backoff_strategy": "exponential",
		"initial_delay":    "2s",
		"max_delay":        "30s",
		"max_elapsed_time": "1m",
		"multiplier":       2.5,
		"random_jitter":    0.3,
		"conditions":       []any{"/Bad Gateway/", "/5\\d\\d /"},
	}
	got, err := DecodeRetryConfig(raw)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NotNil(t, got.MaxAttempts)
	assert.Equal(t, 5, *got.MaxAttempts)
	assert.Equal(t, BackoffExponential, got.BackoffStrategy)
	require.NotNil(t, got.InitialDelay)
	assert.Equal(t, 2*time.Second, *got.InitialDelay)
	require.NotNil(t, got.MaxDelay)
	assert.Equal(t, 30*time.Second, *got.MaxDelay)
	require.NotNil(t, got.MaxElapsedTime)
	assert.Equal(t, time.Minute, *got.MaxElapsedTime)
	require.NotNil(t, got.Multiplier)
	assert.InDelta(t, 2.5, *got.Multiplier, 0.0001)
	require.NotNil(t, got.RandomJitter)
	assert.InDelta(t, 0.3, *got.RandomJitter, 0.0001)
	assert.Equal(t, []string{"/Bad Gateway/", "/5\\d\\d /"}, got.Conditions)
}

func TestDecodeRetryConfig_NotAMap(t *testing.T) {
	_, err := DecodeRetryConfig("nope")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidRetryConfig))
}

func TestDecodeRetryConfig_ConditionsOnly(t *testing.T) {
	got, err := DecodeRetryConfig(map[string]any{
		"max_attempts": 3,
		"conditions":   []any{"connection reset"},
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.MaxAttempts)
	assert.Equal(t, 3, *got.MaxAttempts)
	assert.Equal(t, []string{"connection reset"}, got.Conditions)
	// Unset fields stay nil.
	assert.Nil(t, got.InitialDelay)
	assert.Nil(t, got.MaxDelay)
}
