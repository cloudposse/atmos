package oci

import (
	"archive/tar"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tarLayerEntry struct {
	name string
	body string
}

func orderedTarLayer(t *testing.T, entries []tarLayerEntry) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	for _, entry := range entries {
		require.NoError(t, writer.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o644, Size: int64(len(entry.body))}))
		_, err := writer.Write([]byte(entry.body))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return &buf
}

func extractedFiles(t *testing.T, directory string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	require.NoError(t, filepath.WalkDir(directory, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(directory, name)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(name)
		files[filepath.ToSlash(relative)] = string(content)
		return err
	}))
	return files
}

func TestProcessLayer_WhiteoutsPreserveSameLayerEntries(t *testing.T) {
	for _, markersFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "markers first", false: "markers last"}[markersFirst], func(t *testing.T) {
			destination := t.TempDir()
			lower := writeTestTar(t, map[string]string{
				"remove.txt": "old", "removed/old": "old", "keep": "lower",
				"opaque/old": "old", "opaque/nested/stale": "old", "opaque/shared": "old",
				"replaced": "old", "recreated/old": "old",
			})
			require.NoError(t, processLayer(&MockLayer{uncompressedData: lower.Bytes()}, 0, destination))
			files := []tarLayerEntry{{"opaque/new", "new"}, {"opaque/nested/new", "nested"}, {"opaque/shared", "upper"}, {"replaced", "upper"}, {"recreated/new", "new"}}
			markers := []tarLayerEntry{{".wh.remove.txt", ""}, {".wh.removed", ""}, {"opaque/.wh..wh..opq", ""}, {".wh.replaced", ""}, {".wh.recreated", ""}, {".wh.absent", ""}}
			entries := append(append([]tarLayerEntry{}, files...), markers...)
			if markersFirst {
				entries = append(append([]tarLayerEntry{}, markers...), files...)
			}
			upper := orderedTarLayer(t, entries)
			require.NoError(t, processLayer(&MockLayer{uncompressedData: upper.Bytes()}, 1, destination))
			assert.Equal(t, map[string]string{
				"keep": "lower", "opaque/new": "new", "opaque/nested/new": "nested",
				"opaque/shared": "upper", "replaced": "upper", "recreated/new": "new",
			}, extractedFiles(t, destination))
			last := orderedTarLayer(t, []tarLayerEntry{{".wh.keep", ""}, {"opaque/.wh.new", ""}, {"third", "last"}})
			require.NoError(t, processLayer(&MockLayer{uncompressedData: last.Bytes()}, 2, destination))
			assert.Equal(t, map[string]string{
				"opaque/nested/new": "nested", "opaque/shared": "upper", "replaced": "upper", "recreated/new": "new", "third": "last",
			}, extractedFiles(t, destination))
		})
	}
}

func TestExtractTarball_RootOpaqueWhiteout(t *testing.T) {
	destination := t.TempDir()
	require.NoError(t, extractTarball(writeTestTar(t, map[string]string{"old": "old", "nested/old": "old"}), destination))
	upper := orderedTarLayer(t, []tarLayerEntry{{"nested/new", "new"}, {".wh..wh..opq", ""}, {"last", "last"}})
	require.NoError(t, extractTarball(upper, destination))
	assert.Equal(t, map[string]string{"nested/new": "new", "last": "last"}, extractedFiles(t, destination))
}

func TestExtractTarball_WhiteoutRejectsUnsafeTargets(t *testing.T) {
	for _, name := range []string{"../.wh.victim", "/.wh.victim", ".wh.", ".wh..", ".wh...", "nested/.wh...", ".wh.parent/child", `nested\.wh.victim`} {
		t.Run(name, func(t *testing.T) {
			destination := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(destination, "victim"), []byte("safe"), 0o600))
			err := extractTarball(orderedTarLayer(t, []tarLayerEntry{{name, ""}}), destination)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidFilePath)
			assert.Equal(t, map[string]string{"victim": "safe"}, extractedFiles(t, destination))
		})
	}
}

func TestExtractTarball_WhiteoutDoesNotFollowSymlink(t *testing.T) {
	for _, marker := range []string{"link/.wh.victim", "link/.wh..wh..opq"} {
		t.Run(marker, func(t *testing.T) {
			destination, outside := t.TempDir(), t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(outside, "victim"), []byte("safe"), 0o600))
			createWhiteoutTestLink(t, os.Symlink, outside, filepath.Join(destination, "link"))
			require.Error(t, extractTarball(orderedTarLayer(t, []tarLayerEntry{{marker, ""}}), destination))
			assert.Equal(t, map[string]string{"victim": "safe"}, extractedFiles(t, outside))
		})
	}
}

func TestExtractTarball_WhiteoutUnlinksWithoutChangingLinkTarget(t *testing.T) {
	for _, link := range []struct {
		name string
		make func(string, string) error
	}{{"symlink", os.Symlink}, {"hardlink", os.Link}} {
		t.Run(link.name, func(t *testing.T) {
			destination, outside := t.TempDir(), t.TempDir()
			target := filepath.Join(outside, "victim")
			require.NoError(t, os.WriteFile(target, []byte("safe"), 0o600))
			createWhiteoutTestLink(t, link.make, target, filepath.Join(destination, "victim"))
			require.NoError(t, extractTarball(orderedTarLayer(t, []tarLayerEntry{{"victim", "replacement"}, {".wh.victim", ""}}), destination))
			assert.Equal(t, map[string]string{"victim": "replacement"}, extractedFiles(t, destination))
			assert.Equal(t, map[string]string{"victim": "safe"}, extractedFiles(t, outside))
		})
	}
}

func TestExtractTarball_WhiteoutValidatesBeforeDeletingAndCleansSpool(t *testing.T) {
	for _, invalid := range []string{"nonempty marker", "directory marker", "truncated layer"} {
		t.Run(invalid, func(t *testing.T) {
			spool := t.TempDir()
			for _, variable := range []string{"TMPDIR", "TMP", "TEMP"} {
				t.Setenv(variable, spool)
			}
			destination := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(destination, "victim"), []byte("safe"), 0o600))
			var buf bytes.Buffer
			writer := tar.NewWriter(&buf)
			require.NoError(t, writer.WriteHeader(&tar.Header{Name: ".wh.victim", Mode: 0o644}))
			switch invalid {
			case "nonempty marker":
				require.NoError(t, writer.WriteHeader(&tar.Header{Name: ".wh.other", Size: 1, Mode: 0o644}))
				_, err := writer.Write([]byte("x"))
				require.NoError(t, err)
				require.NoError(t, writer.Close())
			case "directory marker":
				require.NoError(t, writer.WriteHeader(&tar.Header{Name: ".wh.other", Typeflag: tar.TypeDir, Mode: 0o755}))
				require.NoError(t, writer.Close())
			case "truncated layer":
				require.NoError(t, writer.WriteHeader(&tar.Header{Name: "truncated", Size: 10, Mode: 0o644}))
				_, err := writer.Write([]byte("x"))
				require.NoError(t, err)
				require.Error(t, writer.Close())
			}
			require.Error(t, extractTarball(&buf, destination))
			assert.Equal(t, map[string]string{"victim": "safe"}, extractedFiles(t, destination))
			leftovers, err := filepath.Glob(filepath.Join(spool, "atmos-oci-layer-*.tar"))
			require.NoError(t, err)
			assert.Empty(t, leftovers)
			require.NoError(t, extractTarball(orderedTarLayer(t, []tarLayerEntry{{".wh.victim", ""}, {"new", "new"}}), destination))
			assert.Equal(t, map[string]string{"new": "new"}, extractedFiles(t, destination))
			leftovers, err = filepath.Glob(filepath.Join(spool, "atmos-oci-layer-*.tar"))
			require.NoError(t, err)
			assert.Empty(t, leftovers)
		})
	}
}

func TestExtractTarball_WhiteoutsKeepUnsupportedLinksSkipped(t *testing.T) {
	destination := t.TempDir()
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	for _, header := range []*tar.Header{
		{Name: "target", Mode: 0o644},
		{Name: "symlink", Typeflag: tar.TypeSymlink, Linkname: "target"},
		{Name: "hardlink", Typeflag: tar.TypeLink, Linkname: "target"},
		{Name: ".wh.absent", Mode: 0o644},
	} {
		require.NoError(t, writer.WriteHeader(header))
	}
	require.NoError(t, writer.Close())
	require.NoError(t, extractTarball(&buf, destination))
	assert.Equal(t, map[string]string{"target": ""}, extractedFiles(t, destination))
}

func TestExtractTarball_WhiteoutWithoutLowerDirectory(t *testing.T) {
	for _, parentIsFile := range []bool{false, true} {
		for _, marker := range []string{"parent/.wh.victim", "parent/.wh..wh..opq"} {
			t.Run(map[bool]string{false: "missing/", true: "file/"}[parentIsFile]+marker, func(t *testing.T) {
				destination := t.TempDir()
				want := map[string]string{"keep": "safe"}
				if parentIsFile {
					want["parent"] = "not a directory"
				}
				for name, content := range want {
					require.NoError(t, os.WriteFile(filepath.Join(destination, name), []byte(content), 0o600))
				}
				require.NoError(t, extractTarball(orderedTarLayer(t, []tarLayerEntry{{marker, ""}, {"new", "upper"}}), destination))
				want["new"] = "upper"
				assert.Equal(t, want, extractedFiles(t, destination))
			})
		}
	}
}

func TestExtractTarball_WhiteoutSpoolUnavailablePreservesLowerLayer(t *testing.T) {
	destination := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(destination, "victim"), []byte("safe"), 0o600))
	missingSpool := filepath.Join(t.TempDir(), "missing")
	for _, variable := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(variable, missingSpool)
	}
	err := extractTarball(orderedTarLayer(t, []tarLayerEntry{{".wh.victim", ""}, {"new", "upper"}}), destination)
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Equal(t, map[string]string{"victim": "safe"}, extractedFiles(t, destination))
}

func TestPrepareTarLayer_ClosedRootCleansSpool(t *testing.T) {
	destination, spool := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(destination, "victim"), []byte("safe"), 0o600))
	root, err := os.OpenRoot(destination)
	require.NoError(t, err)
	require.NoError(t, root.Close())
	for _, variable := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(variable, spool)
	}
	layer, err := prepareTarLayer(orderedTarLayer(t, []tarLayerEntry{{".wh.victim", ""}}), root, destination)
	require.ErrorIs(t, err, os.ErrClosed)
	assert.Nil(t, layer)
	assert.Equal(t, map[string]string{"victim": "safe"}, extractedFiles(t, destination))
	leftovers, err := os.ReadDir(spool)
	require.NoError(t, err)
	assert.Empty(t, leftovers)
}

func createWhiteoutTestLink(t *testing.T, makeLink func(string, string) error, target, name string) {
	t.Helper()
	err := makeLink(target, name)
	if err != nil && runtime.GOOS == "windows" {
		t.Skipf("link creation unavailable on this Windows filesystem: %v", err)
	}
	require.NoError(t, err)
}
