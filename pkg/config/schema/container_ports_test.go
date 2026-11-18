package configschema

import (
	"bytes"
	"testing"

	stjsonschema "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

func TestContainerPortHostSchema(t *testing.T) {
	compiler := stjsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("atmos-config.json", bytes.NewReader(generatedSchema(t))))
	compiled, err := compiler.Compile("atmos-config.json#/$defs/ContainerPort")
	require.NoError(t, err)
	for _, host := range []any{nil, 8080, "8080", "127.0.0.1:8080", "[::1]:0", "!env PORT"} {
		require.NoError(t, compiled.Validate(map[string]any{"host": host, "container": 3000}))
	}
	for _, host := range []any{true, 1.5, []any{8080}, map[string]any{"port": 8080}} {
		require.Error(t, compiled.Validate(map[string]any{"host": host, "container": 3000}))
	}
}
