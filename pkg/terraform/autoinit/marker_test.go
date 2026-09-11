package autoinit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestMarker_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "atmos-init.json")
	want := &Marker{
		SchemaVersion:      MarkerSchemaVersion,
		Fingerprint:        "abc123",
		BackendFingerprint: "def456",
		InitArgs:           []string{"-upgrade"},
		AtmosVersion:       "1.2.3",
		Binary:             "terraform",
		Timestamp:          time.Now().UTC().Truncate(time.Second),
	}

	require.NoError(t, WriteMarker(path, want))

	got, err := ReadMarker(path)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, want.SchemaVersion, got.SchemaVersion)
	assert.Equal(t, want.Fingerprint, got.Fingerprint)
	assert.Equal(t, want.BackendFingerprint, got.BackendFingerprint)
	assert.Equal(t, want.InitArgs, got.InitArgs)
	assert.Equal(t, want.AtmosVersion, got.AtmosVersion)
	assert.Equal(t, want.Binary, got.Binary)
	assert.True(t, want.Timestamp.Equal(got.Timestamp))
}

func TestReadMarker_MissingReturnsNilNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	got, err := ReadMarker(path)

	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestReadMarker_MalformedJSONReturnsNilNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atmos-init.json")
	require.NoError(t, os.WriteFile(path, []byte("{not valid json"), 0o644))

	got, err := ReadMarker(path)

	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestReadMarker_OtherIOErrorWrapsErrInitMarker(t *testing.T) {
	// A directory at path is a read error that is not os.IsNotExist, so it must surface as a
	// genuine ErrInitMarker-wrapped failure rather than being treated as "no marker yet".
	dirAsPath := t.TempDir()

	got, err := ReadMarker(dirAsPath)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, errUtils.ErrInitMarker))
}

func TestWriteMarker_CreatesParentDirsAndLeavesNoTempFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a", "b", "c", "atmos-init.json")

	require.NoError(t, WriteMarker(path, &Marker{SchemaVersion: MarkerSchemaVersion}))

	entries, err := os.ReadDir(filepath.Join(root, "a", "b", "c"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, MarkerFileName, entries[0].Name())
}

func TestRecord_WritesMarkerWithRecomputedFingerprint(t *testing.T) {
	componentDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte("v1"), 0o644))

	dataDir := filepath.Join(componentDir, ".terraform")
	in := &Inputs{ComponentPath: componentDir, DataDir: dataDir, Binary: testBinary()}

	wantFP, err := Compute(in)
	require.NoError(t, err)

	require.NoError(t, Record(in, []string{"-upgrade"}, "1.2.3"))

	got, err := ReadMarker(MarkerPath(dataDir))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, wantFP.Hash, got.Fingerprint)
	assert.Equal(t, wantFP.BackendHash, got.BackendFingerprint)
	assert.Equal(t, []string{"-upgrade"}, got.InitArgs)
	assert.Equal(t, "1.2.3", got.AtmosVersion)
	assert.Equal(t, MarkerSchemaVersion, got.SchemaVersion)
}
