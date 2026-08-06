package schema

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodeComponentPro_NilOrEmpty mirrors TestDecodeRetryConfig_NilOrEmpty's contract:
// callers can write `if cfg != nil` without first checking the error.
func TestDecodeComponentPro_NilOrEmpty(t *testing.T) {
	got, err := DecodeComponentPro(nil)
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = DecodeComponentPro(map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestDecodeComponentPro_NotAMap covers the type-guard branch.
func TestDecodeComponentPro_NotAMap(t *testing.T) {
	_, err := DecodeComponentPro("yes")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidComponentProConfig))
}

// TestDecodeComponentPro_FullShape decodes every supported field at once.
func TestDecodeComponentPro_FullShape(t *testing.T) {
	raw := map[string]any{
		"enabled": true,
		"drift_detection": map[string]any{
			"enabled": true,
		},
		"pull_request": map[string]any{
			"opened": map[string]any{
				"atmos-pro-terraform-plan.yaml": map[string]any{
					"inputs": map[string]any{"component": "vpc"},
				},
			},
		},
	}
	got, err := DecodeComponentPro(raw)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NotNil(t, got.Enabled)
	assert.True(t, *got.Enabled)
	require.NotNil(t, got.DriftDetection)
	require.NotNil(t, got.DriftDetection.Enabled)
	assert.True(t, *got.DriftDetection.Enabled)
	require.NotNil(t, got.PullRequest)
	require.Contains(t, got.PullRequest.Opened, "atmos-pro-terraform-plan.yaml")
	assert.Equal(t, "vpc", got.PullRequest.Opened["atmos-pro-terraform-plan.yaml"].Inputs["component"])
}

// TestDecodeComponentPro_UnknownFieldRejected is the reason this decoder exists: unlike
// DecodeRetryConfig, ErrorUnused is enabled here, so a typo like "enable" (meant "enabled")
// is rejected instead of silently decoding to an empty, effectively-enabled-by-default config.
func TestDecodeComponentPro_UnknownFieldRejected(t *testing.T) {
	_, err := DecodeComponentPro(map[string]any{"enable": false})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidComponentProConfig))
}

// TestDecodeComponentPro_PartialConfig verifies that only setting `enabled` (a common minimal
// shape) decodes correctly with every other field left nil.
func TestDecodeComponentPro_PartialConfig(t *testing.T) {
	got, err := DecodeComponentPro(map[string]any{"enabled": false})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Enabled)
	assert.False(t, *got.Enabled)
	assert.Nil(t, got.DriftDetection)
	assert.Nil(t, got.PullRequest)
	assert.Nil(t, got.Release)
	assert.Nil(t, got.MergeGroup)
}
