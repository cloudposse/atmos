package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestCheckNotSwitchedFromRendered_NoProjectRecordAllows(t *testing.T) {
	targetDir := t.TempDir()

	err := CheckNotSwitchedFromRendered(targetDir)

	require.NoError(t, err)
}

func TestCheckNotSwitchedFromRendered_TrackedRecordAllows(t *testing.T) {
	targetDir := t.TempDir()
	writeProjectRecord(t, targetDir, projectRecordFixture{source: "embedded", baseRef: "HEAD"})

	err := CheckNotSwitchedFromRendered(targetDir)

	require.NoError(t, err)
}

func TestCheckNotSwitchedFromRendered_NeitherRefAllows(t *testing.T) {
	targetDir := t.TempDir()
	writeProjectRecord(t, targetDir, projectRecordFixture{source: "embedded"})

	err := CheckNotSwitchedFromRendered(targetDir)

	require.NoError(t, err)
}

func TestCheckNotSwitchedFromRendered_RenderedRecordErrors(t *testing.T) {
	targetDir := t.TempDir()
	writeProjectRecord(t, targetDir, projectRecordFixture{source: "embedded", renderedRef: "abc123"})

	err := CheckNotSwitchedFromRendered(targetDir)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUpdateStrategySwitchedToTracked)
}

func TestCheckNotSwitchedFromRendered_CorruptProjectRecordPropagatesError(t *testing.T) {
	targetDir := t.TempDir()
	recordDir := filepath.Join(targetDir, ".atmos")
	require.NoError(t, os.MkdirAll(recordDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(recordDir, "scaffold.yaml"), []byte("not: valid: yaml: ["), 0o644))

	err := CheckNotSwitchedFromRendered(targetDir)

	require.Error(t, err)
}
