package schema

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodeRetryConfig_NilOrEmpty asserts that DecodeRetryConfig returns (nil, nil)
// for nil and empty-map inputs so callers can write `if cfg != nil` without first
// checking the error — this is the contract the post-template-processing path in
// internal/exec/utils.go relies on.
func TestDecodeRetryConfig_NilOrEmpty(t *testing.T) {
	got, err := DecodeRetryConfig(nil)
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = DecodeRetryConfig(map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestDecodeRetryConfig_FullShape decodes every supported field at once and asserts both
// scalar values and pointer-vs-nil semantics, including the duration-string parsing path
// driven by mapstructure's StringToTimeDurationHookFunc.
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

// TestDecodeRetryConfig_NotAMap covers the type-guard branch — a non-map raw value
// must produce an error joined with ErrInvalidRetryConfig so the stack-processor can
// surface a precise config error at extraction time.
func TestDecodeRetryConfig_NotAMap(t *testing.T) {
	_, err := DecodeRetryConfig("nope")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidRetryConfig))
}

// TestDecodeRetryConfig_ConditionsOnly verifies that partial configs (only the fields
// the user set) decode correctly and unset fields stay nil — this is the common shape
// for minimal `retry: { max_attempts: 3, conditions: [...] }` blocks in stack manifests.
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

// TestDecodeRetryConfig_InvalidDurationString covers the decoder.Decode error path:
// a duration field with an unparseable string must produce an error joined with
// ErrInvalidRetryConfig so the surrounding stack-processor surfaces a precise message.
func TestDecodeRetryConfig_InvalidDurationString(t *testing.T) {
	_, err := DecodeRetryConfig(map[string]any{
		"initial_delay": "nope",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidRetryConfig), "error must wrap ErrInvalidRetryConfig")
}

// TestDecodeRetryConfig_UnknownFieldsIgnored documents that mapstructure is configured
// without ErrorUnused, so extra keys (e.g. forwards-compat fields, user typos) decode
// successfully without surfacing an error. If we ever tighten this contract, this test
// should be updated to assert the failure mode explicitly.
func TestDecodeRetryConfig_UnknownFieldsIgnored(t *testing.T) {
	got, err := DecodeRetryConfig(map[string]any{
		"max_attempts":           4,
		"this_is_not_a_real_key": "ignored",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.MaxAttempts)
	assert.Equal(t, 4, *got.MaxAttempts)
}

// TestDecodeRetryConfig_ConditionsOrderPreserved verifies that the conditions slice
// keeps the manifest-declared order — match-any iterates in slice order, so reorder
// regressions would silently change which pattern fires first.
func TestDecodeRetryConfig_ConditionsOrderPreserved(t *testing.T) {
	got, err := DecodeRetryConfig(map[string]any{
		"conditions": []any{"/first/", "/second/", "/third/"},
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Conditions, 3)
	assert.Equal(t, "/first/", got.Conditions[0])
	assert.Equal(t, "/third/", got.Conditions[2])
}
