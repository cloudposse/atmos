package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// TestFileSpecByPath_GlobMatchesEveryFileUnderDirectory verifies a single
// spec.files[] entry with a "**" glob path resolves against every discovered
// file nested under that directory, not just a literal match.
func TestFileSpecByPath_GlobMatchesEveryFileUnderDirectory(t *testing.T) {
	scaffoldYAML := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  files:
    - path: "docs/legacy/**"
      when: "answers.include_legacy_docs == true"
`
	scaffoldConfig, err := config.LoadScaffoldConfigFromContent(scaffoldYAML)
	require.NoError(t, err)

	files := []templates.File{
		{Path: "docs/legacy/intro.md", Content: "intro"},
		{Path: "docs/legacy/nested/deep.md", Content: "deep"},
		{Path: "docs/current.md", Content: "current"},
	}

	specByPath := FileSpecByPath(scaffoldConfig, files)

	assert.Equal(t, "docs/legacy/**", specByPath["docs/legacy/intro.md"].Path)
	assert.Equal(t, "docs/legacy/**", specByPath["docs/legacy/nested/deep.md"].Path)
	_, matched := specByPath["docs/current.md"]
	assert.False(t, matched, "a file outside the glob's directory must not get a spec")
}

// TestFileSpecByPath_LastMatchWins verifies that when two spec.files[]
// entries both match the same discovered file, the *last* one declared
// wins -- mirroring .gitignore/CODEOWNERS precedence, and letting an
// author place a broad glob first and a specific override after it.
func TestFileSpecByPath_LastMatchWins(t *testing.T) {
	scaffoldYAML := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  files:
    - path: "docs/legacy/**"
      when: "answers.include_legacy_docs == true"
    - path: "docs/legacy/keep-this.md"
      when: "always"
`
	scaffoldConfig, err := config.LoadScaffoldConfigFromContent(scaffoldYAML)
	require.NoError(t, err)

	files := []templates.File{
		{Path: "docs/legacy/keep-this.md", Content: "keep"},
	}

	specByPath := FileSpecByPath(scaffoldConfig, files)

	assert.Equal(t, "docs/legacy/keep-this.md", specByPath["docs/legacy/keep-this.md"].Path,
		"the later, more specific entry must win over the earlier broad glob")
}

// TestExecuteWithSetup_FilesGlobWhenSkipsDirectoryRecursively verifies a
// single glob path: + when: entry skips every file nested under that
// directory, at any depth, without listing each file individually.
func TestExecuteWithSetup_FilesGlobWhenSkipsDirectoryRecursively(t *testing.T) {
	ui := createTestUI(t)
	tempDir := t.TempDir()

	scaffoldYAML := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  files:
    - path: "docs/legacy/**"
      when: "answers.include_legacy_docs == true"
`

	embedsConfig := &templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: scaffoldYAML, Permissions: 0o644},
			{Path: "docs/legacy/intro.md", Content: "intro", Permissions: 0o644},
			{Path: "docs/legacy/nested/deep.md", Content: "deep", Permissions: 0o644},
			{Path: "docs/current.md", Content: "current", Permissions: 0o644},
		},
	}

	cmdTemplateValues := map[string]interface{}{"include_legacy_docs": false}
	err := ui.executeWithSetup(embedsConfig, tempDir, false, false, true, "", cmdTemplateValues, []string{"{{", "}}"})
	require.NoError(t, err)

	for _, skipped := range []string{
		filepath.Join("docs", "legacy", "intro.md"),
		filepath.Join("docs", "legacy", "nested", "deep.md"),
	} {
		_, statErr := os.Stat(filepath.Join(tempDir, skipped))
		assert.True(t, os.IsNotExist(statErr), "expected %s to be skipped recursively", skipped)
	}

	_, statErr := os.Stat(filepath.Join(tempDir, "docs", "current.md"))
	assert.NoError(t, statErr, "a file outside the glob's directory must still be generated")
}

// TestExecuteWithSetup_FilesGlobMatrixDuplicatesDirectory verifies a single
// glob path: + matrix: + target: entry duplicates every file nested under
// that directory once per matrix combination, with .file.RelPath in target:
// preserving each matched file's own relative position and .matrix.<axis>
// available in content exactly like a single-file matrix entry.
func TestExecuteWithSetup_FilesGlobMatrixDuplicatesDirectory(t *testing.T) {
	ui := createTestUI(t)
	tempDir := t.TempDir()

	scaffoldYAML := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  files:
    - path: "components/**"
      target: "environments/{{ .matrix.env }}/{{ .file.RelPath }}"
      matrix:
        env: [dev, staging]
`

	embedsConfig := &templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: scaffoldYAML, Permissions: 0o644},
			{
				Path:        "components/vpc/main.tf",
				Content:     "env = \"{{ .matrix.env }}\"\nsource = \"{{ .file.RelPath }}\"\n",
				IsTemplate:  true,
				Permissions: 0o644,
			},
			{
				Path:        "components/eks/main.tf",
				Content:     "env = \"{{ .matrix.env }}\"\nsource = \"{{ .file.RelPath }}\"\n",
				IsTemplate:  true,
				Permissions: 0o644,
			},
		},
	}

	err := ui.executeWithSetup(embedsConfig, tempDir, false, false, true, "", nil, []string{"{{", "}}"})
	require.NoError(t, err)

	// The source's own path is consumed by the directory-level matrix, never
	// written verbatim.
	for _, sourcePath := range []string{"components/vpc/main.tf", "components/eks/main.tf"} {
		_, statErr := os.Stat(filepath.Join(tempDir, sourcePath))
		assert.True(t, os.IsNotExist(statErr))
	}

	for _, tc := range []struct {
		relPath string
		want    string
	}{
		{filepath.Join("environments", "dev", "vpc", "main.tf"), "env = \"dev\"\nsource = \"vpc/main.tf\"\n"},
		{filepath.Join("environments", "dev", "eks", "main.tf"), "env = \"dev\"\nsource = \"eks/main.tf\"\n"},
		{filepath.Join("environments", "staging", "vpc", "main.tf"), "env = \"staging\"\nsource = \"vpc/main.tf\"\n"},
		{filepath.Join("environments", "staging", "eks", "main.tf"), "env = \"staging\"\nsource = \"eks/main.tf\"\n"},
	} {
		content, readErr := os.ReadFile(filepath.Join(tempDir, tc.relPath))
		require.NoError(t, readErr, "expected %s to be generated", tc.relPath)
		assert.Equal(t, tc.want, string(content))
	}
}

// TestExecuteWithSetup_FilesGlobMatrixWithoutFileDifferentiationCollides
// verifies that a directory-level matrix entry whose target: doesn't
// differentiate matched files (no .file.Path/.file.RelPath reference) fails
// with the existing global duplicate-output-path guard rather than silently
// letting the second matched file overwrite the first's output -- proving
// this pre-existing guard covers the new directory-glob footgun with no new
// validation code.
func TestExecuteWithSetup_FilesGlobMatrixWithoutFileDifferentiationCollides(t *testing.T) {
	ui := createTestUI(t)
	tempDir := t.TempDir()

	scaffoldYAML := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  files:
    - path: "components/**"
      target: "environments/{{ .matrix.env }}/main.tf"
      matrix:
        env: [dev]
`

	embedsConfig := &templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: scaffoldYAML, Permissions: 0o644},
			{Path: "components/vpc/main.tf", Content: "vpc", Permissions: 0o644},
			{Path: "components/eks/main.tf", Content: "eks", Permissions: 0o644},
		},
	}

	err := ui.executeWithSetup(embedsConfig, tempDir, false, false, true, "", nil, []string{"{{", "}}"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldDuplicateOutputPath)
}
