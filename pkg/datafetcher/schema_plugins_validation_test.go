package datafetcher

import "testing"

func TestManifestSchema_PluginDefaults(t *testing.T) {
	schemas := map[string][]byte{
		"embedded":     loadEmbeddedSchemaBytes(t),
		"fixture":      loadFixtureSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
	}
	for schemaName, schemaData := range schemas {
		for _, componentType := range []string{"helm", "helmfile"} {
			t.Run(schemaName+"/"+componentType, func(t *testing.T) {
				for _, plugins := range []any{[]any{"diff@v3.15.10", "secrets"}, []any{}, "!include plugins.yaml"} {
					assertSchemaValid(t, schemaData, map[string]any{componentType: map[string]any{"plugins": plugins}})
				}
				for _, plugins := range []any{"diff", []any{123}, map[string]any{"diff": "v3.15.10"}} {
					assertSchemaInvalid(t, schemaData, map[string]any{componentType: map[string]any{"plugins": plugins}})
				}
			})
		}
	}
}
