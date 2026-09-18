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
// up front, before any file is written -- not only once the second matched
// file's write collides with the first's, which would otherwise leave a
// partial, half-generated project on disk (the first matched file's output
// would already exist even though the overall command reports failure).
// validateDirectoryMatrixTargetsDifferentiate catches this deterministically
// (not just probabilistically) because .file.* is the only template data
// that varies per matched file for a given resolved combination.
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
	assert.ErrorIs(t, err, errUtils.ErrScaffoldMatrixTargetMissingFileContext)

	// Zero writes -- not even the first matched file's output -- since the
	// check runs before the file-processing loop starts.
	_, statErr := os.Stat(filepath.Join(tempDir, "environments"))
	assert.True(t, os.IsNotExist(statErr), "no output should be written when the upfront check rejects the scaffold")
}

// TestExecuteWithSetup_FilesGlobMatrixWithFileRelPathAvoidsCollision is the
// positive counterpart: the same two matched files, but target: references
// .file.RelPath, so validateDirectoryMatrixTargetsDifferentiate must not
// reject it, and generation must succeed.
func TestExecuteWithSetup_FilesGlobMatrixWithFileRelPathAvoidsCollision(t *testing.T) {
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
	require.NoError(t, err)

	for _, want := range []string{
		filepath.Join("environments", "dev", "vpc", "main.tf"),
		filepath.Join("environments", "dev", "eks", "main.tf"),
	} {
		_, statErr := os.Stat(filepath.Join(tempDir, want))
		assert.NoError(t, statErr, "expected %s to be generated", want)
	}
}

// TestExecuteWithSetup_FilesGlobMatrixWithLiteralDotFileTextCollides is a
// regression test for a real bug: validateDirectoryMatrixTargetsDifferentiate
// used to accept any target: containing the raw substring ".file." anywhere,
// even outside a template action. Here target: renders to a literal output
// filename containing ".file." as plain text (not a genuine .file.Path/
// .file.RelPath reference), so it must still be rejected up front -- exactly
// like the no-differentiation case -- instead of silently passing the check
// and colliding on the first two matched files' writes.
func TestExecuteWithSetup_FilesGlobMatrixWithLiteralDotFileTextCollides(t *testing.T) {
	ui := createTestUI(t)
	tempDir := t.TempDir()

	scaffoldYAML := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  files:
    - path: "components/**"
      target: "environments/{{ .matrix.env }}/output.file.txt"
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
	assert.ErrorIs(t, err, errUtils.ErrScaffoldMatrixTargetMissingFileContext)

	// Zero writes -- not even the first matched file's output -- since the
	// check runs before the file-processing loop starts.
	_, statErr := os.Stat(filepath.Join(tempDir, "environments"))
	assert.True(t, os.IsNotExist(statErr), "no output should be written when the upfront check rejects the scaffold")
}

// TestExecuteWithSetup_FilesGlobMatrixWithInvalidFileFieldCollides is another
// regression case for the same substring-match bug: target: references
// ".file.Unknown", which lexically contains ".file." but is not one of
// FileContext's two real fields (Path, RelPath) and can never actually
// differentiate matched files. The upfront check must reject this the same
// way it rejects a target with no .file reference at all.
func TestExecuteWithSetup_FilesGlobMatrixWithInvalidFileFieldCollides(t *testing.T) {
	ui := createTestUI(t)
	tempDir := t.TempDir()

	scaffoldYAML := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  files:
    - path: "components/**"
      target: "environments/{{ .matrix.env }}/{{ .file.Unknown }}"
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
	assert.ErrorIs(t, err, errUtils.ErrScaffoldMatrixTargetMissingFileContext)

	_, statErr := os.Stat(filepath.Join(tempDir, "environments"))
	assert.True(t, os.IsNotExist(statErr), "no output should be written when the upfront check rejects the scaffold")
}

// TestProcessFileEntry_MatrixExpansionCachedPerSpecPath is a regression test
// for a real bug: without caching, a directory-level glob entry matching N
// files calls engine.ExpandMatrix N independent times with identical inputs
// -- for a non-deterministic axis expression (e.g. Sprig's randAlphaNum),
// this resolves a *different* value per matched file for what should be one
// shared combination. Proven deterministically here (no reliance on actual
// randomness): the first matched file populates matrixExpansions[spec.Path];
// mergedValues is then mutated to a value that would resolve differently if
// re-expanded; the second matched file must still land under the FIRST
// call's cached resolution, not the mutated one.
func TestProcessFileEntry_MatrixExpansionCachedPerSpecPath(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	scaffoldConfig := &config.ScaffoldConfig{}
	spec := config.FileSpec{
		Path:   "components/**",
		Target: "out/{{ .matrix.env }}/{{ .file.RelPath }}",
		Matrix: map[string]any{"env": "answers.environments"},
	}

	fileA := templates.File{Path: "components/a/main.tf", Content: "a", Permissions: 0o644}
	fileB := templates.File{Path: "components/b/main.tf", Content: "b", Permissions: 0o644}

	mergedValues := map[string]interface{}{"environments": []interface{}{"dev"}}
	matrixExpansions := make(map[string]matrixExpansionResult)
	seenRenderedPaths := make(map[string]string)

	_, _, _, err := ui.processFileEntry(fileA, spec, targetDir, false, false, scaffoldConfig, mergedValues, []string{"{{", "}}"}, seenRenderedPaths, matrixExpansions)
	require.NoError(t, err)

	cached, ok := matrixExpansions[spec.Path]
	require.True(t, ok, "expected the first matched file to populate the cache")
	require.Len(t, cached.rows, 1)
	require.Equal(t, "dev", cached.rows[0]["env"])

	// Mutate mergedValues to a value that would resolve differently if
	// ExpandMatrix were recomputed for the second matched file.
	mergedValues["environments"] = []interface{}{"staging"}

	_, _, _, err = ui.processFileEntry(fileB, spec, targetDir, false, false, scaffoldConfig, mergedValues, []string{"{{", "}}"}, seenRenderedPaths, matrixExpansions)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(targetDir, "out", "dev", "b", "main.tf"))
	require.NoError(t, readErr, `the second matched file must land under the first call's cached "dev" row, not a recomputed "staging"`)
	assert.Equal(t, "b", string(content))

	_, statErr := os.Stat(filepath.Join(targetDir, "out", "staging"))
	assert.True(t, os.IsNotExist(statErr), `no output should exist under the recomputed, uncached "staging" value`)
}

// TestExecuteWithSetup_FilesGlobBackslashPatternMatchesLikeForwardSlash is a
// regression test: a backslash-authored path: pattern used to silently
// never match any discovered file on any OS other than the one whose own
// path separator happens to be backslash (i.e. it only worked on Windows),
// because filepath.ToSlash only normalizes the *host* OS's own separator.
// Discovered file paths are always forward-slash regardless of OS, so a
// pattern authored with backslashes on Windows would silently fail to gate
// anything when the same scaffold ran on macOS/Linux (or vice versa).
func TestExecuteWithSetup_FilesGlobBackslashPatternMatchesLikeForwardSlash(t *testing.T) {
	ui := createTestUI(t)
	tempDir := t.TempDir()

	scaffoldYAML := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  files:
    - path: 'docs\legacy\**'
      when: "never"
`

	embedsConfig := &templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: scaffoldYAML, Permissions: 0o644},
			{Path: "docs/legacy/a.md", Content: "legacy", Permissions: 0o644},
		},
	}

	err := ui.executeWithSetup(embedsConfig, tempDir, false, false, true, "", nil, []string{"{{", "}}"})
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(tempDir, "docs", "legacy", "a.md"))
	assert.True(t, os.IsNotExist(statErr), "the backslash-authored pattern must match and skip docs/legacy/a.md, same as its forward-slash equivalent would")
}
