package exec

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/yaml/scalar"
)

func TestNormalizeVariableLayers(t *testing.T) {
	for _, kind := range []string{"terraform", "packer"} {
		component := map[string]any{"vars": []any{
			map[string]any{"config": map[string]any{"old": true}, "keep": 0, "list": []any{"old"}},
			map[string]any{"config": map[string]any{"new": true}, "list": []any{}, "empty": nil},
		}}
		manifest := map[string]any{"components": map[string]any{kind: map[string]any{"app": component, "existing": map[string]any{"vars": map[string]any{}}}}}
		require.NoError(t, normalizeVariableLayers(manifest, "dev.yaml"))
		require.Equal(t, map[string]any{"config": map[string]any{"new": true}, "keep": 0, "list": []any{}, "empty": nil}, component["vars"])
		component["vars"] = []any{map[string]any{}, "invalid"}
		require.ErrorContains(t, normalizeVariableLayers(manifest, "dev.yaml"), "dev.yaml: components."+kind+".app.vars[1]")
	}
}

func TestValidateVariableNumbers(t *testing.T) {
	require.NoError(t, processNullKeyManifest(t, "components:\n  terraform:\n    vpc:\n      vars:\n        big: !!float 1e500\n"))
	// The two values round to the same binary float; schema validation must
	// still reject the lower value when an exact lower bound is declared.
	const name = "numbers.json"
	schemaText := `{"type":"number","minimum":123456789012345678901234567890123456789}`
	ok, err := ValidateWithJsonSchema(scalar.Number("123456789012345678901234567890123456788"), name, schemaText)
	require.Error(t, err)
	require.False(t, ok)
}
