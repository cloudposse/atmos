package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestIsAtmosConfigDocument(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "root atmos.yaml", path: "/repo/atmos.yaml", want: true},
		{name: "root atmos.yml", path: "/repo/atmos.yml", want: true},
		{name: "hidden atmos config", path: "/repo/.atmos.yaml", want: true},
		{name: "atmos.d fragment", path: "/repo/atmos.d/logs.yaml", want: true},
		{name: "hidden atmos.d fragment", path: "/repo/.atmos.d/dev.yaml", want: true},
		{name: "hidden profile fragment", path: "/repo/.atmos/profiles/dev/settings.yaml", want: true},
		{name: "stack manifest", path: "/repo/stacks/deploy/prod.yaml", want: false},
		{name: "generic profiles dir is not claimed", path: "/repo/profiles/dev/settings.yaml", want: false},
		{name: "vendor manifest", path: "/repo/vendor.yaml", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isAtmosConfigDocument(tt.path))
		})
	}
}

func TestSchemaFieldToJSONPath(t *testing.T) {
	tests := []struct {
		field string
		want  string
	}{
		{field: "(root)", want: ""},
		{field: "logs.level", want: "logs.level"},
		{field: "commands.0.env", want: "commands[0].env"},
		{field: "commands.1.steps.12.name", want: "commands[1].steps[12].name"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			assert.Equal(t, tt.want, schemaFieldToJSONPath(tt.field))
		})
	}
}

func TestValidateConfigSchema(t *testing.T) {
	handler := &Handler{}

	t.Run("valid atmos.yaml produces no diagnostics", func(t *testing.T) {
		doc := &Document{
			URI:  "file:///repo/atmos.yaml",
			Text: "base_path: \"./\"\nlogs:\n  level: Debug\n",
		}

		assert.Empty(t, handler.validateConfigSchema(doc))
	})

	t.Run("YAML functions are accepted", func(t *testing.T) {
		doc := &Document{
			URI:  "file:///repo/atmos.yaml",
			Text: "logs: !include shared-logs.yaml\n",
		}

		assert.Empty(t, handler.validateConfigSchema(doc))
	})

	t.Run("type violation produces a positioned diagnostic", func(t *testing.T) {
		doc := &Document{
			URI:  "file:///repo/atmos.yaml",
			Text: "base_path: \"./\"\nlogs:\n  level: 42\n",
		}

		diagnostics := handler.validateConfigSchema(doc)

		require.NotEmpty(t, diagnostics)
		assert.Contains(t, diagnostics[0].Message, "logs.level")
		// `level: 42` is on line 3 (0-based: 2).
		assert.Equal(t, uint32(2), diagnostics[0].Range.Start.Line)
		assert.Equal(t, "atmos-lsp", *diagnostics[0].Source)
	})

	t.Run("deprecated property produces a warning diagnostic", func(t *testing.T) {
		doc := &Document{
			URI:  "file:///repo/atmos.yaml",
			Text: "stacks:\n  name_pattern: '{stage}'\n",
		}

		diagnostics := handler.validateConfigSchema(doc)

		require.Len(t, diagnostics, 1)
		assert.Equal(t, protocol.DiagnosticSeverityWarning, *diagnostics[0].Severity)
		assert.Contains(t, diagnostics[0].Message, "`stacks.name_pattern` is deprecated")
	})
}

// TestValidateAtmosFileRoutesConfigDocuments confirms atmos.yaml documents get
// schema diagnostics instead of stack heuristics (and vice versa).
func TestValidateAtmosFileRoutesConfigDocuments(t *testing.T) {
	handler := &Handler{}

	t.Run("config document validated against config schema", func(t *testing.T) {
		doc := &Document{
			URI:  "file:///repo/atmos.yaml",
			Text: "logs:\n  level: 42\n",
		}

		diagnostics := handler.validateAtmosFile(doc)

		require.NotEmpty(t, diagnostics)
		assert.Contains(t, diagnostics[0].Message, "logs.level")
	})

	t.Run("config document skips stack heuristics", func(t *testing.T) {
		// `import:` as a string is valid in atmos.yaml (StringToSliceHookFunc)
		// but the stack heuristic would flag it; config documents must not.
		doc := &Document{
			URI:  "file:///repo/atmos.yaml",
			Text: "import: path/to/extra-config.yaml\n",
		}

		assert.Empty(t, handler.validateAtmosFile(doc))
	})

	t.Run("stack manifest keeps stack heuristics", func(t *testing.T) {
		doc := &Document{
			URI:  "file:///repo/stacks/prod.yaml",
			Text: "import: not-an-array\n",
		}

		diagnostics := handler.validateAtmosFile(doc)

		require.NotEmpty(t, diagnostics)
		assert.Contains(t, diagnostics[0].Message, "'import' should be an array")
	})
}
