package datafetcher

import "testing"

// TestSchemaCloudFormationComponentSecrets checks component-scoped secret declarations in every schema copy.
func TestSchemaCloudFormationComponentSecrets(t *testing.T) {
	schemas := map[string][]byte{
		"manifest":     loadEmbeddedSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
		"fixture":      loadFixtureSchemaBytes(t),
	}
	tests := []struct {
		name    string
		secrets any
		valid   bool
	}{
		{
			name: "component secret declaration",
			secrets: map[string]any{"vars": map[string]any{
				"database_password": map[string]any{
					"store":     "ssm",
					"reference": "/dev/database/password",
					"required":  true,
				},
			}},
			valid: true,
		},
		{name: "scalar secrets rejected", secrets: 42},
	}
	for name, schema := range schemas {
		t.Run(name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					manifest := map[string]any{"components": map[string]any{
						"aws/cloudformation": map[string]any{"vpc": map[string]any{
							"stack_name": "vpc",
							"template":   "template.yaml",
							"secrets":    tt.secrets,
						}},
					}}
					if tt.valid {
						assertSchemaValid(t, schema, manifest)
						return
					}
					assertSchemaInvalid(t, schema, manifest)
				})
			}
		})
	}
}
