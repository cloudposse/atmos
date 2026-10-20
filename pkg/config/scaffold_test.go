package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.AtmosConfiguration{Scaffold: &schema.ScaffoldConfiguration{}}

func TestScaffoldEffectiveConfigPreservesNamesAndSourceOrigin(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	base := filepath.Join(root, "catalog", "base.yaml")
	overlay := filepath.Join(root, "company.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(base), 0o755))
	require.NoError(t, os.WriteFile(base, []byte("scaffold:\n  templates:\n    Company.Stack:\n      source: ./recipe\n      description: Base\n      target_dir: ./out\n"), 0o644))
	require.NoError(t, os.WriteFile(overlay, []byte("scaffold:\n  templates:\n    Company.Stack:\n      description: Override\n"), 0o644))
	config, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigFilesFromArg: []string{base, overlay}})
	require.NoError(t, err)
	require.NotNil(t, config.Scaffold)
	assert.Equal(t, map[string]any{"Company.Stack": map[string]any{"source": "./recipe", "description": "Override", "target_dir": "./out"}}, config.Scaffold.Templates)
	assert.Equal(t, filepath.Dir(base), config.Scaffold.SourceDirs["Company.Stack"])

	// A later source replacement also replaces its origin; a subsequent load
	// cannot inherit entries or origins from the previous invocation.
	require.NoError(t, os.WriteFile(overlay, []byte("scaffold:\n  templates:\n    Company.Stack:\n      source: ./other\n"), 0o644))
	next, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigFilesFromArg: []string{base, overlay}})
	require.NoError(t, err)
	assert.Equal(t, root, next.Scaffold.SourceDirs["Company.Stack"])
	assert.Equal(t, "./other", next.Scaffold.Templates["Company.Stack"].(map[string]any)["source"])
	assert.Equal(t, "./recipe", config.Scaffold.Templates["Company.Stack"].(map[string]any)["source"])
}

func TestScaffoldTemplatesFromImportsAndProfiles(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	imported := filepath.Join(root, "catalog", "templates.yaml")
	profile := filepath.Join(root, "profiles", "company", "atmos.yaml")
	main := filepath.Join(root, "company.yaml")
	for _, path := range []string{imported, profile} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	}
	require.NoError(t, os.WriteFile(imported, []byte("scaffold:\n  templates:\n    Company:\n      source: ./recipe\n      description: Imported\n"), 0o644))
	require.NoError(t, os.WriteFile(main, []byte("profiles:\n  base_path: ./profiles\nimport:\n  - catalog/templates.yaml\n"), 0o644))
	require.NoError(t, os.WriteFile(profile, []byte("scaffold:\n  templates:\n    Company:\n      description: Profile\n    ProfileOnly:\n      source: ./local-recipe\n"), 0o644))
	config, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigFilesFromArg: []string{main}, ProfilesFromArg: []string{"company"}})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"source": "./recipe", "description": "Profile"}, config.Scaffold.Templates["Company"])
	assert.Equal(t, filepath.Dir(imported), config.Scaffold.SourceDirs["Company"])
	assert.Equal(t, map[string]any{"source": "./local-recipe"}, config.Scaffold.Templates["ProfileOnly"])
	assert.Equal(t, filepath.Dir(profile), config.Scaffold.SourceDirs["ProfileOnly"])
}

func TestEmptyEffectiveScaffoldCatalogDoesNotAddSerializedConfiguration(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	main := filepath.Join(root, "company.yaml")
	require.NoError(t, os.WriteFile(main, []byte("base_path: .\n"), 0o644))
	config, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigFilesFromArg: []string{main}})
	require.NoError(t, err)
	assert.True(t, config.ScaffoldConfigLoaded)
	assert.Nil(t, config.Scaffold)
	encoded, err := json.Marshal(config)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"scaffold"`)
}
