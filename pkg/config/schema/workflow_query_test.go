package configschema

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	stjsonschema "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

func TestWorkflowQuerySchema(t *testing.T) {
	root, err := RepoRoot()
	require.NoError(t, err)
	committed, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(EmbeddedPath)))
	require.NoError(t, err)
	for name, raw := range map[string][]byte{"generated": generatedSchema(t), "committed": committed} {
		t.Run(name, func(t *testing.T) {
			var document map[string]any
			require.NoError(t, json.Unmarshal(raw, &document))
			defs := document["$defs"].(map[string]any)
			step := defs["WorkflowStep"].(map[string]any)
			props := step["properties"].(map[string]any)
			query := props["query"].(map[string]any)
			require.Contains(t, query["anyOf"], map[string]any{"$ref": "#/$defs/yamlFunction"})
			compiler := stjsonschema.NewCompiler()
			require.NoError(t, compiler.AddResource("atmos-config.json", bytes.NewReader(raw)))
			compiled, err := compiler.Compile("atmos-config.json#/$defs/WorkflowStep/properties/query")
			require.NoError(t, err)
			for _, query := range []any{"vpc", map[string]any{"page": "1"}, nil, "!env ARTIFACT_QUERY"} {
				require.NoError(t, compiled.Validate(query), "query: %#v", query)
			}
			require.Error(t, compiled.Validate([]any{"vpc"}))
		})
	}
}
