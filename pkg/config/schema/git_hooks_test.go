package configschema

import (
	"bytes"
	"encoding/json"
	"testing"

	stjsonschema "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

func TestGeneratedSchemaGitHookSteps(t *testing.T) {
	compiler := stjsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("atmos-config.json", bytes.NewReader(generatedSchema(t))))
	compiled, err := compiler.Compile("atmos-config.json")
	require.NoError(t, err)

	for _, test := range []struct {
		name  string
		hook  string
		valid bool
	}{
		{"command", `{"command":"echo hello"}`, true},
		{"inline script", `{"steps":[{"name":"check","type":"script","interpreter":"starlark","script":"print(1)","timeout":"30s","retry":{"max_attempts":2}}]}`, true},
		{"shell shorthand", `{"steps":["echo hello"]}`, true},
		{"partial configuration", `{}`, true},
		{"invalid steps", `{"steps":42}`, false},
		{"invalid script", `{"steps":[{"type":"script","script":42}]}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var document any
			require.NoError(t, json.Unmarshal([]byte(`{"git":{"hooks":{"pre-commit":`+test.hook+`}}}`), &document))
			err := compiled.Validate(document)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
