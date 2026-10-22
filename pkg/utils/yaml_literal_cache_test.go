package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStackLiteralCachePolicy(t *testing.T) {
	const source = "vars:\n  value: !literal '{{ external }}'\n  dynamic: !env ATMOS_CACHE_POLICY_TEST\n"
	for _, pin := range []string{"", "2026-10-19", ""} {
		for _, preserve := range []bool{false, true, false, true} {
			config := &schema.AtmosConfiguration{PreserveStackLiterals: preserve, Edition: pin}
			got, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](config, source, "literal-cache-policy.yaml")
			require.NoError(t, err)
			value := got["vars"].(map[string]any)["value"].(string)
			decoded, encoded := literalsource.Decode(value)
			assert.Equal(t, preserve, encoded)
			assert.Equal(t, "{{ external }}", decoded)
			_, tagged := yamlsource.Decode(got["vars"].(map[string]any)["dynamic"].(string))
			assert.Equal(t, pin == "", tagged)
		}
	}
}
