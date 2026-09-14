package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// projectRecordFixture bundles writeProjectRecord's spec fields (grouped
// into a struct, rather than several separate parameters, both to stay
// under revive's argument-limit and so each test call site reads clearly
// about which of baseRef/renderedRef -- normally mutually exclusive, see
// ScaffoldSpec.RenderedRef -- it's setting).
type projectRecordFixture struct {
	source      string
	baseRef     string
	renderedRef string
}

// writeProjectRecord writes a minimal .atmos/scaffold.yaml project record at
// targetDir, matching pkg/project/config's ScaffoldConfig/ScaffoldSpec
// schema, so ResolveRenderedBase has something to load back. The baseRef and
// renderedRef lines are omitted entirely when empty -- an empty YAML scalar
// (`baseRef: `) parses as null, which the schema rejects for a string field,
// whereas an absent key is simply the zero value.
func writeProjectRecord(t *testing.T, targetDir string, fx projectRecordFixture) {
	t.Helper()
	recordDir := filepath.Join(targetDir, ".atmos")
	require.NoError(t, os.MkdirAll(recordDir, 0o755))
	record := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: sample
spec:
  source: ` + fx.source + `
`
	if fx.baseRef != "" {
		record += "  baseRef: " + fx.baseRef + "\n"
	}
	if fx.renderedRef != "" {
		record += "  renderedRef: " + fx.renderedRef + "\n"
	}
	record += `  values:
    project_name: old-project
`
	require.NoError(t, os.WriteFile(filepath.Join(recordDir, "scaffold.yaml"), []byte(record), 0o644))
}

func TestResolveRenderedBase_LoadsOldRefAndValues(t *testing.T) {
	templateDir := writeSampleTemplate(t)
	targetDir := t.TempDir()
	writeProjectRecord(t, targetDir, projectRecordFixture{source: templateDir, renderedRef: "HEAD"})

	renderedBase, err := ResolveRenderedBase(targetDir, "")
	require.NoError(t, err)
	require.NotNil(t, renderedBase.Cleanup)
	t.Cleanup(renderedBase.Cleanup)

	require.NotNil(t, renderedBase.Config)
	assert.NotEmpty(t, renderedBase.Config.Files, "the old ref's template must be fully hydrated")
	assert.Equal(t, map[string]interface{}{"project_name": "old-project"}, renderedBase.Values)
}

func TestResolveRenderedBase_NoProjectRecordErrors(t *testing.T) {
	targetDir := t.TempDir()

	_, err := ResolveRenderedBase(targetDir, "")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRenderedStrategyRequiresConfig)
}

// TestResolveRenderedBase_RecordWithNeitherRefErrors covers a project record
// that exists but was never generated under either update strategy (both
// baseRef and renderedRef empty) -- the same "no rendered history" error as
// no record at all, not the switched-from-tracked error (that one requires
// baseRef to actually be set).
func TestResolveRenderedBase_RecordWithNeitherRefErrors(t *testing.T) {
	targetDir := t.TempDir()
	writeProjectRecord(t, targetDir, projectRecordFixture{source: "embedded"})

	_, err := ResolveRenderedBase(targetDir, "")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRenderedStrategyRequiresConfig)
}

// TestResolveRenderedBase_SwitchedFromTrackedErrors covers a project last
// updated with --update-strategy=tracked (baseRef set, renderedRef never
// populated by tracked mode): rendered has no resolved commit to reconstruct
// from, so this must fail with a switch-specific error, not silently fetch
// something unrelated to what baseRef happens to contain.
func TestResolveRenderedBase_SwitchedFromTrackedErrors(t *testing.T) {
	templateDir := writeSampleTemplate(t)
	targetDir := t.TempDir()
	writeProjectRecord(t, targetDir, projectRecordFixture{source: templateDir, baseRef: "HEAD"})

	_, err := ResolveRenderedBase(targetDir, "")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUpdateStrategySwitchedToRendered)
}

func TestResolveRenderedBase_CorruptProjectRecordPropagatesError(t *testing.T) {
	targetDir := t.TempDir()
	recordDir := filepath.Join(targetDir, ".atmos")
	require.NoError(t, os.MkdirAll(recordDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(recordDir, "scaffold.yaml"), []byte("not: valid: yaml: ["), 0o644))

	_, err := ResolveRenderedBase(targetDir, "")

	require.Error(t, err)
}

func TestResolveRenderedBase_UnfetchableSourcePropagatesError(t *testing.T) {
	targetDir := t.TempDir()
	writeProjectRecord(t, targetDir, projectRecordFixture{
		source:      filepath.Join(t.TempDir(), "does-not-exist"),
		renderedRef: "HEAD",
	})

	_, err := ResolveRenderedBase(targetDir, "")

	require.Error(t, err)
}
