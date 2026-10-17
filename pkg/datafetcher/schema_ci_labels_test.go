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
