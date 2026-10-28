package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestSchemaReferencesStayInsidePackage(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	directory := filepath.Join(parent, "plugin")
	require.NoError(t, os.Mkdir(directory, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(parent, "escape.json"), []byte(`{"type":"string"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "fields.json"), []byte(`{"type":"string","minLength":1}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "config.json"), []byte(`{"type":"object","properties":{"issuer":{"$ref":"fields.json"}},"required":["issuer"]}`), 0o600))
	compiled, err := CompileSchema(directory, "config.json")
	require.NoError(t, err)
	require.NoError(t, compiled.Validate(map[string]any{"issuer": "https://issuer.example"}))
	require.Error(t, compiled.Validate(map[string]any{"issuer": ""}))
	require.Error(t, compiled.Validate(map[string]any{}))
	for _, test := range []struct{ name, body string }{
		{"network", `{"$ref":"https://example.invalid/schema.json"}`},
		{"outside", `{"$ref":"%2e%2e/escape.json"}`},
		{"missing", `{"$ref":"absent.json"}`},
		{"malformed", `not json`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			name := test.name + ".json"
			require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte(test.body), 0o600))
			_, err := CompileSchema(directory, name)
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		})
	}
	_, err = CompileSchema(directory, "../outside.json")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

func TestAuthAndCommandExportMetadata(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schema.json"), []byte(`{"type":"object"}`), 0o600))
	for _, test := range []struct {
		name     string
		metadata map[string]any
		valid    bool
	}{
		{"valid", map[string]any{"schema": "schema.json", "credentials": []any{"oidc/v1", "aws/v1", "custom-token.v2/v12"}}, true},
		{"missing schema", map[string]any{"credentials": []any{"oidc/v1"}}, false},
		{"schema read fails", map[string]any{"schema": "missing.json", "credentials": []any{"oidc/v1"}}, false},
		{"missing output", map[string]any{"schema": "schema.json"}, false},
		{"empty output", map[string]any{"schema": "schema.json", "credentials": []any{}}, false},
		{"wrong output type", map[string]any{"schema": "schema.json", "credentials": []any{42}}, false},
		{"unversioned output", map[string]any{"schema": "schema.json", "credentials": []any{"oidc"}}, false},
		{"missing version", map[string]any{"schema": "schema.json", "credentials": []any{"oidc/v"}}, false},
		{"nonnumeric version", map[string]any{"schema": "schema.json", "credentials": []any{"oidc/vbeta"}}, false},
		{"missing type", map[string]any{"schema": "schema.json", "credentials": []any{"/v1"}}, false},
		{"extra segment", map[string]any{"schema": "schema.json", "credentials": []any{"a/v1/extra"}}, false},
		{"zero version", map[string]any{"schema": "schema.json", "credentials": []any{"oidc/v0"}}, false},
		{"leading zero version", map[string]any{"schema": "schema.json", "credentials": []any{"oidc/v01"}}, false},
		{"uppercase type", map[string]any{"schema": "schema.json", "credentials": []any{"OIDC/v1"}}, false},
		{"whitespace", map[string]any{"schema": "schema.json", "credentials": []any{"oidc/v1 "}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateAuth(Binding{Directory: directory, Metadata: test.metadata})
			if test.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			}
		})
	}
	require.NoError(t, ValidateCommand(Binding{Directory: directory, Metadata: map[string]any{"description": "About this plugin", "schema": "schema.json"}}))
	for _, metadata := range []map[string]any{{}, {"description": "  "}, {"description": "About", "schema": true}, {"description": "About", "schema": "missing.json"}} {
		require.ErrorIs(t, ValidateCommand(Binding{Directory: directory, Metadata: metadata}), errUtils.ErrPluginInvalidConfig)
	}
	require.NoError(t, ValidateCommand(Binding{Metadata: map[string]any{"description": "About"}}))
}
