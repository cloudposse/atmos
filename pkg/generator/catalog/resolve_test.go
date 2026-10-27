package catalog

import (
	"os"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
