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
// any environment) via the "keys" template function, pruned by when: to only
// the environment/region pairs that actually apply.
const computedAxisScaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  files:
    - path: deploy.yaml
      target: "deploy/{{ .matrix.environment }}/{{ .matrix.region }}.yaml"
      matrix:
        environment: '{{ keys answers.environments }}'
        region: '{{ keys answers.environments "regions" }}'
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
        environment: '{{ keys answers.environments }}'
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

// TestExecuteWithSetup_FilesMatrixComputedAxisValueContainingWhitespace is
// the end-to-end regression test for a computed axis value that itself
// contains whitespace: proves both target-path rendering (the generated
// file's own path) and the file's rendered content carry the value through
// intact, all the way from the raw answers map through ExpandMatrix,
// .matrix.<axis> binding, and template rendering -- not split into two
// combinations, and not truncated at the space.
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

	for _, tc := range []struct {
		relPath string
		want    string
	}{
		{filepath.Join("deploy", "us east.yaml"), "environment: us east\n"},
		{filepath.Join("deploy", "dev.yaml"), "environment: dev\n"},
	} {
		content, readErr := os.ReadFile(filepath.Join(targetDir, tc.relPath))
		require.NoError(t, readErr, "expected %s to be generated", tc.relPath)
		assert.Equal(t, tc.want, string(content))
	}
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
