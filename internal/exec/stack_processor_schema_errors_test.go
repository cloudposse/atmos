package exec

import (
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// multiFailureSchema makes the object branch of a oneOf fail two keywords at once (a property type and
// additionalProperties). The validator groups multiple failures of one schema node under a wrapper whose
// message is empty.
const multiFailureSchema = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "definitions": {
    "auth": {
      "oneOf": [
        {"type": "string", "pattern": "^!include"},
        {"type": "object", "additionalProperties": false, "properties": {"identity": {"type": "string"}}}
      ]
    }
  },
  "type": "object",
  "properties": {"auth": {"$ref": "#/definitions/auth"}}
}`

// validateAgainstSchema compiles schemaText and returns the validation error for doc.
func validateAgainstSchema(t *testing.T, schemaText string, doc any) *jsonschema.ValidationError {
	t.Helper()

	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft7
	require.NoError(t, compiler.AddResource("schema.json", strings.NewReader(schemaText)))
	compiled, err := compiler.Compile("schema.json")
	require.NoError(t, err)

	var validationErr *jsonschema.ValidationError
	require.ErrorAs(t, compiled.Validate(doc), &validationErr)
	return validationErr
}

// TestFormatManifestSchemaValidationErrors_NoEmptyMessages verifies that schema nodes which only group
// several failed keywords (empty message) never render as their own bullet, and that the real causes
// are surfaced instead.
func TestFormatManifestSchemaValidationErrors_NoEmptyMessages(t *testing.T) {
	tests := []struct {
		name     string
		doc      map[string]any
		contains []string
	}{
		{
			name:     "unknown key and wrong type fail together",
			doc:      map[string]any{"auth": map[string]any{"identity": 42, "identitty": "x"}},
			contains: []string{"auth.identity:", "additionalProperties 'identitty' not allowed"},
		},
		{
			name:     "single unknown key",
			doc:      map[string]any{"auth": map[string]any{"identitty": "x"}},
			contains: []string{"additionalProperties 'identitty' not allowed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validationErr := validateAgainstSchema(t, multiFailureSchema, tt.doc)

			// Prove the fixture really produces a message-less wrapper, so this test cannot pass vacuously.
			if len(tt.contains) > 1 {
				hasEmptyNode := false
				for _, basicErr := range validationErr.BasicOutput().Errors {
					if basicErr.Error == "" && basicErr.KeywordLocation != "" {
						hasEmptyNode = true
					}
				}
				require.True(t, hasEmptyNode, "fixture must produce an empty-message wrapper node")
			}

			out := formatManifestSchemaValidationErrors("stack.yaml", validationErr, nil)

			lines := strings.Split(strings.TrimPrefix(out, "\n"), "\n")
			require.NotEmpty(t, lines)
			for _, line := range lines {
				assert.True(t, strings.HasPrefix(line, "- stack.yaml:"), "unexpected line %q", line)
				assert.NotRegexp(t, `error: [^:]*:\s*$`, line, "line must carry a message: %q", line)
			}
			for _, want := range tt.contains {
				assert.Contains(t, out, want)
			}
		})
	}
}
