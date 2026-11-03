package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestValidateAbsoluteSchemaBases(t *testing.T) {
	project := t.TempDir()
	schemas := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(schemas, "input.json"), []byte(`{"type":"object","required":["field"]}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(schemas, "input.rego"), []byte("package atmos\nerrors[msg] { input.field != data.constants.expected; msg := \"wrong value\" }\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(schemas, "modules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schemas, "modules", "constants.rego"), []byte("package constants\nexpected := \"value\"\n"), 0o600))
	config := &schema.AtmosConfiguration{BasePath: project, BasePathAbsolute: project, Schemas: map[string]any{
		"jsonschema": schema.ResourcePath{BasePath: schemas}, "opa": schema.ResourcePath{BasePath: schemas},
	}}
	for _, kind := range []string{"jsonschema", "opa"} {
		t.Run(kind, func(t *testing.T) {
			path := "input.json"
			var modules []string
			if kind == "opa" {
				path = "input.rego"
				modules = []string{"modules"}
			}
			valid, err := validateComponentInternal(config, map[string]any{"field": "value"}, path, kind, modules, 30)
			require.NoError(t, err)
			require.True(t, valid)
			valid, err = validateComponentInternal(config, map[string]any{}, path, kind, modules, 30)
			if kind == "jsonschema" {
				require.Error(t, err)
				require.False(t, valid)
			}
		})
	}
}
