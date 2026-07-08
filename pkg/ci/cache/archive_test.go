package cache

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestArchiveRoot_RoundTrip(t *testing.T) {
	src := t.TempDir()
	// Build a nested tree. Keep first/last entries deterministic by name.
	files := map[string]string{
		"a-first.txt": "first",
		filepath.Join("toolchain", "bin", "tool"): "binary",
		filepath.Join("toolchain", "lock.yaml"):   "lock",
		"z-last.txt":                              "last",
	}
	for rel, content := range files {
		full := filepath.Join(src, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	// State dir must be excluded from the archive.
	require.NoError(t, os.MkdirAll(filepath.Join(src, stateDirName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, stateDirName, "state.json"), []byte("{}"), 0o644))

	var buf bytes.Buffer
	require.NoError(t, archiveRoot(&buf, src, nil, false))

	dst := t.TempDir()
	require.NoError(t, extractToRoot(&buf, dst))

	// Assert contents by value (first and last entries explicitly).
	gotFirst, err := os.ReadFile(filepath.Join(dst, "a-first.txt"))
	require.NoError(t, err)
	assert.Equal(t, "first", string(gotFirst))

	gotLast, err := os.ReadFile(filepath.Join(dst, "z-last.txt"))
	require.NoError(t, err)
	assert.Equal(t, "last", string(gotLast))

	gotTool, err := os.ReadFile(filepath.Join(dst, "toolchain", "bin", "tool"))
	require.NoError(t, err)
	assert.Equal(t, "binary", string(gotTool))

	// State dir must NOT have been archived/restored.
	_, statErr := os.Stat(filepath.Join(dst, stateDirName))
	assert.True(t, os.IsNotExist(statErr), "state dir must be excluded from the archive")
}

func TestArchiveRoot_IncludesFilter(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "toolchain"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "toolchain", "tool"), []byte("keep"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(src, "other"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "other", "skip"), []byte("drop"), 0o644))

	var buf bytes.Buffer
	require.NoError(t, archiveRoot(&buf, src, []string{"toolchain"}, false))

	dst := t.TempDir()
	require.NoError(t, extractToRoot(&buf, dst))

	_, err := os.Stat(filepath.Join(dst, "toolchain", "tool"))
	assert.NoError(t, err, "included path must be archived")

	_, err = os.Stat(filepath.Join(dst, "other", "skip"))
	assert.True(t, os.IsNotExist(err), "excluded path must not be archived")
}

func TestArchiveRoot_DefaultExcludesAuthCaches(t *testing.T) {
	src := t.TempDir()
	files := map[string]string{
		filepath.Join("toolchain", "tool"):                        "tool",
		filepath.Join("aws-sso", "sessions", "x.json"):            "sso-token",
		filepath.Join("azure-device-code", "p", "token.json"):     "azure-token",
		filepath.Join("aws-webflow", "id-realm", "refresh.json"):  "webflow-refresh",
		filepath.Join("auth", "p", "provisioned-identities.yaml"): "identities",
	}
	for rel, content := range files {
		full := filepath.Join(src, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}

	var buf bytes.Buffer
	require.NoError(t, archiveRoot(&buf, src, nil, false))

	dst := t.TempDir()
	require.NoError(t, extractToRoot(&buf, dst))

	_, err := os.Stat(filepath.Join(dst, "toolchain", "tool"))
	assert.NoError(t, err, "non-auth path must be archived")

	for _, excluded := range []string{"aws-sso", "azure-device-code", "aws-webflow", "auth"} {
		_, err := os.Stat(filepath.Join(dst, excluded))
		assert.True(t, os.IsNotExist(err), "%q must be excluded from the default archive", excluded)
	}
}

func TestArchiveRoot_AllowUnsafeAuthCache(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "aws-sso", "sessions"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "aws-sso", "sessions", "x.json"), []byte("sso-token"), 0o644))

	var buf bytes.Buffer
	require.NoError(t, archiveRoot(&buf, src, nil, true))

	dst := t.TempDir()
	require.NoError(t, extractToRoot(&buf, dst))

	got, err := os.ReadFile(filepath.Join(dst, "aws-sso", "sessions", "x.json"))
	require.NoError(t, err, "opted-in auth cache must be archived")
	assert.Equal(t, "sso-token", string(got))
}

func TestSafeJoin_RejectsEscape(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "root")

	// Hostile entry names that attempt to escape the root must be rejected.
	for _, name := range []string{
		"../escape",
		filepath.Join("..", "..", "etc", "passwd"),
		filepath.Join("sub", "..", "..", "escape"),
	} {
		_, err := safeJoin(root, name)
		require.Error(t, err, "entry %q must be rejected", name)
	}

	// Legitimate nested entries resolve within the root.
	ok, err := safeJoin(root, filepath.Join("sub", "file"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "sub", "file"), ok)

	okNested, err := safeJoin(root, filepath.Join("a", "b", "c.txt"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "a", "b", "c.txt"), okNested)
}

func TestArchiveSkipDecision(t *testing.T) {
	sep := string(os.PathSeparator)

	t.Run("state dir is pruned", func(t *testing.T) {
		handled, action := archiveSkipDecision(stateDirName, nil, true, false)
		assert.True(t, handled)
		assert.Equal(t, filepath.SkipDir, action)
	})

	t.Run("file under state dir is skipped", func(t *testing.T) {
		handled, action := archiveSkipDecision(stateDirName+sep+"state.json", nil, false, false)
		assert.True(t, handled)
		assert.NoError(t, action)
	})

	t.Run("empty includes match everything", func(t *testing.T) {
		handled, _ := archiveSkipDecision("anything", nil, false, false)
		assert.False(t, handled)
	})

	t.Run("dir outside includes is pruned", func(t *testing.T) {
		handled, action := archiveSkipDecision("other", []string{"toolchain"}, true, false)
		assert.True(t, handled)
		assert.Equal(t, filepath.SkipDir, action)
	})

	t.Run("file outside includes is skipped", func(t *testing.T) {
		handled, action := archiveSkipDecision("other.txt", []string{"toolchain"}, false, false)
		assert.True(t, handled)
		assert.NoError(t, action)
	})

	t.Run("ancestor dir of an include is kept", func(t *testing.T) {
		handled, _ := archiveSkipDecision("toolchain", []string{filepath.Join("toolchain", "bin")}, true, false)
		assert.False(t, handled)
	})

	t.Run("matching path is kept", func(t *testing.T) {
		handled, _ := archiveSkipDecision("toolchain", []string{"toolchain"}, true, false)
		assert.False(t, handled)
	})

	t.Run("each default excluded subdir is pruned", func(t *testing.T) {
		for _, ex := range defaultExcludedPaths {
			handled, action := archiveSkipDecision(ex, nil, true, false)
			assert.True(t, handled, "expected %q to be pruned", ex)
			assert.Equal(t, filepath.SkipDir, action)
		}
	})

	t.Run("file under a default excluded dir is skipped", func(t *testing.T) {
		handled, action := archiveSkipDecision(filepath.Join("aws-sso", "sessions", "x.json"), nil, false, false)
		assert.True(t, handled)
		assert.NoError(t, action)
	})

	t.Run("default excluded dir is pruned even when explicitly included", func(t *testing.T) {
		handled, action := archiveSkipDecision("aws-sso", []string{"aws-sso"}, true, false)
		assert.True(t, handled, "explicit include must not override the default exclusion")
		assert.Equal(t, filepath.SkipDir, action)
	})

	t.Run("allowUnsafeAuthCache opts back in", func(t *testing.T) {
		handled, _ := archiveSkipDecision("aws-sso", nil, true, true)
		assert.False(t, handled)
	})
}

func TestExtractToRoot_InvalidGzip(t *testing.T) {
	err := extractToRoot(bytes.NewReader([]byte("not gzip data")), t.TempDir())
	require.ErrorIs(t, err, errUtils.ErrCacheExtractFailed)
}

func TestArchiveRoot_NonExistentRoot(t *testing.T) {
	// WalkDir on a missing root surfaces as an archive failure.
	var buf bytes.Buffer
	err := archiveRoot(&buf, filepath.Join(t.TempDir(), "missing"), nil, false)
	require.ErrorIs(t, err, errUtils.ErrCacheArchiveFailed)
}

// writeGzippedTar builds a gzip-compressed tar from the given entries so extract
// edge cases can be exercised with hand-crafted headers.
func writeGzippedTar(t *testing.T, entries []*tar.Header, bodies [][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for i, h := range entries {
		require.NoError(t, tw.WriteHeader(h))
		if len(bodies[i]) > 0 {
			_, err := tw.Write(bodies[i])
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	return buf.Bytes()
}

func TestExtractToRoot_RejectsPathEscape(t *testing.T) {
	// A hostile entry name that escapes the root must be rejected at extraction.
	body := []byte("pwned")
	data := writeGzippedTar(
		t,
		[]*tar.Header{{
			Name:     "../escape.txt",
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len(body)),
		}},
		[][]byte{body},
	)

	err := extractToRoot(bytes.NewReader(data), t.TempDir())
	require.ErrorIs(t, err, errUtils.ErrCacheExtractFailed)
}

func TestExtractToRoot_DefaultPermFallback(t *testing.T) {
	// A regular entry with a zero mode must still extract, falling back to the
	// default file permission rather than creating an unwritable/zero-mode file.
	body := []byte("zero-mode content")
	data := writeGzippedTar(
		t,
		[]*tar.Header{{
			Name:     "zero.txt",
			Typeflag: tar.TypeReg,
			Mode:     0,
			Size:     int64(len(body)),
		}},
		[][]byte{body},
	)

	dst := t.TempDir()
	require.NoError(t, extractToRoot(bytes.NewReader(data), dst))

	got, err := os.ReadFile(filepath.Join(dst, "zero.txt"))
	require.NoError(t, err)
	assert.Equal(t, string(body), string(got))
}

func TestArchiveRoot_SkipsSymlink(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "real.txt"), []byte("real"), 0o644))
	if err := os.Symlink(filepath.Join(src, "real.txt"), filepath.Join(src, "link.txt")); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}

	var buf bytes.Buffer
	require.NoError(t, archiveRoot(&buf, src, nil, false))

	dst := t.TempDir()
	require.NoError(t, extractToRoot(&buf, dst))

	// The regular file is restored; the symlink is not.
	_, err := os.Stat(filepath.Join(dst, "real.txt"))
	require.NoError(t, err)
	_, err = os.Lstat(filepath.Join(dst, "link.txt"))
	require.Error(t, err, "symlink must be skipped during archive")
}
