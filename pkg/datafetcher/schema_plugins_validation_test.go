package datafetcher

import "testing"

func TestManifestSchema_PluginDefaults(t *testing.T) {
	schemas := map[string][]byte{
		"embedded":     loadEmbeddedSchemaBytes(t),
		"fixture":      loadFixtureSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
	}
	cases := []struct {
		name    string
		plugins any
		valid   bool
	}{
		{name: "plugin list", plugins: []any{"diff@v3.15.10", "secrets"}, valid: true},
		{name: "empty list", plugins: []any{}, valid: true},
		{name: "include", plugins: "!include plugins.yaml", valid: true},
		{name: "scalar", plugins: "diff"},
		{name: "non-string entry", plugins: []any{123}},
		{name: "map", plugins: map[string]any{"diff": "v3.15.10"}},
	}
	for schemaName, schemaData := range schemas {
		for _, componentType := range []string{"helm", "helmfile"} {
			for _, scope := range []string{"defaults", "component"} {
				for _, tc := range cases {
					t.Run(schemaName+"/"+componentType+"/"+scope+"/"+tc.name, func(t *testing.T) {
						section := map[string]any{"plugins": tc.plugins}
						manifest := map[string]any{componentType: section}
						if scope == "component" {
							manifest = map[string]any{
								"components": map[string]any{
									componentType: map[string]any{"example": section},
								},
							}
						}
						if tc.valid {
							assertSchemaValid(t, schemaData, manifest)
						} else {
							assertSchemaInvalid(t, schemaData, manifest)
						}
					})
				}
			}
		}
	}
}
