package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.AtmosConfiguration{Scaffold: &schema.ScaffoldConfiguration{}}

func TestScaffoldConfigIncludes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		config   string
		included string
	}{
		{
			name:     "scaffold section",
			config:   "scaffold: !include ./parts/templates.yaml\n",
			included: "templates:\n  Company.Stack:\n    source: ./recipe\n    description: Included\n",
		},
		{
			name:     "templates map",
			config:   "scaffold:\n  templates: !include ./parts/templates.yaml\n",
			included: "Company.Stack:\n  source: ./recipe\n  description: Included\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			main := filepath.Join(root, "company.yaml")
			included := filepath.Join(filepath.Dir(main), "parts", "templates.yaml")
			require.NoError(t, os.MkdirAll(filepath.Dir(included), 0o755))
			require.NoError(t, os.WriteFile(main, []byte(tc.config), 0o644))
			require.NoError(t, os.WriteFile(included, []byte(tc.included), 0o644))
			config, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigFilesFromArg: []string{main}})
			require.NoError(t, err)
			require.NotNil(t, config.Scaffold)
			assert.Equal(t, map[string]any{
				"Company.Stack": map[string]any{"source": "./recipe", "description": "Included"},
			}, config.Scaffold.Templates)
			assert.Equal(t, filepath.Dir(main), config.Scaffold.SourceDirs["Company.Stack"])
		})
	}
}

func TestScaffoldIncludedTemplatesFromImportsAndProfiles(t *testing.T) {
	setupTestAdapters()
	root := t.TempDir()
	t.Chdir(root)
	imported := filepath.Join(root, "catalog", "templates.yaml")
	templates := filepath.Join(root, "catalog", "entries.yaml")
	profile := filepath.Join(root, "profiles", "company", "atmos.yaml")
	main := filepath.Join(root, "company.yaml")
	for _, path := range []string{imported, profile} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	}
	require.NoError(t, os.WriteFile(imported, []byte("scaffold:\n  templates: !include "+strconv.Quote(filepath.ToSlash(templates))+"\n"), 0o644))
	require.NoError(t, os.WriteFile(templates, []byte("Company.Stack:\n  source: ./recipe\n  description: Imported\n"), 0o644))
	require.NoError(t, os.WriteFile(main, []byte("profiles:\n  base_path: ./profiles\nimport:\n  - catalog/templates.yaml\n"), 0o644))
	require.NoError(t, os.WriteFile(profile, []byte("scaffold:\n  templates:\n    Company.Stack:\n      description: !env ATMOS_TEST_SCAFFOLD_DESCRIPTION\n    ProfileOnly:\n      source: ./local-recipe\n"), 0o644))
	t.Setenv("ATMOS_TEST_SCAFFOLD_DESCRIPTION", "Profile")
	config, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigFilesFromArg: []string{main}, ProfilesFromArg: []string{"company"}})
	require.NoError(t, err)
	require.NotNil(t, config.Scaffold)
	assert.Equal(t, map[string]any{"source": "./recipe", "description": "Profile"}, config.Scaffold.Templates["Company.Stack"])
	assert.Equal(t, filepath.Dir(imported), config.Scaffold.SourceDirs["Company.Stack"])
	assert.Equal(t, map[string]any{"source": "./local-recipe"}, config.Scaffold.Templates["ProfileOnly"])
	assert.Equal(t, filepath.Dir(profile), config.Scaffold.SourceDirs["ProfileOnly"])
}

func TestResolveScaffoldConfigSectionValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "empty source"},
		{name: "null source", content: "null\n"},
		{name: "invalid root mapping", content: "[invalid]\n", wantErr: "cannot unmarshal"},
		{name: "missing section", content: "env:\n  UNRELATED: !unsupported ignored\n"},
		{name: "null section", content: "scaffold: null\n"},
		{name: "empty section", content: "scaffold: {}\n"},
		{name: "null templates", content: "scaffold:\n  templates: null\n"},
		{name: "scalar section", content: "scaffold: invalid\n", wantErr: "scaffold must be a mapping"},
		{name: "scalar templates", content: "scaffold:\n  templates: invalid\n", wantErr: "scaffold.templates must be a mapping"},
		{name: "sequence templates", content: "scaffold:\n  templates: [invalid]\n", wantErr: "scaffold.templates must be a mapping"},
		{name: "recursive alias", content: "scaffold: &recursive\n  templates: *recursive\n", wantErr: "contains itself"},
		{name: "invalid merge", content: "scaffold:\n  templates:\n    <<: invalid\n", wantErr: "map or sequence"},
		{name: "duplicate template", content: "scaffold:\n  templates:\n    Company: {}\n    Company: {}\n", wantErr: "already defined"},
		{name: "invalid YAML", content: "scaffold: [\n", wantErr: "yaml:"},
		{name: "missing include", content: "scaffold: !include ./missing.yaml\n", wantErr: "missing.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "company.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o644))
			v := viper.New()
			v.SetConfigFile(path)
			config := &schema.AtmosConfiguration{}
			err := resolveScaffoldConfig(v, config)
			if tc.wantErr != "" {
				require.ErrorIs(t, err, errUtils.ErrInvalidScaffoldConfig)
				assert.ErrorContains(t, err, tc.wantErr)
				assert.ErrorContains(t, err, path)
				assert.False(t, config.ScaffoldConfigLoaded)
				return
			}
			require.NoError(t, err)
			assert.Nil(t, config.Scaffold)
			assert.True(t, config.ScaffoldConfigLoaded)
		})
	}
}

func TestResolveScaffoldConfigIncludeUsesDeclaringDirectory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	main := filepath.Join(root, "catalog", "company.yaml")
	included := filepath.Join(filepath.Dir(main), "parts", "templates.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(included), 0o755))
	require.NoError(t, os.WriteFile(main, []byte("env:\n  UNRELATED: !unsupported ignored\nscaffold:\n  templates: !include ./parts/templates.yaml\n"), 0o644))
	require.NoError(t, os.WriteFile(included, []byte("Company.Stack:\n  source: ./recipe\n"), 0o644))
	v := viper.New()
	v.SetConfigFile(main)
	config := &schema.AtmosConfiguration{}
	require.NoError(t, resolveScaffoldConfig(v, config))
	require.NotNil(t, config.Scaffold)
	assert.Equal(t, map[string]any{"Company.Stack": map[string]any{"source": "./recipe"}}, config.Scaffold.Templates)
	assert.Equal(t, filepath.Dir(main), config.Scaffold.SourceDirs["Company.Stack"])
}

func TestResolveScaffoldConfigYAMLAliasesAndMerges(t *testing.T) {
	t.Setenv("ATMOS_TEST_SCAFFOLD_DESCRIPTION", "Resolved")
	for _, tc := range []struct {
		name    string
		content string
	}{
		{
			name:    "root merge",
			content: "base: &base\n  scaffold:\n    templates:\n      Company.Stack:\n        source: ./recipe\n        description: Resolved\n<<: *base\n",
		},
		{
			name:    "section alias with function",
			content: "base: &base\n  templates:\n    Company.Stack:\n      source: ./recipe\n      description: !env ATMOS_TEST_SCAFFOLD_DESCRIPTION\nscaffold: *base\n",
		},
		{
			name:    "templates merge with explicit override",
			content: "base: &base\n  Company.Stack:\n    source: ./wrong\nscaffold:\n  templates:\n    Company.Stack:\n      source: ./recipe\n      description: Resolved\n    <<: *base\n",
		},
		{
			name:    "entry merge sequence uses first value",
			content: "first: &first\n  source: ./recipe\n  description: First\nsecond: &second\n  source: ./wrong\nscaffold:\n  templates:\n    Company.Stack:\n      <<: [*first, *second]\n      description: !env ATMOS_TEST_SCAFFOLD_DESCRIPTION\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "company.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o644))
			v := viper.New()
			v.SetConfigFile(path)
			config := &schema.AtmosConfiguration{}
			require.NoError(t, resolveScaffoldConfig(v, config))
			require.NotNil(t, config.Scaffold)
			assert.Equal(t, map[string]any{"Company.Stack": map[string]any{
				"source": "./recipe", "description": "Resolved",
			}}, config.Scaffold.Templates)
			assert.Equal(t, filepath.Dir(path), config.Scaffold.SourceDirs["Company.Stack"])
		})
	}
}

func TestScaffoldTemplateSequenceAliasesPreserveOrder(t *testing.T) {
	t.Setenv("ATMOS_TEST_SCAFFOLD_OPTION", "second")
	templates, err := decodeScaffoldTemplates(mergedConfigSource{
		path: filepath.Join(t.TempDir(), "company.yaml"),
		content: "option: &option !env ATMOS_TEST_SCAFFOLD_OPTION\nscaffold:\n  templates:\n    Company.Stack:\n" +
			"      source: ./recipe\n      vars:\n        options: [first, *option, third]\n",
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"Company.Stack": map[string]any{
		"source": "./recipe",
		"vars":   map[string]any{"options": []any{"first", "second", "third"}},
	}}, templates)
}

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
	setupTestAdapters()
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
