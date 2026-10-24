package validator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindDeprecatedYAMLFields(t *testing.T) {
	tests := []struct {
		name     string
		schema   string
		yaml     string
		expected []DeprecatedField
	}{
		{
			name: "properties arrays and additional properties",
			schema: `{
  "type": "object",
  "properties": {
    "legacy": {"deprecated": true, "x-atmos-replacement": "modern"},
    "items": {"type": "array", "items": {"type": "object", "properties": {"old": {"deprecated": true}}}},
    "providers": {"type": "object", "additionalProperties": {"type": "object", "properties": {"type": {"deprecated": true, "x-atmos-replacement": "kind"}}}}
  }
}`,
			yaml: "legacy: true\nitems:\n  - old: value\nproviders:\n  github:\n    type: release\n",
			expected: []DeprecatedField{
				{Path: "items[0].old"},
				{Path: "legacy", Replacement: "modern"},
				{Path: "providers.github.type", Replacement: "kind"},
			},
		},
		{
			name: "local references and composition branches",
			schema: `{
  "type": "object",
  "$defs": {"legacy": {"deprecated": true, "x-atmos-replacement": "modern"}},
  "properties": {
    "referenced": {"$ref": "#/$defs/legacy"},
    "all": {"allOf": [{"deprecated": true, "x-atmos-replacement": "all-modern"}]},
    "any": {"anyOf": [{"deprecated": true, "x-atmos-replacement": "any-modern"}]},
    "one": {"oneOf": [{"deprecated": true, "x-atmos-replacement": "one-modern"}]}
  }
}`,
			yaml: "referenced: true\nall: true\nany: true\none: true\n",
			expected: []DeprecatedField{
				{Path: "all", Replacement: "all-modern"},
				{Path: "any", Replacement: "any-modern"},
				{Path: "one", Replacement: "one-modern"},
				{Path: "referenced", Replacement: "modern"},
			},
		},
		{
			name: "pattern properties",
			schema: `{
  "type": "object",
  "patternProperties": {"^legacy_": {"deprecated": true, "x-atmos-replacement": "modern"}}
}`,
			yaml:     "legacy_name: true\n",
			expected: []DeprecatedField{{Path: "legacy_name", Replacement: "modern"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schemaPath := filepath.Join(t.TempDir(), "schema.json")
			require.NoError(t, os.WriteFile(schemaPath, []byte(test.schema), 0o600))

			fields, err := FindDeprecatedYAMLFields(nil, schemaPath, []byte(test.yaml))

			require.NoError(t, err)
			assert.Equal(t, test.expected, fields)
		})
	}
}

func TestFindDeprecatedYAMLFieldsSuppressesParentWarning(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	schema := `{"type":"object","properties":{"legacy":{"deprecated":true,"properties":{"child":{"deprecated":true}}}}}`
	require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o600))

	fields, err := FindDeprecatedYAMLFields(nil, schemaPath, []byte("legacy:\n  child: true\n"))

	require.NoError(t, err)
	assert.Equal(t, []DeprecatedField{{Path: "legacy.child"}}, fields)
}

func TestFormatDeprecatedFieldKeepsReplacementNotesOutsideCode(t *testing.T) {
	assert.Equal(
		t,
		"`settings.terminal.no_color` is deprecated; use `settings.terminal.color` (invert the value)",
		FormatDeprecatedField(DeprecatedField{
			Path:        "settings.terminal.no_color",
			Replacement: "settings.terminal.color (invert the value)",
		}),
	)
}
