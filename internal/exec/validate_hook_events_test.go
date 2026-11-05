package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStackHookEventValidationScopes(t *testing.T) {
	invalid := map[string]any{"typo": map[string]any{"events": []string{"after.terraform.aply"}}}
	for _, scope := range []string{"global", "type", "component", "overrides", "custom"} {
		t.Run(scope, func(t *testing.T) {
			component := map[string]any{}
			stack := map[string]any{"components": map[string]any{"terraform": map[string]any{"example": component}}}
			switch scope {
			case "global":
				stack["hooks"] = invalid
			case "type":
				stack["terraform"] = map[string]any{"hooks": invalid}
			case "component":
				component["hooks"] = invalid
			case "overrides":
				component["overrides"] = map[string]any{"hooks": invalid}
			case "custom":
				stack["components"] = map[string]any{"widgets": map[string]any{"example": map[string]any{"hooks": invalid}}}
			}
			dir := t.TempDir()
			_, _, err := ProcessStackConfig(&schema.AtmosConfiguration{}, dir, "", "", "", "", filepath.Join(dir, "dev.yaml"), stack, false, false, "", nil, nil, false)
			require.ErrorIs(t, err, errUtils.ErrUnsupportedHookEvent)
			require.Contains(t, err.Error(), "dev")
			require.NoError(t, validateStackHookEvents("2026-10-18", stack))
		})
	}
	require.NoError(t, validateStackHookEvents("", map[string]any{"vars": map[string]any{"hooks": invalid}}))
}

func TestStackHookEventValidationCacheIsolation(t *testing.T) {
	config := &schema.AtmosConfiguration{Edition: "2026-10-18"}
	old := getFindStacksMapCacheKey(config, false)
	config.Edition = "2026-10-19"
	require.NotEqual(t, old, getFindStacksMapCacheKey(config, false))
}

func TestManifestSchemaValidationChecksHookEvents(t *testing.T) {
	for _, pin := range []string{"", "2026-10-18"} {
		t.Run("edition-"+pin, func(t *testing.T) {
			root := t.TempDir()
			manifest := filepath.Join(root, "catalog.yaml")
			schemaPath := filepath.Join(root, "schema.json")
			require.NoError(t, os.WriteFile(schemaPath, []byte(`{"type":"object"}`), 0o600))
			require.NoError(t, os.WriteFile(manifest, []byte("hooks:\n  typo:\n    events: [after.terraform.aply]\n"), 0o600))
			_, err := ProcessYAMLConfigFile(&schema.AtmosConfiguration{Edition: pin}, root, manifest,
				map[string]map[string]any{}, nil, false, false, false, false,
				nil, nil, nil, nil, schemaPath)
			if pin == "" {
				require.ErrorIs(t, err, errUtils.ErrUnsupportedHookEvent)
				require.Contains(t, err.Error(), "catalog.yaml")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
