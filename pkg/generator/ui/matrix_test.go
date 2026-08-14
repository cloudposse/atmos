package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/condition"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// matrixScaffoldYAML is a minimal template declaring one matrix file entry:
// a single source (deploy.yaml) expanded into one output per resolved
// environment/region combination, pruned by when: to only the combinations
// answers.regions_by_env allows.
const matrixScaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  files:
    - path: deploy.yaml
      target: "deploy/{{ .matrix.environment }}/{{ .matrix.region }}.yaml"
      matrix:
        environment: [dev, staging, production]
        region: [us-east-1, us-west-2]
      when: "matrix.region in answers.regions_by_env[matrix.environment]"
`

func matrixEmbedsConfig() *templates.Configuration {
	return &templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: matrixScaffoldYAML, Permissions: 0o644},
			{
				Path:        "deploy.yaml",
				Content:     "environment: {{ .matrix.environment }}\nregion: {{ .matrix.region }}\n",
				IsTemplate:  true,
				Permissions: 0o644,
			},
		},
	}
}

func TestExecuteWithSetup_FilesMatrixExpansion(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	cmdTemplateValues := map[string]interface{}{
		"regions_by_env": map[string]interface{}{
			"dev":        []interface{}{"us-east-1"},
			"staging":    []interface{}{"us-east-1"},
			"production": []interface{}{"us-east-1", "us-west-2"},
		},
	}

	err := ui.executeWithSetup(matrixEmbedsConfig(), targetDir, false, false, true, "", cmdTemplateValues, []string{"{{", "}}"})
	require.NoError(t, err)

	// The source's own path is consumed by matrix, never written verbatim.
	_, statErr := os.Stat(filepath.Join(targetDir, "deploy.yaml"))
	assert.True(t, os.IsNotExist(statErr))

	for _, tc := range []struct {
		relPath string
		want    string
	}{
		{filepath.Join("deploy", "dev", "us-east-1.yaml"), "environment: dev\nregion: us-east-1\n"},
		{filepath.Join("deploy", "staging", "us-east-1.yaml"), "environment: staging\nregion: us-east-1\n"},
		{filepath.Join("deploy", "production", "us-east-1.yaml"), "environment: production\nregion: us-east-1\n"},
		{filepath.Join("deploy", "production", "us-west-2.yaml"), "environment: production\nregion: us-west-2\n"},
	} {
		content, readErr := os.ReadFile(filepath.Join(targetDir, tc.relPath))
		require.NoError(t, readErr, "expected %s to be generated", tc.relPath)
		assert.Equal(t, tc.want, string(content))
	}

	// dev/us-west-2 and staging/us-west-2 are pruned by when:, so they must
	// never be written.
	for _, relPath := range []string{
		filepath.Join("deploy", "dev", "us-west-2.yaml"),
		filepath.Join("deploy", "staging", "us-west-2.yaml"),
	} {
		_, statErr := os.Stat(filepath.Join(targetDir, relPath))
		assert.True(t, os.IsNotExist(statErr), "expected %s to be pruned", relPath)
	}
}

// computedAxisScaffoldYAML replicates the exact matrix shape discussed in
// https://github.com/orgs/cloudposse/discussions/126: axes computed from
// nested/structured answer data (every environment, and every region used by
// any environment) via the "collectKeys" template function, pruned by when:
// to only the environment/region pairs that actually apply.
const computedAxisScaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  files:
    - path: deploy.yaml
      target: "deploy/{{ .matrix.environment }}/{{ .matrix.region }}.yaml"
      matrix:
        environment: '{{ collectKeys answers.environments }}'
        region: '{{ collectKeys answers.environments "regions" }}'
      when: "matrix.region in answers.environments[matrix.environment].regions"
`

func computedAxisEmbedsConfig() *templates.Configuration {
	return &templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: computedAxisScaffoldYAML, Permissions: 0o644},
			{
				Path:        "deploy.yaml",
				Content:     "environment: {{ .matrix.environment }}\nregion: {{ .matrix.region }}\n",
				IsTemplate:  true,
				Permissions: 0o644,
			},
		},
	}
}

func TestExecuteWithSetup_FilesMatrixComputedAxesFromNestedAnswers(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	cmdTemplateValues := map[string]interface{}{
		"environments": map[string]interface{}{
			"dev": map[string]interface{}{
				"regions": map[string]interface{}{"us-east-1": map[string]interface{}{}},
			},
			"staging": map[string]interface{}{
				"regions": map[string]interface{}{"us-east-1": map[string]interface{}{}},
			},
			"production": map[string]interface{}{
				"regions": map[string]interface{}{
					"us-east-1": map[string]interface{}{},
					"us-west-2": map[string]interface{}{},
				},
			},
		},
	}

	err := ui.executeWithSetup(computedAxisEmbedsConfig(), targetDir, false, false, true, "", cmdTemplateValues, []string{"{{", "}}"})
	require.NoError(t, err)

	for _, tc := range []struct {
		relPath string
		want    string
	}{
		{filepath.Join("deploy", "dev", "us-east-1.yaml"), "environment: dev\nregion: us-east-1\n"},
		{filepath.Join("deploy", "staging", "us-east-1.yaml"), "environment: staging\nregion: us-east-1\n"},
		{filepath.Join("deploy", "production", "us-east-1.yaml"), "environment: production\nregion: us-east-1\n"},
		{filepath.Join("deploy", "production", "us-west-2.yaml"), "environment: production\nregion: us-west-2\n"},
	} {
		content, readErr := os.ReadFile(filepath.Join(targetDir, tc.relPath))
		require.NoError(t, readErr, "expected %s to be generated", tc.relPath)
		assert.Equal(t, tc.want, string(content))
	}

	// dev/us-west-2 and staging/us-west-2 are pruned by when:, even though
	// the region axis (computed across every environment) includes
	// us-west-2 for the production/us-west-2 combination above.
	for _, relPath := range []string{
		filepath.Join("deploy", "dev", "us-west-2.yaml"),
		filepath.Join("deploy", "staging", "us-west-2.yaml"),
	} {
		_, statErr := os.Stat(filepath.Join(targetDir, relPath))
		assert.True(t, os.IsNotExist(statErr), "expected %s to be pruned", relPath)
	}
}

// whitespaceAxisScaffoldYAML declares a single computed axis (environment)
// over answers.environments, whose keys include one containing whitespace.
const whitespaceAxisScaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  files:
    - path: deploy.yaml
      target: "deploy/{{ .matrix.environment }}.yaml"
      matrix:
        environment: '{{ collectKeys answers.environments }}'
`

func whitespaceAxisEmbedsConfig() *templates.Configuration {
	return &templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: whitespaceAxisScaffoldYAML, Permissions: 0o644},
			{
				Path:        "deploy.yaml",
				Content:     "environment: {{ .matrix.environment }}\n",
				IsTemplate:  true,
				Permissions: 0o644,
			},
		},
	}
}

// TestExecuteWithSetup_FilesMatrixComputedAxisValueContainingWhitespace
// documents a known, accepted limitation: an axis expression's rendered
// result is parsed by splitting on whitespace, so a value that itself
// contains whitespace is split into multiple axis values rather than kept
// intact. See "Computed axes" in docs/prd/atmos-scaffold.md.
func TestExecuteWithSetup_FilesMatrixComputedAxisValueContainingWhitespace(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	cmdTemplateValues := map[string]interface{}{
		"environments": map[string]interface{}{
			"us east": map[string]interface{}{},
			"dev":     map[string]interface{}{},
		},
	}

	err := ui.executeWithSetup(whitespaceAxisEmbedsConfig(), targetDir, false, false, true, "", cmdTemplateValues, []string{"{{", "}}"})
	require.NoError(t, err)

	// "us east" is split into "us" and "east", each becoming its own
	// combination -- not kept intact as one value.
	for _, tc := range []struct {
		relPath string
		want    string
	}{
		{filepath.Join("deploy", "us.yaml"), "environment: us\n"},
		{filepath.Join("deploy", "east.yaml"), "environment: east\n"},
		{filepath.Join("deploy", "dev.yaml"), "environment: dev\n"},
	} {
		content, readErr := os.ReadFile(filepath.Join(targetDir, tc.relPath))
		require.NoError(t, readErr, "expected %s to be generated", tc.relPath)
		assert.Equal(t, tc.want, string(content))
	}

	entries, readDirErr := os.ReadDir(filepath.Join(targetDir, "deploy"))
	require.NoError(t, readDirErr)
	gotNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		gotNames = append(gotNames, entry.Name())
	}
	assert.ElementsMatch(t, []string{"dev.yaml", "us.yaml", "east.yaml"}, gotNames)
}

func TestExecuteWithSetup_FilesMatrixDuplicateTargetFails(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	scaffoldYAML := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  files:
    - path: deploy.yaml
      target: "deploy.yaml"
      matrix:
        environment: [dev, staging]
`
	embedsConfig := &templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: scaffoldYAML, Permissions: 0o644},
			{Path: "deploy.yaml", Content: "environment: {{ .matrix.environment }}\n", IsTemplate: true, Permissions: 0o644},
		},
	}

	err := ui.executeWithSetup(embedsConfig, targetDir, false, false, true, "", nil, []string{"{{", "}}"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrScaffoldDuplicateOutputPath), err)

	// "First write wins": the first-resolved combination (dev, declared
	// first in the environment axis) is written; the later duplicate
	// (staging) is refused rather than silently overwriting it -- see
	// checkDuplicateRenderedPath's own comment in pkg/generator/ui/ui.go.
	content, readErr := os.ReadFile(filepath.Join(targetDir, "deploy.yaml"))
	require.NoError(t, readErr, "the first combination's write must still have succeeded")
	assert.Equal(t, "environment: dev\n", string(content))
}

// TestProcessMatrixedFileEntry_ZeroRowsWritesSkipLine proves a matrix entry
// whose axis resolves to zero values (e.g. an empty multiselect answer)
// still writes exactly one skip line naming the file, matching
// processSingleFileEntry's own skip-line behavior, instead of silently
// producing no output at all for the entry.
func TestProcessMatrixedFileEntry_ZeroRowsWritesSkipLine(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	file := templates.File{Path: "deploy.yaml", Content: "environment: {{ .matrix.environment }}\n", IsTemplate: true, Permissions: 0o644}
	spec := config.FileSpec{
		Path:   "deploy.yaml",
		Target: "deploy/{{ .matrix.environment }}.yaml",
		Matrix: map[string]any{"environment": "answers.environments"},
	}
	mergedValues := map[string]interface{}{"environments": []interface{}{}}
	scaffoldConfig := &config.ScaffoldConfig{}

	successCount, errorCount, failedPaths, entryErr := ui.processMatrixedFileEntry(
		file, spec, spec.Target, targetDir, false, false, scaffoldConfig, mergedValues, []string{"{{", "}}"}, make(map[string]string),
	)

	assert.Equal(t, 0, successCount)
	assert.Equal(t, 0, errorCount)
	assert.Empty(t, failedPaths)
	assert.NoError(t, entryErr)
	output := ui.output.String()
	assert.Contains(t, output, "deploy.yaml")
	// Exactly one skip line, not one per would-be (zero) combination -- a
	// regression that wrote a duplicate skip line would still pass a plain
	// assert.Contains check.
	assert.Equal(t, 1, strings.Count(output, skippedText))
}

// TestProcessMatrixedFileEntry_DedupesFailedPathPerEntry proves that when
// more than one combination of the same matrix entry fails to write (here,
// because target: doesn't vary per combination, so every combination after
// the first collides), the entry's source file.Path is named once in
// failedPaths, not once per failed combination -- errorCount still counts
// every failed combination as its own failed file.
func TestProcessMatrixedFileEntry_DedupesFailedPathPerEntry(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	file := templates.File{Path: "deploy.yaml", Content: "environment: {{ .matrix.environment }}\n", IsTemplate: true, Permissions: 0o644}
	spec := config.FileSpec{
		Path:   "deploy.yaml",
		Target: "deploy.yaml",
		Matrix: map[string]any{"environment": []string{"dev", "staging", "production"}},
	}
	scaffoldConfig := &config.ScaffoldConfig{}

	successCount, errorCount, failedPaths, entryErr := ui.processMatrixedFileEntry(
		file, spec, spec.Target, targetDir, false, false, scaffoldConfig, map[string]interface{}{}, []string{"{{", "}}"}, make(map[string]string),
	)

	assert.Equal(t, 1, successCount)
	assert.Equal(t, 2, errorCount, "each colliding combination is still its own failed file")
	assert.Equal(t, []string{"deploy.yaml"}, failedPaths, "the source entry is named once, not once per failed combination")
	assert.True(t, errors.Is(entryErr, errUtils.ErrScaffoldDuplicateOutputPath), entryErr)
}

// TestProcessSingleFileEntry_WriteFailureReturnsFailedPath proves a
// non-matrix file that collides with an already-claimed output path (the
// same checkDuplicateRenderedPath guard matrix combinations use) is
// reported as one failed file, mirroring processMatrixedFileEntry's own
// failure counting for the non-matrix dispatch branch.
func TestProcessSingleFileEntry_WriteFailureReturnsFailedPath(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	file := templates.File{Path: "deploy.yaml", Content: "environment: dev\n", IsTemplate: true, Permissions: 0o644}
	spec := config.FileSpec{Path: "deploy.yaml", Target: "deploy.yaml"}
	scaffoldConfig := &config.ScaffoldConfig{}
	// Pre-claim deploy.yaml's rendered path under a different source file,
	// so checkDuplicateRenderedPath refuses this entry's own write.
	seenRenderedPaths := map[string]string{"deploy.yaml": "other.yaml"}

	successCount, errorCount, failedPaths, err := ui.processSingleFileEntry(
		file, spec, spec.Target, targetDir, false, false, scaffoldConfig, map[string]interface{}{}, []string{"{{", "}}"}, seenRenderedPaths,
	)

	assert.Equal(t, 0, successCount)
	assert.Equal(t, 1, errorCount)
	assert.Equal(t, []string{"deploy.yaml"}, failedPaths)
	assert.True(t, errors.Is(err, errUtils.ErrScaffoldDuplicateOutputPath), err)
}

// TestProcessSingleFileEntry_SkippedFileCountsAsNeitherSuccessNorFailure
// proves a non-matrix file whose rendered path is one of ShouldSkipFile's
// sentinel values (here "false", as a plain field's rendered target might
// be) is neither a success nor a failure -- distinct from spec.When's own
// earlier skip check higher up in processSingleFileEntry, which never
// reaches this far.
func TestProcessSingleFileEntry_SkippedFileCountsAsNeitherSuccessNorFailure(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	file := templates.File{Path: "deploy.yaml", Content: "environment: dev\n", IsTemplate: true, Permissions: 0o644}
	spec := config.FileSpec{Path: "deploy.yaml", Target: "false"}
	scaffoldConfig := &config.ScaffoldConfig{}

	successCount, errorCount, failedPaths, err := ui.processSingleFileEntry(
		file, spec, spec.Target, targetDir, false, false, scaffoldConfig, map[string]interface{}{}, []string{"{{", "}}"}, make(map[string]string),
	)

	assert.Equal(t, 0, successCount)
	assert.Equal(t, 0, errorCount)
	assert.Empty(t, failedPaths)
	assert.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(targetDir, "false"))
	assert.True(t, os.IsNotExist(statErr), "a skipped rendered path must not be written")
}

// TestProcessMatrixedFileEntry_ExpansionErrorReturnsFailedPath proves a
// matrix entry whose axis expression fails to resolve (an `answers.`-
// prefixed dot-path pointing at a missing key) is reported as one failed
// file, raised before any combination is even resolved.
func TestProcessMatrixedFileEntry_ExpansionErrorReturnsFailedPath(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	file := templates.File{Path: "deploy.yaml", Content: "environment: {{ .matrix.environment }}\n", IsTemplate: true, Permissions: 0o644}
	spec := config.FileSpec{
		Path:   "deploy.yaml",
		Target: "deploy/{{ .matrix.environment }}.yaml",
		Matrix: map[string]any{"environment": "answers.missing"},
	}
	scaffoldConfig := &config.ScaffoldConfig{}

	successCount, errorCount, failedPaths, entryErr := ui.processMatrixedFileEntry(
		file, spec, spec.Target, targetDir, false, false, scaffoldConfig, map[string]interface{}{}, []string{"{{", "}}"}, make(map[string]string),
	)

	assert.Equal(t, 0, successCount)
	assert.Equal(t, 1, errorCount)
	assert.Equal(t, []string{"deploy.yaml"}, failedPaths)
	assert.True(t, errors.Is(entryErr, errUtils.ErrScaffoldMatrixSourceNotFound), entryErr)
}

// TestProcessSingleFileEntry_ExistingFileWithoutForceReturnsFailedPath
// proves a real pre-existing file on disk (not an in-run duplicate-target
// collision) is also reported as one failed file, exercising
// reportWriteResult's default branch via ProcessFile's own error.
func TestProcessSingleFileEntry_ExistingFileWithoutForceReturnsFailedPath(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "deploy.yaml"), []byte("pre-existing"), 0o644))

	file := templates.File{Path: "deploy.yaml", Content: "environment: dev\n", IsTemplate: true, Permissions: 0o644}
	spec := config.FileSpec{Path: "deploy.yaml", Target: "deploy.yaml"}
	scaffoldConfig := &config.ScaffoldConfig{}

	successCount, errorCount, failedPaths, err := ui.processSingleFileEntry(
		file, spec, spec.Target, targetDir, false, false, scaffoldConfig, map[string]interface{}{}, []string{"{{", "}}"}, make(map[string]string),
	)

	assert.Equal(t, 0, successCount)
	assert.Equal(t, 1, errorCount)
	assert.Equal(t, []string{"deploy.yaml"}, failedPaths)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file already exists")
	content, readErr := os.ReadFile(filepath.Join(targetDir, "deploy.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, "pre-existing", string(content), "the pre-existing file must be left untouched")
}

// TestProcessSingleFileEntry_DryRunReportsCreateAndUpdateStatus proves
// --dry-run's "(would create)" vs "(would update)" label -- reportWriteResult's
// only DryRun-gated branch -- is driven by whether the target already
// exists, for both a brand-new file and one that would overwrite an
// existing file.
func TestProcessSingleFileEntry_DryRunReportsCreateAndUpdateStatus(t *testing.T) {
	ui := createTestUI(t)
	ui.SetDryRun(true)
	targetDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "existing.yaml"), []byte("old"), 0o644))

	scaffoldConfig := &config.ScaffoldConfig{}

	// force=true, update=false: skips handleExistingFile's ErrFileExists
	// guard and its git-backed 3-way-merge branch, going straight to
	// writeNewFile -- which, in dry-run mode, computes but skips the actual
	// write regardless. Neither sub-case below writes to disk.
	newFile := templates.File{Path: "new.yaml", Content: "environment: dev\n", IsTemplate: true, Permissions: 0o644}
	newSpec := config.FileSpec{Path: "new.yaml", Target: "new.yaml"}
	successCount, errorCount, _, err := ui.processSingleFileEntry(
		newFile, newSpec, newSpec.Target, targetDir, true, false, scaffoldConfig, map[string]interface{}{}, []string{"{{", "}}"}, make(map[string]string),
	)
	require.NoError(t, err)
	assert.Equal(t, 1, successCount)
	assert.Equal(t, 0, errorCount)
	_, statErr := os.Stat(filepath.Join(targetDir, "new.yaml"))
	assert.True(t, os.IsNotExist(statErr), "dry-run must not actually write the new file")

	existingFile := templates.File{Path: "existing.yaml", Content: "environment: staging\n", IsTemplate: true, Permissions: 0o644}
	existingSpec := config.FileSpec{Path: "existing.yaml", Target: "existing.yaml"}
	successCount, errorCount, _, err = ui.processSingleFileEntry(
		existingFile, existingSpec, existingSpec.Target, targetDir, true, false, scaffoldConfig, map[string]interface{}{}, []string{"{{", "}}"}, make(map[string]string),
	)
	require.NoError(t, err)
	assert.Equal(t, 1, successCount)
	assert.Equal(t, 0, errorCount)
	content, readErr := os.ReadFile(filepath.Join(targetDir, "existing.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(content), "dry-run must not actually overwrite the existing file")

	output := ui.output.String()
	assert.Contains(t, output, dryRunCreateStatus)
	assert.Contains(t, output, dryRunUpdateStatus)
}

// TestProcessSingleFileEntry_MalformedTargetFallsBackToRawTemplate proves
// writeOneOutput's path-rendering fallback: when outputTemplate fails to
// parse (an author typo, not a missing answer), the error status line
// names the raw template string rather than an empty path or a panic.
func TestProcessSingleFileEntry_MalformedTargetFallsBackToRawTemplate(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	const malformedTarget = "deploy/{{ .matrix.region"
	file := templates.File{Path: "deploy.yaml", Content: "environment: dev\n", IsTemplate: true, Permissions: 0o644}
	spec := config.FileSpec{Path: "deploy.yaml", Target: malformedTarget}
	scaffoldConfig := &config.ScaffoldConfig{}

	successCount, errorCount, failedPaths, err := ui.processSingleFileEntry(
		file, spec, spec.Target, targetDir, false, false, scaffoldConfig, map[string]interface{}{}, []string{"{{", "}}"}, make(map[string]string),
	)

	assert.Equal(t, 0, successCount)
	assert.Equal(t, 1, errorCount)
	assert.Equal(t, []string{"deploy.yaml"}, failedPaths)
	require.Error(t, err)
	assert.Contains(t, ui.output.String(), malformedTarget)
}

// TestProcessMatrixRow_PrunedRowWithMalformedTargetFallsBackToRawTemplate
// mirrors the test above for processMatrixRow's own copy of the fallback,
// reached only when the row is pruned by `when:` for the skip line's
// display.
func TestProcessMatrixRow_PrunedRowWithMalformedTargetFallsBackToRawTemplate(t *testing.T) {
	ui := createTestUI(t)
	targetDir := t.TempDir()

	const malformedTarget = "deploy/{{ .matrix.region"
	file := templates.File{Path: "deploy.yaml", Content: "environment: dev\n", IsTemplate: true, Permissions: 0o644}
	spec := config.FileSpec{
		Path:   "deploy.yaml",
		Target: malformedTarget,
		Matrix: map[string]any{"region": []string{"us-east-1"}},
		When:   condition.Must("false"),
	}

	success, failed, causeErr := ui.processMatrixRow(
		file, spec, spec.Target, targetDir, false, false, &config.ScaffoldConfig{}, map[string]interface{}{}, map[string]string{"region": "us-east-1"}, []string{"{{", "}}"}, make(map[string]string),
	)

	assert.False(t, success)
	assert.False(t, failed)
	assert.NoError(t, causeErr)
	assert.Contains(t, ui.output.String(), malformedTarget)
}
