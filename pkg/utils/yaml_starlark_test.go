package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/function/starlarksource"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStarlarkTagPreservesBodyAndLocation(t *testing.T) {
	input := "vars:\n  tags: !starlark |\n    return {\"x\": \"{{literal}}\"}\n"
	result, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{}, input, "catalog/base.yaml")
	require.NoError(t, err)
	source := starlarksource.Decode(result["vars"].(map[string]any)["tags"].(string))
	assert.Equal(t, "catalog/base.yaml", source.File)
	assert.Equal(t, int32(3), source.Line)
	assert.Equal(t, "return {\"x\": \"{{literal}}\"}\n", source.Code)
}

func TestStarlarkTagRequiresScalar(t *testing.T) {
	_, err := UnmarshalYAML[map[string]any]("value: !starlark {a: b}")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scalar function body")
}
