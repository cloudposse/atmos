package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStackLiteralCachePolicy(t *testing.T) {
	const source = "vars:\n  value: !literal '{{ external }}'\n"
	for _, preserve := range []bool{false, true, false, true} {
		config := &schema.AtmosConfiguration{PreserveStackLiterals: preserve}
		got, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](config, source, "literal-cache-policy.yaml")
		require.NoError(t, err)
		value := got["vars"].(map[string]any)["value"].(string)
		decoded, encoded := literalsource.Decode(value)
		assert.Equal(t, preserve, encoded)
		assert.Equal(t, "{{ external }}", decoded)
	}
}
