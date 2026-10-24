package datafetcher

import "testing"

func TestCILabelsMetadataSchema(t *testing.T) {
	for name, data := range map[string][]byte{
		"manifest": loadEmbeddedSchemaBytes(t),
		"stack":    loadStackConfigSchemaBytes(t),
	} {
		t.Run(name, func(t *testing.T) {
			for _, value := range []any{true, false, "true", `!ci.labels ["preview"]`} {
				assertSchemaValid(t, data, map[string]any{
					"components": map[string]any{"terraform": map[string]any{
						"preview": map[string]any{"metadata": map[string]any{
							"labels": map[string]any{"preview": value},
						}},
					}},
				})
			}
			assertSchemaInvalid(t, data, map[string]any{
				"components": map[string]any{"terraform": map[string]any{
					"preview": map[string]any{"metadata": map[string]any{
						"labels": map[string]any{"preview": []any{"invalid"}},
					}},
				}},
			})
		})
	}
	assertSchemaValid(t, loadEmbeddedSchemaBytes(t), map[string]any{
		"metadata": map[string]any{"labels": map[string]any{"preview": true, "disabled": false}},
	})
}

// TestYAMLFunctionsInBooleanFlagFields guards the documented `settings.pro.enabled: !ci.labels [...]`
// usage: stack validation sees the unevaluated function string, so boolean flag fields must accept it.
func TestYAMLFunctionsInBooleanFlagFields(t *testing.T) {
	const expression = `!ci.labels ["preview"]`
	component := func(section string, body map[string]any) map[string]any {
		return map[string]any{
			"components": map[string]any{"terraform": map[string]any{
				"preview": map[string]any{section: body},
			}},
		}
	}
	for name, data := range map[string][]byte{
		"manifest": loadEmbeddedSchemaBytes(t),
		"stack":    loadStackConfigSchemaBytes(t),
	} {
		t.Run(name, func(t *testing.T) {
			for _, value := range []any{true, false, expression, "!env PREVIEW"} {
				assertSchemaValid(t, data, component("metadata", map[string]any{"enabled": value}))
				assertSchemaValid(t, data, component("settings", map[string]any{"pro": map[string]any{"enabled": value}}))
			}
			// Plain strings and other types remain invalid; only YAML functions are deferred.
			for _, value := range []any{"yes", "true", 1, []any{"x"}} {
				assertSchemaInvalid(t, data, component("metadata", map[string]any{"enabled": value}))
				assertSchemaInvalid(t, data, component("settings", map[string]any{"pro": map[string]any{"enabled": value}}))
			}
		})
	}
	// Stack-wide defaults accept the function too (manifest schema only).
	assertSchemaValid(t, loadEmbeddedSchemaBytes(t), map[string]any{
		"metadata": map[string]any{"enabled": expression},
	})
}
