package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// writeProjectRecord writes a minimal .atmos/scaffold.yaml project record at
// targetDir, matching pkg/project/config's ScaffoldConfig/ScaffoldSpec
// schema, so ResolveRenderedBase has something to load back.
func writeProjectRecord(t *testing.T, targetDir, source, baseRef string) {
	t.Helper()
	recordDir := filepath.Join(targetDir, ".atmos")
	require.NoError(t, os.MkdirAll(recordDir, 0o755))
	record := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: sample
spec:
  source: ` + source + `
  baseRef: ` + baseRef + `
  values:
    project_name: old-project
`
	require.NoError(t, os.WriteFile(filepath.Join(recordDir, "scaffold.yaml"), []byte(record), 0o644))
}

func TestResolveRenderedBase_LoadsOldRefAndValues(t *testing.T) {
	templateDir := writeSampleTemplate(t)
	targetDir := t.TempDir()
	writeProjectRecord(t, targetDir, templateDir, "HEAD")

	oldConfig, oldValues, cleanup, err := ResolveRenderedBase(targetDir, "")
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	t.Cleanup(cleanup)

	require.NotNil(t, oldConfig)
	assert.NotEmpty(t, oldConfig.Files, "the old ref's template must be fully hydrated")
	assert.Equal(t, map[string]interface{}{"project_name": "old-project"}, oldValues)
}

func TestResolveRenderedBase_NoProjectRecordErrors(t *testing.T) {
	targetDir := t.TempDir()

	_, _, _, err := ResolveRenderedBase(targetDir, "")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRenderedStrategyRequiresConfig)
}

func TestResolveRenderedBase_CorruptProjectRecordPropagatesError(t *testing.T) {
	targetDir := t.TempDir()
	recordDir := filepath.Join(targetDir, ".atmos")
	require.NoError(t, os.MkdirAll(recordDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(recordDir, "scaffold.yaml"), []byte("not: valid: yaml: ["), 0o644))

	_, _, _, err := ResolveRenderedBase(targetDir, "")

	require.Error(t, err)
}

func TestResolveRenderedBase_UnfetchableSourcePropagatesError(t *testing.T) {
	targetDir := t.TempDir()
	writeProjectRecord(t, targetDir, filepath.Join(t.TempDir(), "does-not-exist"), "")

	_, _, _, err := ResolveRenderedBase(targetDir, "")

	require.Error(t, err)
}
