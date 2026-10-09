package datafetcher

import (
	"testing"
)

// provisionTargetAuthManifest builds a stack manifest with one CloudFormation component whose
// provision target carries the given auth block.
func provisionTargetAuthManifest(auth any) map[string]any {
	return cloudFormationComponent(map[string]any{
		"stack_name": "vpc",
		"path":       "template.yaml",
		"provision": map[string]any{
			"default": "artifacts",
			"targets": map[string]any{
				"artifacts": map[string]any{
					"kind":   "aws/s3",
					"bucket": "shared-cfn-artifacts",
					"auth":   auth,
				},
			},
		},
	})
}

// TestSchemaProvisionTargetAuth checks that every schema copy that models provision targets accepts the
// full auth form a target honors at runtime (identity shorthand, identities with default, providers) and
// still rejects unknown keys so typos fail validation.
func TestSchemaProvisionTargetAuth(t *testing.T) {
	schemas := map[string][]byte{"manifest": loadEmbeddedSchemaBytes(t), "stack-config": loadStackConfigSchemaBytes(t)}
	tests := []struct {
		name  string
		auth  any
		valid bool
	}{
		{name: "identity shorthand", auth: map[string]any{"identity": "shared-services"}, valid: true},
		{name: "identities with default", auth: map[string]any{"identities": map[string]any{"shared-services": map[string]any{"default": true}}}, valid: true},
		{name: "providers", auth: map[string]any{"providers": map[string]any{"sso": map[string]any{"kind": "aws/iam-identity-center", "region": "us-east-1", "start_url": "https://example.awsapps.com/start"}}}, valid: true},
		{name: "providers and identities", auth: map[string]any{
			"providers":  map[string]any{"sso": map[string]any{"kind": "aws/iam-identity-center", "region": "us-east-1", "start_url": "https://example.awsapps.com/start"}},
			"identities": map[string]any{"admin": map[string]any{"kind": "aws/permission-set", "default": true, "via": map[string]any{"provider": "sso"}, "principal": map[string]any{"name": "Admin", "account": map[string]any{"name": "shared"}}}},
		}, valid: true},
		{name: "realm and integrations", auth: map[string]any{"realm": "artifacts", "integrations": map[string]any{"ecr": map[string]any{"kind": "aws/ecr"}}}, valid: true},
		{name: "include string", auth: "!include auth.yaml", valid: true},
		{name: "empty mapping", auth: map[string]any{}, valid: true},
		{name: "unknown key is rejected", auth: map[string]any{"identitty": "shared-services"}},
		{name: "identity wrong type", auth: map[string]any{"identity": 42}},
		{name: "non include string is rejected", auth: "shared-services"},
	}
	for name, schema := range schemas {
		t.Run(name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if tt.valid {
						assertSchemaValid(t, schema, provisionTargetAuthManifest(tt.auth))
						return
					}
					assertSchemaInvalid(t, schema, provisionTargetAuthManifest(tt.auth))
				})
			}
		})
	}
}
