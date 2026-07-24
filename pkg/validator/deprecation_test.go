package validator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindDeprecatedYAMLFields(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	schema := `{
  "type": "object",
  "properties": {
    "legacy": {"deprecated": true, "x-atmos-replacement": "modern"},
    "items": {"type": "array", "items": {"type": "object", "properties": {"old": {"deprecated": true}}}},
    "providers": {"type": "object", "additionalProperties": {"type": "object", "properties": {"type": {"deprecated": true, "x-atmos-replacement": "kind"}}}}
  }
}`
	require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o600))

	fields, err := FindDeprecatedYAMLFields(nil, schemaPath, []byte("legacy: true\nitems:\n  - old: value\nproviders:\n  github:\n    type: release\n"))

	require.NoError(t, err)
	assert.Equal(t, []DeprecatedField{
		{Path: "items[0].old"},
		{Path: "legacy", Replacement: "modern"},
		{Path: "providers.github.type", Replacement: "kind"},
	}, fields)
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
