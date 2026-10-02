package datafetcher

import "testing"

func TestSchemaCloudFormationTypeDefaults(t *testing.T) {
	schemas := map[string][]byte{"manifest": loadEmbeddedSchemaBytes(t), "stack-config": loadStackConfigSchemaBytes(t)}
	defaults := map[string]any{
		"generate":     map[string]any{"template.yaml": "Resources: {}"},
		"locals":       map[string]any{"region": "us-east-1"},
		"auth":         map[string]any{},
		"dependencies": map[string]any{"components": []any{map[string]any{"component": "network"}}},
		"source":       map[string]any{"uri": "https://example.com/template.yaml"},
		"provision":    map[string]any{"default": "deploy", "targets": map[string]any{"deploy": map[string]any{"kind": "aws/cloudformation"}}},
	}
	for name, schema := range schemas {
		t.Run(name, func(t *testing.T) {
			for key, value := range defaults {
				t.Run(key, func(t *testing.T) {
					assertSchemaValid(t, schema, map[string]any{"aws/cloudformation": map[string]any{key: value}})
				})
			}
			assertSchemaInvalid(t, schema, map[string]any{"aws/cloudformation": map[string]any{"unknown_setting": true}})
			assertSchemaInvalid(t, schema, map[string]any{"aws/cloudformation": map[string]any{"provision": 42}})
		})
	}
}
