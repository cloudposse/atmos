package datafetcher

import "testing"

// TestSchemaCloudFormationPackaging validates the shared provision target selector in every schema copy.
func TestSchemaCloudFormationPackaging(t *testing.T) {
	schemas := map[string][]byte{
		"manifest":     loadEmbeddedSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
		"fixture":      loadFixtureSchemaBytes(t),
	}
	for name, schema := range schemas {
		t.Run(name, func(t *testing.T) {
			for _, selector := range []any{"artifacts", 42} {
				manifest := map[string]any{"components": map[string]any{
					"aws/cloudformation": map[string]any{"vpc": map[string]any{
						"stack_name": "vpc", "template": "template.yaml",
						"provision": map[string]any{"targets": map[string]any{
							"deploy": map[string]any{"kind": "aws/cloudformation", "packaging": selector},
						}},
					}},
				}}
				if selector == "artifacts" {
					assertSchemaValid(t, schema, manifest)
				} else {
					assertSchemaInvalid(t, schema, manifest)
				}
			}
		})
	}
}
