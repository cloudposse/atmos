package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveBuiltinAndConfiguredOverride(t *testing.T) {
	t.Chdir(t.TempDir())
	builtin, cleanup, err := Resolve("stack", "", ".", "")
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, "embedded", builtin.Source)
	require.NotEmpty(t, builtin.Files)
	require.NoError(t, os.Mkdir("recipe", 0o755))
	require.NoError(t, os.WriteFile(filepath.Join("recipe", "scaffold.yaml"), []byte("apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: stack\nspec: {}\n"), 0o644))
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("scaffold:\n  templates:\n    stack:\n      source: ./recipe\n      description: Company stack\n"), 0o644))
	configured, cleanup, err := Resolve("stack", "", ".", "")
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, "Company stack", configured.Description)
	assert.Equal(t, "./recipe", configured.Source)
	// An invalid override must never silently fall back to the built-in.
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("scaffold:\n  templates:\n    stack:\n      description: Missing source\n"), 0o644))
	_, _, err = Resolve("stack", "", ".", "")
	require.ErrorIs(t, err, errUtils.ErrInvalidTemplateData)
}

func TestResolveUnknownAndMalformedCatalog(t *testing.T) {
	t.Chdir(t.TempDir())
	_, _, err := Resolve("unknown-recipe", "", ".", "")
	require.Error(t, err)
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("scaffold:\n  templates: invalid\n"), 0o644))
	_, _, err = Resolve("stack", "", ".", "")
	require.Error(t, err)
}

func TestResolveConfiguredSourceFromConfigDir(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(project, "recipe"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, "recipe", "scaffold.yaml"), []byte("apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: stack\nspec: {}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(project, "atmos.yaml"), []byte("scaffold:\n  templates:\n    stack:\n      source: ./recipe\n      description: Company stack\n"), 0o644))
	// The CWD has neither atmos.yaml nor ./recipe.
	t.Chdir(t.TempDir())
	configured, cleanup, err := Resolve("stack", "", project, "")
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, "Company stack", configured.Description)
	require.NotEmpty(t, configured.Files)
	// Resolving from the CWD instead falls back to the built-in recipe.
	builtin, cleanup, err := Resolve("stack", "", ".", "")
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, "embedded", builtin.Source)
}

const minimalScaffold = "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: recipe\n  description: Recipe description\nspec: {}\n"

func writeRecipe(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaffold.yaml"), []byte(minimalScaffold), 0o644))
}

func TestFromConfigRejectsInvalidEntries(t *testing.T) {
	for name, tc := range map[string]struct {
		data any
		want string
	}{
		"not a map":       {data: "./recipe", want: "has invalid structure"},
		"nil entry":       {data: nil, want: "has invalid structure"},
		"missing source":  {data: map[string]any{"description": "No source"}, want: "has no `source`"},
		"non-string":      {data: map[string]any{"source": 42}, want: "has no `source`"},
		"blank source":    {data: map[string]any{"source": ""}, want: "has no `source`"},
		"list of sources": {data: []any{"./recipe"}, want: "has invalid structure"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			_, err := FromConfig("my-template", tc.data, "")
			require.ErrorIs(t, err, errUtils.ErrInvalidTemplateData)
			details := strings.Join(cockroach.GetAllDetails(err), "\n")
			assert.Contains(t, details, tc.want)
			assert.Contains(t, details, "`my-template`")
			assert.Equal(t, 2, errUtils.GetExitCode(err))
		})
	}
}

func TestFromConfigRemoteSourceIsAnUnfetchedStub(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg, err := FromConfig("remote", map[string]any{
		"source":      "github.com/cloudposse/atmos//examples/quick-start-simple?ref=main",
		"description": "Remote recipe",
		"target_dir":  "./ignored-for-stubs",
	}, "")
	require.NoError(t, err)
	assert.Equal(t, "remote", cfg.Name)
	assert.Equal(t, "github.com/cloudposse/atmos//examples/quick-start-simple?ref=main", cfg.Source)
	assert.Equal(t, "Remote recipe", cfg.Description)
	assert.Empty(t, cfg.Files, "remote sources are only fetched at generation time")
	assert.Empty(t, cfg.TargetDir)
}

func TestFromConfigLocalSourceKeepsDeclaredFields(t *testing.T) {
	t.Chdir(t.TempDir())
	writeRecipe(t, "recipe")

	cfg, err := FromConfig("local", map[string]any{"source": "./recipe", "target_dir": "./out"}, "")
	require.NoError(t, err)
	assert.Equal(t, "./recipe", cfg.Source, "the source stays as declared in atmos.yaml")
	assert.Equal(t, "./out", cfg.TargetDir)
	require.NotEmpty(t, cfg.Files)
	assert.Equal(t, "Recipe description", cfg.Description, "the recipe's own description is used when atmos.yaml has none")

	// An explicit description in atmos.yaml wins over the recipe's.
	cfg, err = FromConfig("local", map[string]any{"source": "./recipe", "description": "Overridden"}, "")
	require.NoError(t, err)
	assert.Equal(t, "Overridden", cfg.Description)
	assert.Empty(t, cfg.TargetDir)
}

func TestFromConfigReportsUnloadableLocalSource(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := FromConfig("local", map[string]any{"source": "./does-not-exist"}, "")
	require.Error(t, err)
}

func TestLocalSourceDirResolution(t *testing.T) {
	// Build real absolute paths: a rooted path without a drive letter is not absolute on Windows.
	configDir := filepath.Join(t.TempDir(), "project")
	absolute := filepath.Join(t.TempDir(), "elsewhere", "recipe")
	require.True(t, filepath.IsAbs(absolute))
	for name, tc := range map[string]struct {
		configDir, source, want string
	}{
		"relative source resolves from the config directory": {configDir, "./recipe", filepath.Join(configDir, "recipe")},
		"no config directory keeps the source":               {"", "./recipe", "./recipe"},
		"absolute source is untouched":                       {configDir, absolute, absolute},
		"non-local source is untouched":                      {configDir, "github.com/cloudposse/atmos//examples/x", "github.com/cloudposse/atmos//examples/x"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, localSourceDir(tc.configDir, tc.source))
		})
	}
}

func TestResolveFallsBackWhenConfiguredCatalogDoesNotNameTheRecipe(t *testing.T) {
	t.Chdir(t.TempDir())
	writeRecipe(t, "recipe")
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("scaffold:\n  templates:\n    other:\n      source: ./recipe\n"), 0o644))

	builtin, cleanup, err := Resolve("stack", "", ".", "")
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, "embedded", builtin.Source, "a catalog that names other recipes does not hide the built-in one")

	other, cleanup, err := Resolve("other", "", ".", "")
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, "./recipe", other.Source)
}

func TestResolveReportsUnreadableProjectConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("scaffold: [unclosed\n"), 0o644))
	_, _, err := Resolve("stack", "", ".", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "atmos.yaml")
}

func TestResolveDefaultSources(t *testing.T) {
	entries, err := templates.LoadCatalog()
	require.NoError(t, err)
	require.NotEmpty(t, entries, "the embedded catalog advertises scaffolds")

	t.Run("catalog entry", func(t *testing.T) {
		got, err := resolveDefault(entries[0].Name)
		require.NoError(t, err)
		assert.Equal(t, entries[0].Name, got.Name)
		assert.True(t, strings.HasPrefix(got.Source, entries[0].Source+"?ref="), "got %q", got.Source)
		assert.Empty(t, got.Files, "catalog entries are stubs until hydrated")
	})
	t.Run("direct source", func(t *testing.T) {
		for _, name := range []string{"github.com/cloudposse/atmos//examples/quick-start-simple", "./scaffolds/local"} {
			got, err := resolveDefault(name)
			require.NoError(t, err)
			assert.Equal(t, templates.Configuration{Name: name, Source: name}, got)
		}
	})
	t.Run("unknown name", func(t *testing.T) {
		_, err := resolveDefault("not-a-catalog-name")
		require.ErrorIs(t, err, errUtils.ErrScaffoldNotFound)
		assert.Contains(t, err.Error(), "not-a-catalog-name")
	})
}

func TestResolveWithEffectiveConfig(t *testing.T) {
	root := t.TempDir()
	writeRecipe(t, filepath.Join(root, "recipe"))
	t.Chdir(t.TempDir())
	// A neighboring atmos.yaml must never override the selected configuration.
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("scaffold:\n  templates:\n    stack:\n      source: ./missing\n"), 0o644))
	scaffold := &schema.ScaffoldConfiguration{
		Templates:  map[string]any{"Company.Stack": map[string]any{"source": "./recipe", "description": "Effective"}},
		SourceDirs: map[string]string{"Company.Stack": root},
	}
	configured, cleanup, err := ResolveWithConfig("Company.Stack", "", "", scaffold)
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, "Effective", configured.Description)
	require.NotEmpty(t, configured.Files)
	builtin, cleanup, err := ResolveWithConfig("stack", "", "", scaffold)
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, "embedded", builtin.Source)
	_, _, err = ResolveWithConfig("company.stack", "", "", scaffold)
	require.ErrorIs(t, err, errUtils.ErrScaffoldNotFound)
}
