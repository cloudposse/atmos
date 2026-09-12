package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFormatOutputs_DelegatesToSharedOutput verifies the deprecated wrapper
// still forwards to pkg/output correctly (params in the right order, error
// and result passed through unchanged) -- a real regression guard against a
// copy-paste mistake in the delegation, not a test of pkg/output's own
// formatting logic (which pkg/output/format_test.go already covers).
func TestFormatOutputs_DelegatesToSharedOutput(t *testing.T) {
	outputs := map[string]any{"url": "https://example.com"}

	result, err := FormatOutputs(outputs, FormatJSON)
	require.NoError(t, err)
	assert.Contains(t, result, `"url": "https://example.com"`)
}

func TestFormatOutputsWithOptions_DelegatesToSharedOutput(t *testing.T) {
	outputs := map[string]any{"nested": map[string]any{"id": "vpc-123"}}

	result, err := FormatOutputsWithOptions(outputs, FormatJSON, FormatOptions{Flatten: true})
	require.NoError(t, err)
	assert.Contains(t, result, "nested_id")
	assert.Contains(t, result, "vpc-123")
}

func TestFlattenMap_DelegatesToSharedOutput(t *testing.T) {
	result := FlattenMap(map[string]any{"a": map[string]any{"b": "c"}}, "", DefaultFlattenSeparator)
	assert.Equal(t, "c", result["a_b"])
}

func TestFormatSingleValue_DelegatesToSharedOutput(t *testing.T) {
	result, err := FormatSingleValue("key", "value", FormatEnv)
	require.NoError(t, err)
	assert.Equal(t, "key=value\n", result)
}

func TestFormatSingleValueWithOptions_DelegatesToSharedOutput(t *testing.T) {
	result, err := FormatSingleValueWithOptions("key", "value", FormatEnv, FormatOptions{})
	require.NoError(t, err)
	assert.Equal(t, "key=value\n", result)
}
