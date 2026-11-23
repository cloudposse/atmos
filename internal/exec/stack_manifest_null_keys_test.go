package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// processNullKeyManifest loads a single stack manifest, validating it against the Atmos manifest
// JSON Schema like `atmos validate stacks` does.
func processNullKeyManifest(t *testing.T, manifest string) error {
	t.Helper()
	dir := t.TempDir()
	stackPath := filepath.Join(dir, "dev.yaml")
	require.NoError(t, os.WriteFile(stackPath, []byte(manifest), 0o600))
	schemaPath, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "schemas", "atmos", "atmos-manifest", "1.0", "atmos-manifest.json"))
	require.NoError(t, err)
	_, err = ProcessYAMLConfigFile(
		&schema.AtmosConfiguration{}, dir, stackPath, map[string]map[string]any{}, nil,
		false, false, false, false, nil, nil, nil, nil, schemaPath,
	)
	return err
}

func TestStackManifestNullKeyIsValidationError(t *testing.T) {
	for _, tc := range []struct {
		name, manifest, path string
	}{
		{"null component name", "components:\n  terraform:\n    null:\n      vars:\n        a: 1\n", "components.terraform"},
		{"tilde component name", "components:\n  terraform:\n    ~:\n      vars: {}\n", "components.terraform"},
		{"null var name", "components:\n  terraform:\n    vpc:\n      vars:\n        null: 1\n", "components.terraform.vpc.vars"},
		{"null key under a non-string key", "components:\n  terraform:\n    vpc:\n      vars:\n        1:\n          null: x\n", "components.terraform.vpc.vars.1"},
		{"null key in a list item", "components:\n  terraform:\n    vpc:\n      vars:\n        rules:\n          - null: 1\n", "components.terraform.vpc.vars.rules[0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() { err = processNullKeyManifest(t, tc.manifest) })
			require.ErrorIs(t, err, errUtils.ErrInvalidStackManifest)
			assert.Contains(t, errUtils.Format(err, errUtils.DefaultFormatterConfig()), tc.path)
			assert.Contains(t, errUtils.Format(err, errUtils.DefaultFormatterConfig()), "dev.yaml")
		})
	}
}

func TestStackManifestQuotedNullKeyIsAllowed(t *testing.T) {
	require.NoError(t, processNullKeyManifest(t, "components:\n  terraform:\n    \"null\":\n      vars:\n        a: 1\n"))
}

func TestStackManifestNullValueIsAllowed(t *testing.T) {
	require.NoError(t, processNullKeyManifest(t, "components:\n  terraform:\n    vpc:\n      vars:\n        a: null\n        list: [~]\n"))
}
