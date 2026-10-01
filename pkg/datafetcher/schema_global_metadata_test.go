package datafetcher

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

// TestGlobalMetadataSchema_ValidatesAllowlist locks in the JSON-Schema-level enforcement layer
// for global (stack-manifest-root) metadata: the allowed data-bag/gate fields validate, while a
// structural/identity field (metadata.inherits) set at the same scope is rejected.
func TestGlobalMetadataSchema_ValidatesAllowlist(t *testing.T) {
	data, err := (&atmosFetcher{}).FetchData(embeddedSchemaSource)
	require.NoError(t, err, "failed to fetch embedded manifest schema")
	schemaLoader := gojsonschema.NewBytesLoader(data)

	tests := []struct {
		name    string
		doc     map[string]any
		wantErr bool
	}{
		{
			name: "allowed global metadata fields validate",
			doc: map[string]any{
				"metadata": map[string]any{
					"labels":                      map[string]any{"org": "acme"},
					"tags":                        []any{"production"},
					"custom":                      map[string]any{"owner": "platform-team"},
					"enabled":                     true,
					"locked":                      false,
					"terraform_workspace_pattern": "{tenant}-{environment}-{stage}",
				},
			},
			wantErr: false,
		},
		{
			name: "disallowed inherits field at global scope is rejected",
			doc: map[string]any{
				"metadata": map[string]any{
					"inherits": []any{"vpc/defaults"},
				},
			},
			wantErr: true,
		},
		{
			name: "disallowed component field at global scope is rejected",
			doc: map[string]any{
				"metadata": map[string]any{
					"component": "vpc/network",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := gojsonschema.Validate(schemaLoader, gojsonschema.NewGoLoader(tt.doc))
			require.NoError(t, err)
			if tt.wantErr {
				require.False(t, result.Valid(), "expected schema validation to fail for %v", tt.doc)
			} else {
				require.True(t, result.Valid(), "expected schema validation to succeed, got errors: %v", result.Errors())
			}
		})
	}
}
