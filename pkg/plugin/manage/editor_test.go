package manage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
)

const commentedConfig = `# Top comment: keep me
base_path: "."   # inline comment on base_path
stacks:
  base_path: "stacks"
  included_paths: ["**/*"]
  name_template: "{{ .vars.stage }}"

# Plugins section comment
plugins:
  packages:
    # comment about one
    one:
      source: oci://SOURCE   # inline source comment
      version: ^0.1

# Trailing comment: keep me
`

func TestDeclarationEditsPreserveBlockStyleCommentsAndBlankLines(t *testing.T) {
	manager, source, _ := setup(t)
	registryPath := source[len("oci://"):]
	config := []byte(replaceSource(commentedConfig, registryPath))
	configPath := filepath.Join(manager.Project, manager.ConfigFile)
	require.NoError(t, os.WriteFile(configPath, config, 0o644))
	reloadConfiguration(t, manager)

	// Changing the constraint of an existing package edits one scalar and nothing else.
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "one", Source: source, Constraint: "^1"}))
	updated, err := os.ReadFile(configPath)
	require.NoError(t, err)
	want := replaceSource(commentedConfig, registryPath)
	want = replaceOnce(want, "version: ^0.1", `version: ^1`)
	assert.Equal(t, want, string(updated))
	assert.NotContains(t, string(updated), `{"source"`, "declarations must stay in block style")

	// Removing the only package keeps the surrounding structure and comments valid.
	reloadConfiguration(t, manager)
	require.NoError(t, manager.Remove(t.Context(), "one"))
	removed, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(removed), "  packages: {}\n")
	for _, fragment := range []string{"# Top comment: keep me", "   # inline comment on base_path", "\n\n# Plugins section comment\nplugins:", "\n\n# Trailing comment: keep me\n"} {
		assert.Contains(t, string(removed), fragment)
	}

	// A new package is written in block style beside the comments.
	reloadConfiguration(t, manager)
	require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "two", Source: source, Constraint: "~1.0"}))
	added, err := os.ReadFile(configPath)
	require.NoError(t, err)
	want = replaceSource(commentedConfig, registryPath)
	want = replaceOnce(want, "    # comment about one\n    one:", "    two:")
	want = replaceOnce(want, "   # inline source comment", "")
	want = replaceOnce(want, "version: ^0.1", "version: ~1.0")
	assert.Equal(t, want, string(added))
}

func replaceSource(text, source string) string { return replaceOnce(text, "SOURCE", source) }

func replaceOnce(text, old, replacement string) string {
	index := indexOf(text, old)
	if index < 0 {
		return text
	}
	return text[:index] + replacement + text[index+len(old):]
}

func indexOf(text, fragment string) int {
	for index := 0; index+len(fragment) <= len(text); index++ {
		if text[index:index+len(fragment)] == fragment {
			return index
		}
	}
	return -1
}

func TestFailedDeclarationReadPreservesExistingReleaseLock(t *testing.T) {
	for _, operation := range []string{"add", "remove"} {
		t.Run(operation, func(t *testing.T) {
			manager, source, _ := setup(t)
			require.NoError(t, manager.Add(t.Context(), manage.AddOptions{Alias: "fixture", Source: source, Constraint: "1.0.0"}))
			before, err := os.ReadFile(pluginlock.Path(manager.Project))
			require.NoError(t, err)
			configPath := filepath.Join(manager.Project, manager.ConfigFile)
			require.NoError(t, os.Remove(configPath))
			require.NoError(t, os.Mkdir(configPath, 0o700))
			if operation == "add" {
				err = manager.Add(t.Context(), manage.AddOptions{Alias: "fixture", Source: source, Constraint: "^1.0.0"})
			} else {
				err = manager.Remove(t.Context(), "fixture")
			}
			require.Error(t, err)
			after, err := os.ReadFile(pluginlock.Path(manager.Project))
			require.NoError(t, err)
			require.Equal(t, before, after, "a failed declaration edit cannot commit a changed lock")
			require.DirExists(t, configPath)
		})
	}
}

func TestMalformedLockBlocksAllProjectOperationsWithoutChangingConfiguration(t *testing.T) {
	manager, source, _ := setup(t)
	broken := []byte("not: [valid yaml")
	require.NoError(t, os.WriteFile(pluginlock.Path(manager.Project), broken, 0o600))
	configPath := filepath.Join(manager.Project, manager.ConfigFile)
	before, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Error(t, manager.Add(t.Context(), manage.AddOptions{Alias: "fixture", Source: source}))
	require.Error(t, manager.Remove(t.Context(), "fixture"))
	require.Error(t, manager.Restore(t.Context(), manage.RestoreOptions{}))
	_, err = manage.List(manager.Config, manager.Project)
	require.Error(t, err)
	_, err = manage.Info(manager.Config, manager.Project, "fixture")
	require.Error(t, err)
	after, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, before, after)
	locked, err := os.ReadFile(pluginlock.Path(manager.Project))
	require.NoError(t, err)
	require.Equal(t, broken, locked, "invalid locks must not be silently reset")
}
