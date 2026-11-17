package snapshot

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// withRehashHook registers a callback invoked after a source file was hashed again and before the
// recheck inspects its metadata one last time.
func withRehashHook(hook func(path string)) Option {
	return func(o *options) { o.onRehashed = hook }
}

func TestCapture_RejectsAModeChangeDuringTheFinalHash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("file modes cannot be changed independently of content on %s", runtime.GOOS)
	}
	src := t.TempDir()
	file := filepath.Join(src, "tool.sh")
	require.NoError(t, os.WriteFile(file, []byte("#!/bin/sh\n"), 0o644))
	require.NoError(t, os.Chmod(file, 0o644))

	// Byte count, digest, and modification time all stay the same: only the mode changes.
	hook := func(rel string) {
		assert.Equal(t, "tool.sh", rel)
		require.NoError(t, os.Chmod(file, 0o755))
	}
	snap, err := Capture(t.Context(), src, WithStagingRoot(t.TempDir()), withRehashHook(hook))

	require.ErrorIs(t, err, errUtils.ErrArtifactSourceChanged)
	assert.Nil(t, snap)
}

// withResolvedHook registers a callback invoked once the source has been resolved and before it is read.
func withResolvedHook(hook func(path string)) Option {
	return func(o *options) { o.onResolved = hook }
}

// A source directory replaced by a symlink to another directory after it was resolved would be read
// through the link: every child record describes the link's target, so the per-file checks agree with
// each other. The root's own identity is retained and verified, so the capture is rejected.
func TestCapture_RejectsReplacementOfTheSourceRootBeforeItIsRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("a directory cannot be swapped for a symlink portably on %s", runtime.GOOS)
	}
	parent := t.TempDir()
	src := filepath.Join(parent, "src")
	require.NoError(t, os.Mkdir(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("selected"), 0o644))
	outside := filepath.Join(parent, "outside")
	require.NoError(t, os.Mkdir(outside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("not selected"), 0o644))

	swapped := false
	hook := func(string) {
		swapped = true
		require.NoError(t, os.Rename(src, filepath.Join(parent, "moved")))
		require.NoError(t, os.Symlink(outside, src))
	}
	snap, err := Capture(t.Context(), src, WithStagingRoot(t.TempDir()), withResolvedHook(hook))

	require.True(t, swapped, "the hook ran")
	require.ErrorIs(t, err, errUtils.ErrArtifactSourceChanged)
	assert.Nil(t, snap)
}

func TestCapture_StaysQuietWhenNothingChangesDuringTheFinalHash(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0o644))
	var seen []string
	snap, err := Capture(t.Context(), src, WithStagingRoot(t.TempDir()), withRehashHook(func(rel string) { seen = append(seen, rel) }))
	require.NoError(t, err)
	defer snap.Close()
	assert.Equal(t, []string{"a.txt"}, seen)
}

func TestCapture_RejectsCancellationBeforeAndAfterAnEmptyDirectory(t *testing.T) {
	tests := []struct {
		name string
		do   func(t *testing.T, stagingRoot string) error
	}{
		{"already canceled", func(t *testing.T, stagingRoot string) error {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err := Capture(ctx, t.TempDir(), WithStagingRoot(stagingRoot))
			return err
		}},
		{"canceled after the last file was rechecked", func(t *testing.T, stagingRoot string) error {
			src := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0o644))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			_, err := Capture(ctx, src, WithStagingRoot(stagingRoot), withRehashHook(func(string) { cancel() }))
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stagingRoot := t.TempDir()
			require.ErrorIs(t, tc.do(t, stagingRoot), context.Canceled)
			leftovers, err := os.ReadDir(stagingRoot)
			require.NoError(t, err)
			assert.Empty(t, leftovers, "a canceled capture leaves no staging directory behind")
		})
	}
}

// The containment check reads the captured links, not the source: the source may change after the
// final recheck, and the snapshot still records the links as they were captured.
func TestCheckLinkContainment_UsesTheCapturedLinkTargets(t *testing.T) {
	root := t.TempDir()
	// On disk the directory link now points deeper, which would make the chain below look contained.
	require.NoError(t, os.Mkdir(filepath.Join(root, "sub"), 0o755))
	if err := os.Symlink("sub", filepath.Join(root, "d")); err != nil {
		t.Skipf("this platform cannot create symlinks: %v", err)
	}
	require.NoError(t, os.Symlink("d/../outside/missing", filepath.Join(root, "L")))

	tests := []struct {
		name    string
		sources []sourceRecord
		wantErr error
	}{
		{"the captured chain escapes", []sourceRecord{
			{abs: filepath.Join(root, "d"), rel: "d", target: "."},
			{abs: filepath.Join(root, "L"), rel: "L", target: "d/../outside/missing"},
		}, errUtils.ErrArtifactUnsafePath},
		{"the captured chain stays inside", []sourceRecord{
			{abs: filepath.Join(root, "d"), rel: "d", target: "sub"},
			{abs: filepath.Join(root, "L"), rel: "L", target: "d/../outside/missing"},
		}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &capturer{root: root, sources: tc.sources}
			err := c.checkLinkContainment()
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Contains(t, err.Error(), `"L"`)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCapture_RejectsPathReplacementDuringTheFinalHash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows denies replacement while the source descriptor is open")
	}
	src := t.TempDir()
	file := filepath.Join(src, "data.txt")
	require.NoError(t, os.WriteFile(file, []byte("original"), 0o644))
	info, err := os.Stat(file)
	require.NoError(t, err)
	replacement := filepath.Join(t.TempDir(), "replacement")
	require.NoError(t, os.WriteFile(replacement, []byte("original"), info.Mode()))
	require.NoError(t, os.Chtimes(replacement, info.ModTime(), info.ModTime()))
	hook := func(rel string) {
		require.Equal(t, "data.txt", rel)
		require.NoError(t, os.Rename(replacement, file))
	}
	snap, err := Capture(t.Context(), src, WithStagingRoot(t.TempDir()), withRehashHook(hook))
	require.ErrorIs(t, err, errUtils.ErrArtifactSourceChanged)
	assert.Nil(t, snap)
}
