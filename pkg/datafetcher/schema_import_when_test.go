package datafetcher

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"

	"github.com/cloudposse/atmos/pkg/condition"
)

func TestImportWhenSchemaMatchesDecoder(t *testing.T) {
	tests := []struct {
		name  string
		value any
		valid bool
	}{
		{"unset", nil, true},
		{"predicate", "ci", true},
		{"expression", "labels.pci == true", true},
		{"boolean expression", "false", true},
		{"list", []any{"ci", map[string]any{"not": "local"}}, true},
		{"all", map[string]any{"all": []any{"ci", "always"}}, true},
		{"any", map[string]any{"any": []any{"local", "ci"}}, true},
		{"single child", map[string]any{"any": "ci"}, true},
		{"normalized operator", map[string]any{" ALL ": []any{"ci"}}, true},
		{"empty children", map[string]any{"all": nil}, true},
		{"false", false, false},
		{"true", true, false},
		{"number", 42, false},
		{"cel object", map[string]any{"cel": "true"}, false},
		{"nested boolean", []any{"ci", false}, false},
		{"nested cel object", map[string]any{"all": []any{map[string]any{"cel": "true"}}}, false},
		{"empty object", map[string]any{}, false},
		{"multiple operators", map[string]any{"all": "ci", "any": "local"}, false},
		{"missing negation", map[string]any{"not": nil}, false},
		{"empty negation", map[string]any{"not": " \t"}, false},
	}
	for schemaName, data := range map[string][]byte{
		"manifest": loadEmbeddedSchemaBytes(t),
		"stack":    loadStackConfigSchemaBytes(t),
	} {
		t.Run(schemaName, func(t *testing.T) {
			validator, err := gojsonschema.NewSchema(gojsonschema.NewBytesLoader(data))
			require.NoError(t, err)
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					_, decodeErr := condition.NewWithPolicy(tt.value, true)
					require.Equal(t, tt.valid, decodeErr == nil, "decoder: %v", decodeErr)
					document := map[string]any{"import": []any{
						map[string]any{"path": "mixins/pci", "when": tt.value},
					}}
					result, err := validator.Validate(gojsonschema.NewGoLoader(document))
					require.NoError(t, err)
					require.Equal(t, tt.valid, result.Valid(), "schema errors: %v", result.Errors())
				})
			}
		})
	}
}
