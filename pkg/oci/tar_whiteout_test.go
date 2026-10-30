package oci

import (
	"archive/tar"
	"bytes"
	"io"
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
			leftovers, err := filepath.Glob(filepath.Join(destination, ".atmos-oci-layer-*.tar"))
			require.NoError(t, err)
			assert.Empty(t, leftovers)
			require.NoError(t, extractTarball(orderedTarLayer(t, []tarLayerEntry{{".wh.victim", ""}, {"new", "new"}}), destination))
			assert.Equal(t, map[string]string{"new": "new"}, extractedFiles(t, destination))
			leftovers, err = filepath.Glob(filepath.Join(destination, ".atmos-oci-layer-*.tar"))
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

func TestExtractTarball_UnavailableGlobalTemp(t *testing.T) {
	for _, kind := range []string{"ordinary", "whiteout", "root opaque"} {
		t.Run(kind, func(t *testing.T) {
			destination := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(destination, "victim"), []byte("lower"), 0o600))
			missingTemp := filepath.Join(t.TempDir(), "missing")
			for _, variable := range []string{"TMPDIR", "TMP", "TEMP"} {
				t.Setenv(variable, missingTemp)
			}
			entries := []tarLayerEntry{{"new", "upper"}}
			want := map[string]string{"new": "upper"}
			switch kind {
			case "ordinary":
				want["victim"] = "lower"
			case "whiteout":
				entries = append(entries, tarLayerEntry{".wh.victim", ""})
			case "root opaque":
				entries = append(entries, tarLayerEntry{".wh..wh..opq", ""})
			}
			require.NoError(t, extractTarball(orderedTarLayer(t, entries), destination))
			assert.Equal(t, want, extractedFiles(t, destination))
		})
	}
}

func TestPrepareTarLayer_ClosedRootCreatesNoSpool(t *testing.T) {
	destination := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(destination, "victim"), []byte("safe"), 0o600))
	root, err := os.OpenRoot(destination)
	require.NoError(t, err)
	require.NoError(t, root.Close())
	layer, err := prepareTarLayer(orderedTarLayer(t, []tarLayerEntry{{".wh.victim", ""}}), root, destination)
	require.ErrorIs(t, err, os.ErrClosed)
	assert.Nil(t, layer)
	assert.Equal(t, map[string]string{"victim": "safe"}, extractedFiles(t, destination))
}

type tarSpoolReaderFunc func([]byte) (int, error)

func (read tarSpoolReaderFunc) Read(p []byte) (int, error) { return read(p) }

func TestExtractTarball_RejectsSpoolCollisionsBeforeDeleting(t *testing.T) {
	for _, kind := range []string{"entry", "descendant", "whiteout", "symlink", "hardlink", "alias descendant", "directory alias", "whiteout alias"} {
		t.Run(kind, func(t *testing.T) {
			destination := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(destination, "victim"), []byte("safe"), 0o600))
			var archive io.Reader
			alias := ""
			reader := tarSpoolReaderFunc(func(p []byte) (int, error) {
				if archive == nil {
					spools, err := filepath.Glob(filepath.Join(destination, ".atmos-oci-layer-*.tar"))
					require.NoError(t, err)
					require.Len(t, spools, 1)
					var name string
					name, alias = createSpoolCollision(t, kind, destination, spools[0])
					archive = orderedTarLayer(t, []tarLayerEntry{{".wh.victim", ""}, {name, ""}})
				}
				return archive.Read(p)
			})
			err := extractTarball(reader, destination)
			require.ErrorIs(t, err, ErrInvalidFilePath)
			if alias != "" {
				require.NoError(t, os.Remove(alias))
			}
			assert.Equal(t, map[string]string{"victim": "safe"}, extractedFiles(t, destination))
		})
	}
}

func createSpoolCollision(t *testing.T, kind, destination, spool string) (string, string) {
	t.Helper()
	name := filepath.Base(spool)
	switch kind {
	case "entry":
		return name, ""
	case "descendant":
		return name + "/child", ""
	case "whiteout":
		return ".wh." + name, ""
	}
	alias := filepath.Join(destination, "alias")
	switch kind {
	case "directory alias":
		createWhiteoutTestLink(t, os.Symlink, ".", alias)
		return "alias/" + name, alias
	case "symlink":
		createWhiteoutTestLink(t, os.Symlink, name, alias)
	default:
		createWhiteoutTestLink(t, os.Link, spool, alias)
	}
	switch kind {
	case "alias descendant":
		return "alias/child", alias
	case "whiteout alias":
		return ".wh.alias", alias
	default:
		return "alias", alias
	}
}

func TestExtractTarball_RootOpaqueRemovesUnrelatedSpoolNames(t *testing.T) {
	destination := t.TempDir()
	for _, name := range []string{".atmos-oci-layer-old.tar", "atmos-oci-layer-old.tar", "ordinary"} {
		require.NoError(t, os.WriteFile(filepath.Join(destination, name), []byte("lower"), 0o600))
	}
	require.NoError(t, extractTarball(orderedTarLayer(t, []tarLayerEntry{{"new", "upper"}, {".atmos-oci-layer-upper.tar", "payload"}, {".wh..wh..opq", ""}}), destination))
	assert.Equal(t, map[string]string{"new": "upper", ".atmos-oci-layer-upper.tar": "payload"}, extractedFiles(t, destination))
}

func TestPrepareTarLayer_DestinationRenameKeepsCleanupConfined(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("renaming a directory with open handles is not supported on every Windows filesystem")
	}
	parent := t.TempDir()
	destination, moved := filepath.Join(parent, "destination"), filepath.Join(parent, "moved")
	require.NoError(t, os.Mkdir(destination, 0o700))
	root, err := os.OpenRoot(destination)
	require.NoError(t, err)
	defer root.Close()
	layer, err := prepareTarLayer(orderedTarLayer(t, []tarLayerEntry{{"new", "upper"}}), root, destination)
	require.NoError(t, err)
	defer removeTarLayer(layer)
	info, err := layer.Stat()
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
	require.NoError(t, os.Rename(destination, moved))
	require.NoError(t, os.Mkdir(destination, 0o700))
	decoy := filepath.Join(destination, layer.name)
	require.NoError(t, os.WriteFile(decoy, []byte("unrelated"), 0o600))
	removeTarLayer(layer)
	entries, err := os.ReadDir(moved)
	require.NoError(t, err)
	assert.Empty(t, entries)
	content, err := os.ReadFile(decoy)
	require.NoError(t, err)
	assert.Equal(t, "unrelated", string(content))
}

func TestExtractTarball_RechecksSpoolAliasBeforeReplay(t *testing.T) {
	destination := t.TempDir()
	archive := orderedTarLayer(t, []tarLayerEntry{{"alias", ""}})
	alias := filepath.Join(destination, "alias")
	reader := tarSpoolReaderFunc(func(p []byte) (int, error) {
		// The empty entry has been validated when only the two end blocks remain.
		if archive.Len() == 1024 {
			spools, err := filepath.Glob(filepath.Join(destination, ".atmos-oci-layer-*.tar"))
			require.NoError(t, err)
			require.Len(t, spools, 1)
			createWhiteoutTestLink(t, os.Link, spools[0], alias)
		}
		return archive.Read(p)
	})
	err := extractTarball(reader, destination)
	require.ErrorIs(t, err, ErrInvalidFilePath)
	info, err := os.Stat(alias)
	require.NoError(t, err)
	assert.Positive(t, info.Size(), "replay must not truncate the active spool through its alias")
	require.NoError(t, os.Remove(alias))
	entries, err := os.ReadDir(destination)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestTarLayer_ClosedSpoolRejectsPathCheck(t *testing.T) {
	destination := t.TempDir()
	root, err := os.OpenRoot(destination)
	require.NoError(t, err)
	defer root.Close()
	layer, err := prepareTarLayer(orderedTarLayer(t, nil), root, destination)
	require.NoError(t, err)
	defer removeTarLayer(layer)
	require.NoError(t, layer.checkPath("payload"))
	require.NoError(t, layer.Close())
	var pathErr *os.PathError
	require.ErrorAs(t, layer.checkPath("payload"), &pathErr)
	assert.Equal(t, layer.Name(), pathErr.Path)
}

func createWhiteoutTestLink(t *testing.T, makeLink func(string, string) error, target, name string) {
	t.Helper()
	err := makeLink(target, name)
	if err != nil && runtime.GOOS == "windows" {
		t.Skipf("link creation unavailable on this Windows filesystem: %v", err)
	}
	require.NoError(t, err)
}
