package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/generator/templates"
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
}
