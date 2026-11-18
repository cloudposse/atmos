package statusfile_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/internal/testfakes"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
	"github.com/cloudposse/atmos/plugins/ssh/statusfile"
)

func sample(identity string) statusfile.Status {
	return statusfile.Status{
		Identity: identity, Operation: statusfile.OperationAuthenticate, Time: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC),
		Outcome: "failed", Key: "k", Store: "file", Code: "missing-scope", Error: "the token lacks write:ssh_signing_key",
		Added: []string{"signing"}, Warnings: []string{"w"},
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	p := testfakes.Paths(t)
	require.NoError(t, statusfile.Write(p, sample("ops")))

	got, err := statusfile.Read(p, "ops")

	require.NoError(t, err)
	assert.Equal(t, sample("ops"), got)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(statusfile.Dir(p), "ops.json"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestWriteReplacesThePreviousStatusAndLeavesNoTemporaryFile(t *testing.T) {
	p := testfakes.Paths(t)
	require.NoError(t, statusfile.Write(p, sample("ops")))
	second := sample("ops")
	second.Outcome, second.Error = "unchanged", ""
	require.NoError(t, statusfile.Write(p, second))

	got, err := statusfile.Read(p, "ops")

	require.NoError(t, err)
	assert.Equal(t, "unchanged", got.Outcome)
	entries, err := os.ReadDir(statusfile.Dir(p))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestReadOfAMissingStatus(t *testing.T) {
	_, err := statusfile.Read(testfakes.Paths(t), "ops")
	assert.ErrorIs(t, err, statusfile.ErrNotFound)
}

func TestUnsafeNamesAreRejected(t *testing.T) {
	p := testfakes.Paths(t)
	assert.ErrorIs(t, statusfile.Write(p, sample("../x")), paths.ErrInvalidIdentity)
	_, err := statusfile.Read(p, "a/b")
	assert.ErrorIs(t, err, paths.ErrInvalidIdentity)
	assert.ErrorIs(t, statusfile.Remove(p, ".."), paths.ErrInvalidIdentity)
}

func TestListIsSortedAndReportsUnreadableFilesWithoutHidingTheRest(t *testing.T) {
	p := testfakes.Paths(t)
	for _, name := range []string{"zeta", "alpha"} {
		require.NoError(t, statusfile.Write(p, sample(name)))
	}
	require.NoError(t, os.WriteFile(filepath.Join(statusfile.Dir(p), "broken.json"), []byte("{"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(statusfile.Dir(p), "notes.txt"), []byte("x"), 0o600))

	list, err := statusfile.List(p)

	require.Error(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, []string{"alpha", "zeta"}, []string{list[0].Identity, list[1].Identity})
}

func TestListOfNothing(t *testing.T) {
	list, err := statusfile.List(testfakes.Paths(t))
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestRemove(t *testing.T) {
	p := testfakes.Paths(t)
	require.NoError(t, statusfile.Write(p, sample("ops")))
	require.NoError(t, statusfile.Remove(p, "ops"))
	require.NoError(t, statusfile.Remove(p, "ops"), "removing twice is fine")
	_, err := statusfile.Read(p, "ops")
	assert.ErrorIs(t, err, statusfile.ErrNotFound)
}
