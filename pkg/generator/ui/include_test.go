package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/generator/templates"
)

// TestExecuteWithSetup_IncludedLocalFileExcludedFromOutput proves a file
// consumed by a computed field's !include (a local reference table that
// exists solely to be included) is excluded from generation output, while
// an unrelated file in the same template still generates normally -- see
// config.WithIncludedPaths and ui.go's includedSet check.
func TestExecuteWithSetup_IncludedLocalFileExcludedFromOutput(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	// !include's local resolution needs a real source directory, unlike the
	// rest of this package's in-memory-only Configuration fixtures.
	sourceDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "lib"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "lib", "licenses.yaml"), []byte("MIT: {}\n"), 0o600))

	configuration := &templates.Configuration{
		Name:   "include-exclusion",
		Source: sourceDir,
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: include-exclusion
spec:
  fields:
    - name: license_lookup
      type: computed
      value: !include ./lib/licenses.yaml
  files:
    - path: lib/licenses.yaml
    - path: output.txt
`, Permissions: 0o644},
			{Path: "lib/licenses.yaml", Content: "MIT: {}\n", Permissions: 0o644},
			{Path: "output.txt", Content: "lookup: {{ .Config.license_lookup }}\n", IsTemplate: true, Permissions: 0o644},
		},
	}

	err := ui.executeWithSetup(configuration, targetDir, false, false, true, "", map[string]interface{}{}, []string{"{{", "}}"})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(targetDir, "lib", "licenses.yaml"))
	assert.True(t, os.IsNotExist(err), "the !include-consumed file must not be generated as output")

	generated, err := os.ReadFile(filepath.Join(targetDir, "output.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(generated), "lookup: map[MIT:map[]]")
}

// TestExecuteWithSetup_AbsoluteIncludedLocalFileExcludedFromOutput proves the
// same exclusion holds when scaffold.yaml's !include argument is an absolute
// path inside the template's own source directory rather than a "./"-relative
// one. The findLocalFile helper resolves such a path directly, but
// config.WithIncludedPaths records the absolute argument unchanged --
// includedPathSet must rebase it onto the source directory before comparing
// against tmpl.File.Path (which is always relative), or the consumed file
// leaks into project output. See includedPathSet's own doc comment.
func TestExecuteWithSetup_AbsoluteIncludedLocalFileExcludedFromOutput(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	sourceDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "lib"), 0o755))
	licensesPath := filepath.Join(sourceDir, "lib", "licenses.yaml")
	require.NoError(t, os.WriteFile(licensesPath, []byte("MIT: {}\n"), 0o600))

	configuration := &templates.Configuration{
		Name:   "include-exclusion-absolute",
		Source: sourceDir,
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: include-exclusion-absolute
spec:
  fields:
    - name: license_lookup
      type: computed
      value: !include ` + filepath.ToSlash(licensesPath) + `
  files:
    - path: lib/licenses.yaml
    - path: output.txt
`, Permissions: 0o644},
			{Path: "lib/licenses.yaml", Content: "MIT: {}\n", Permissions: 0o644},
			{Path: "output.txt", Content: "lookup: {{ .Config.license_lookup }}\n", IsTemplate: true, Permissions: 0o644},
		},
	}

	err := ui.executeWithSetup(configuration, targetDir, false, false, true, "", map[string]interface{}{}, []string{"{{", "}}"})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(targetDir, "lib", "licenses.yaml"))
	assert.True(t, os.IsNotExist(err), "the absolute !include-consumed file must not be generated as output")

	generated, err := os.ReadFile(filepath.Join(targetDir, "output.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(generated), "lookup: map[MIT:map[]]")
}
