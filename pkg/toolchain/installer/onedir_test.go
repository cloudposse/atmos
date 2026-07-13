package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

// ---------------------------------------------------------------------------
// Fixture helpers (support dirs and symlinks, which the shared helpers do not).
// ---------------------------------------------------------------------------

type tarEntry struct {
	name     string
	content  string
	link     string // Non-empty => a symlink entry with this target.
	hardlink string // Non-empty => a hard-link entry with this target (archive-relative).
	dir      bool
	mode     int64
}

// writeTarGzTree builds a tar.gz containing regular files, directories, and
// symlinks, for exercising onedir extraction end to end.
func writeTarGzTree(t *testing.T, path string, entries []tarEntry) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		switch {
		case e.dir:
			require.NoError(t, tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: tar.TypeDir, Mode: 0o755}))
		case e.link != "":
			require.NoError(t, tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: tar.TypeSymlink, Linkname: e.link, Mode: mode}))
		case e.hardlink != "":
			require.NoError(t, tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: tar.TypeLink, Linkname: e.hardlink, Mode: mode}))
		default:
			require.NoError(t, tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: tar.TypeReg, Mode: mode, Size: int64(len(e.content))}))
			_, err := tw.Write([]byte(e.content))
			require.NoError(t, err)
		}
	}
}

type zipEntry struct {
	name    string
	content string
	link    string // Non-empty => a symlink entry with this target.
	mode    os.FileMode
}

// writeZipTree builds a zip containing regular files and symlinks.
func writeZipTree(t *testing.T, path string, entries []zipEntry) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	for _, e := range entries {
		if e.link != "" {
			hdr := &zip.FileHeader{Name: e.name}
			hdr.SetMode(os.ModeSymlink | 0o777)
			fw, err := w.CreateHeader(hdr)
			require.NoError(t, err)
			_, err = fw.Write([]byte(e.link))
			require.NoError(t, err)
			continue
		}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &zip.FileHeader{Name: e.name}
		hdr.SetMode(mode)
		fw, err := w.CreateHeader(hdr)
		require.NoError(t, err)
		_, err = fw.Write([]byte(e.content))
		require.NoError(t, err)
	}
}

func writeFileUnder(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// ---------------------------------------------------------------------------
// The onedir gate.
// ---------------------------------------------------------------------------

func TestShouldPreserveTree(t *testing.T) {
	testCases := []struct {
		name  string
		eps   []entrypoint
		build func(root string)
		want  bool
	}{
		{
			name:  "single binary only stays flat",
			eps:   []entrypoint{{name: "tool", src: "tool"}},
			build: func(root string) { writeFileUnder(t, root, "tool", "bin") },
			want:  false,
		},
		{
			name: "binary plus docs stays flat",
			eps:  []entrypoint{{name: "tool", src: "tool"}},
			build: func(root string) {
				writeFileUnder(t, root, "tool", "bin")
				writeFileUnder(t, root, "LICENSE", "license")
				writeFileUnder(t, root, "README.md", "readme")
				writeFileUnder(t, root, "docs/CHANGELOG.txt", "changes")
			},
			want: false,
		},
		{
			name: "binary plus runtime sibling triggers onedir",
			eps:  []entrypoint{{name: "tool", src: "tool"}},
			build: func(root string) {
				writeFileUnder(t, root, "tool", "bin")
				writeFileUnder(t, root, "libdep.so", "shared-lib")
			},
			want: true,
		},
		{
			name:  "nested entrypoint alone stays flat",
			eps:   []entrypoint{{name: "helm", src: "linux-amd64/helm"}},
			build: func(root string) { writeFileUnder(t, root, "linux-amd64/helm", "bin") },
			want:  false,
		},
		{
			name: "nested entrypoint plus sibling triggers onedir",
			eps:  []entrypoint{{name: "aws", src: "aws/dist/aws"}},
			build: func(root string) {
				writeFileUnder(t, root, "aws/dist/aws", "bin")
				writeFileUnder(t, root, "aws/dist/libpython.so", "shared-lib")
			},
			want: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.build(root)

			got, err := shouldPreserveTree(root, tc.eps)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestIsIgnorableExtraFile(t *testing.T) {
	ignorable := []string{"LICENSE", "LICENSE.txt", "licence", "README", "README.md", "NOTICE", "CHANGELOG", "AUTHORS", "docs/guide.md", "notes.txt"}
	for _, name := range ignorable {
		assert.Truef(t, isIgnorableExtraFile(name), "%q should be ignorable", name)
	}
	notIgnorable := []string{"libpython3.14.so.1.0", "aws", "node", "lib/npm-cli.js", "bin/tool"}
	for _, name := range notIgnorable {
		assert.Falsef(t, isIgnorableExtraFile(name), "%q should NOT be ignorable", name)
	}
}

// ---------------------------------------------------------------------------
// Symlink materialization.
// ---------------------------------------------------------------------------

func TestExtractSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privilege on Windows; onedir Windows support is tracked separately")
	}

	t.Run("creates a relative symlink", func(t *testing.T) {
		dest := t.TempDir()
		link := filepath.Join(dest, "bin", "npm")
		require.NoError(t, extractSymlink(link, "../lib/npm-cli.js", dest))

		info, err := os.Lstat(link)
		require.NoError(t, err)
		require.NotZero(t, info.Mode()&os.ModeSymlink)
		target, err := os.Readlink(link)
		require.NoError(t, err)
		assert.Equal(t, "../lib/npm-cli.js", target)
	})

	t.Run("rejects a relative target that escapes dest", func(t *testing.T) {
		dest := t.TempDir()
		link := filepath.Join(dest, "evil")
		err := extractSymlink(link, "../../../../etc/passwd", dest)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("rejects an absolute target outside dest", func(t *testing.T) {
		dest := t.TempDir()
		link := filepath.Join(dest, "evil")
		err := extractSymlink(link, "/etc/passwd", dest)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("links to a target extracted later (lexical validation)", func(t *testing.T) {
		dest := t.TempDir()
		link := filepath.Join(dest, "bin", "npm")
		// The target does not exist yet (tar ordering); the link must still be created.
		require.NoError(t, extractSymlink(link, "../lib/cli.js", dest))

		// Materialize the target afterwards and confirm the link resolves.
		writeFileUnder(t, dest, "lib/cli.js", "CLI")
		got, err := os.ReadFile(link)
		require.NoError(t, err)
		assert.Equal(t, "CLI", string(got))
	})
}

// ---------------------------------------------------------------------------
// onedir installs (#2743 aws-cli shape, #2744 node shape).
// ---------------------------------------------------------------------------

// TestExtractFilesFromDir_Onedir_PreservesRuntimeSiblings reproduces the #2743
// shape: a bundled binary with a runtime shared-library sibling. The fix must
// preserve the whole tree and co-locate the sibling with the resolved binary.
func TestExtractFilesFromDir_Onedir_PreservesRuntimeSiblings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink entrypoints require privilege on Windows; onedir Windows support is tracked separately")
	}

	tmp := t.TempDir()
	staging := filepath.Join(tmp, "staging")
	writeFileUnder(t, staging, "aws/dist/aws", "MAIN-BINARY")
	writeFileUnder(t, staging, "aws/dist/aws_completer", "COMPLETER")
	writeFileUnder(t, staging, "aws/dist/libpython3.so", "SHARED-LIB") // The dropped-by-old-code sibling.

	versionDir := filepath.Join(tmp, "version")
	binaryPath := filepath.Join(versionDir, "aws")
	inst := &Installer{}
	tool := &registry.Tool{Files: []registry.File{
		{Name: "aws", Src: "aws/dist/aws"},
		{Name: "aws_completer", Src: "aws/dist/aws_completer"},
	}}

	require.NoError(t, inst.extractFilesFromDir(staging, binaryPath, tool))

	// The complete tree is preserved under .pkg, including the runtime sibling.
	assert.FileExists(t, filepath.Join(versionDir, onedirTreeName, "aws", "dist", "libpython3.so"))

	// The primary entrypoint is a symlink resolving to the real binary.
	info, err := os.Lstat(binaryPath)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSymlink, "primary entrypoint must be a symlink")
	got, err := os.ReadFile(binaryPath)
	require.NoError(t, err)
	assert.Equal(t, "MAIN-BINARY", string(got))

	// The additional entrypoint is exposed too.
	completer := filepath.Join(versionDir, "aws_completer")
	cinfo, err := os.Lstat(completer)
	require.NoError(t, err)
	assert.NotZero(t, cinfo.Mode()&os.ModeSymlink)

	// The critical #2743 guarantee: the shared library sits next to the resolved
	// binary, so it loads at runtime.
	resolved, err := filepath.EvalSymlinks(binaryPath)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(filepath.Dir(resolved), "libpython3.so"))
}

// TestInstallOnedirForOS_UsesWindowsExeEntrypoint verifies that Windows Aqua
// registry sources can omit `.exe` while the archive contains it. This mirrors
// nodejs/node's `files[].src` behavior.
func TestInstallOnedirForOS_UsesWindowsExeEntrypoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test creates a symlink without Windows privilege requirements")
	}

	tmp := t.TempDir()
	staging := filepath.Join(tmp, "staging")
	writeFileUnder(t, staging, "node/bin/node.exe", "NODE")
	writeFileUnder(t, staging, "node/lib/runtime.dll", "RUNTIME")

	versionDir := filepath.Join(tmp, "version")
	binaryPath := filepath.Join(versionDir, "node.exe")
	eps := []entrypoint{{name: "node", src: "node/bin/node"}}

	require.NoError(t, (&Installer{}).installOnedirForOS(staging, binaryPath, eps, "windows"))

	info, err := os.Lstat(binaryPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
	got, err := os.ReadFile(binaryPath)
	require.NoError(t, err)
	assert.Equal(t, "NODE", string(got))
}

func TestResolveOnedirEntrypointSourceForOS(t *testing.T) {
	t.Run("returns the Windows exe variant", func(t *testing.T) {
		staging := t.TempDir()
		writeFileUnder(t, staging, "node.exe", "NODE")

		src, err := resolveOnedirEntrypointSourceForOS(staging, "node", "windows")
		require.NoError(t, err)
		assert.Equal(t, "node.exe", src)
	})

	t.Run("fails when neither Windows variant exists", func(t *testing.T) {
		_, err := resolveOnedirEntrypointSourceForOS(t.TempDir(), "node", "windows")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrToolNotFound)
	})
}

// TestInstallOnedir_FailedReinstallPreservesExistingTree verifies that an
// incomplete replacement does not delete a known-good onedir installation.
func TestInstallOnedir_FailedReinstallPreservesExistingTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink entrypoints require privilege on Windows")
	}

	tmp := t.TempDir()
	versionDir := filepath.Join(tmp, "version")
	treeDir := filepath.Join(versionDir, onedirTreeName)
	writeFileUnder(t, treeDir, "old/tool", "KNOWN-GOOD")
	require.NoError(t, os.Symlink(filepath.Join(onedirTreeName, "old", "tool"), filepath.Join(versionDir, "tool")))

	staging := filepath.Join(tmp, "staging")
	writeFileUnder(t, staging, "new/tool", "REPLACEMENT")
	writeFileUnder(t, staging, "new/runtime.so", "RUNTIME")
	// `new/missing-helper` is intentionally absent.
	eps := []entrypoint{
		{name: "tool", src: "new/tool"},
		{name: "helper", src: "new/missing-helper"},
	}

	err := (&Installer{}).installOnedir(staging, filepath.Join(versionDir, "tool"), eps)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrToolNotFound)

	// The old tree and root entrypoint must still be usable after a failed
	// reinstall; replacing .pkg before this validation caused dangling links.
	got, err := os.ReadFile(filepath.Join(versionDir, "tool"))
	require.NoError(t, err)
	assert.Equal(t, "KNOWN-GOOD", string(got))
	assert.FileExists(t, filepath.Join(treeDir, "old", "tool"))
}

// TestExtractTarGz_Onedir_RecreatesSymlinkEntrypoint reproduces the #2744 shape:
// a tar.gz whose npm entrypoint is an in-archive symlink into a sibling lib tree.
// The fix must recreate the symlink (not drop it) and expose a working entrypoint.
func TestExtractTarGz_Onedir_RecreatesSymlinkEntrypoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink entrypoints require privilege on Windows; onedir Windows support is tracked separately")
	}

	tmp := t.TempDir()
	archive := filepath.Join(tmp, "node.tar.gz")
	writeTarGzTree(t, archive, []tarEntry{
		{name: "node-v1.2.3/", dir: true},
		{name: "node-v1.2.3/bin/", dir: true},
		{name: "node-v1.2.3/bin/node", content: "NODE-BINARY", mode: 0o755},
		{name: "node-v1.2.3/bin/npm", link: "../lib/node_modules/npm/bin/npm-cli.js"},
		{name: "node-v1.2.3/lib/", dir: true},
		{name: "node-v1.2.3/lib/node_modules/npm/bin/", dir: true},
		{name: "node-v1.2.3/lib/node_modules/npm/bin/npm-cli.js", content: "NPM-CLI", mode: 0o644},
	})

	versionDir := filepath.Join(tmp, "version")
	binaryPath := filepath.Join(versionDir, "node")
	inst := &Installer{}
	tool := &registry.Tool{Files: []registry.File{
		{Name: "node", Src: "node-v1.2.3/bin/node"},
		{Name: "npm", Src: "node-v1.2.3/bin/npm"},
	}}

	require.NoError(t, inst.extractTarGz(archive, binaryPath, tool))

	// node resolves to the real binary.
	got, err := os.ReadFile(binaryPath)
	require.NoError(t, err)
	assert.Equal(t, "NODE-BINARY", string(got))

	// The in-archive symlink was RECREATED inside the preserved tree (not dropped).
	internal := filepath.Join(versionDir, onedirTreeName, "node-v1.2.3", "bin", "npm")
	linfo, err := os.Lstat(internal)
	require.NoError(t, err)
	require.NotZero(t, linfo.Mode()&os.ModeSymlink, "in-archive symlink must be recreated")

	// npm entrypoint chains through the recreated symlink to the real CLI script.
	got, err = os.ReadFile(filepath.Join(versionDir, "npm"))
	require.NoError(t, err)
	assert.Equal(t, "NPM-CLI", string(got))
}

// TestExtractZip_Onedir_RecreatesSymlinkEntrypoint verifies the same behavior for
// zip archives (aws-cli is distributed as a zip), including zip symlink entries.
func TestExtractZip_Onedir_RecreatesSymlinkEntrypoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink entrypoints require privilege on Windows; onedir Windows support is tracked separately")
	}

	tmp := t.TempDir()
	archive := filepath.Join(tmp, "tool.zip")
	writeZipTree(t, archive, []zipEntry{
		{name: "pkg/dist/tool", content: "TOOL-BINARY", mode: 0o755},
		{name: "pkg/dist/libdep.so", content: "SHARED-LIB", mode: 0o644},
		{name: "pkg/bin/tool", link: "../dist/tool"}, // zip symlink entry.
	})

	versionDir := filepath.Join(tmp, "version")
	binaryPath := filepath.Join(versionDir, "tool")
	inst := &Installer{}
	// The entrypoint is the symlinked launcher, which points at the real binary.
	tool := &registry.Tool{Files: []registry.File{
		{Name: "tool", Src: "pkg/bin/tool"},
	}}

	require.NoError(t, inst.extractZip(archive, binaryPath, tool))

	// Runtime sibling preserved.
	assert.FileExists(t, filepath.Join(versionDir, onedirTreeName, "pkg", "dist", "libdep.so"))
	// Entrypoint resolves through the recreated zip symlink to the real binary.
	got, err := os.ReadFile(binaryPath)
	require.NoError(t, err)
	assert.Equal(t, "TOOL-BINARY", string(got))
}

// TestExtractAndInstall_CleansUpOnFailure verifies that a failed extraction of a
// fresh version does not orphan a partial install (which would otherwise fool
// FindBinaryPath into reporting the tool as installed).
func TestExtractAndInstall_CleansUpOnFailure(t *testing.T) {
	tmp := t.TempDir()
	inst := &Installer{binDir: tmp}

	// A tar.gz whose payload does not contain the configured entrypoint.
	archive := filepath.Join(tmp, "asset.tar.gz")
	createTestTarGzArchive(t, archive, map[string]string{"unrelated-file": "x"})

	tool := &registry.Tool{
		RepoOwner: "acme",
		RepoName:  "tool",
		Files:     []registry.File{{Name: "tool", Src: "does/not/exist"}},
	}

	_, err := inst.extractAndInstall(tool, archive, "1.0.0")
	require.Error(t, err)

	// The version directory must not be left behind.
	_, statErr := os.Stat(filepath.Join(tmp, "acme", "tool", "1.0.0"))
	assert.Truef(t, os.IsNotExist(statErr), "failed install must not orphan a version dir (stat err: %v)", statErr)
}

// TestUninstall_RemovesOnedirTree verifies uninstall removes the whole version
// directory, including the preserved tree and entrypoint symlinks.
func TestUninstall_RemovesOnedirTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink entrypoints require privilege on Windows; onedir Windows support is tracked separately")
	}

	tmp := t.TempDir()
	inst := &Installer{binDir: tmp}

	versionDir := filepath.Join(tmp, "acme", "tool", "1.0.0")
	require.NoError(t, os.MkdirAll(filepath.Join(versionDir, onedirTreeName, "dist"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, onedirTreeName, "dist", "tool"), []byte("BIN"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, onedirTreeName, "dist", "lib.so"), []byte("LIB"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(onedirTreeName, "dist", "tool"), filepath.Join(versionDir, "tool")))

	require.NoError(t, inst.Uninstall("acme", "tool", "1.0.0"))

	_, err := os.Stat(versionDir)
	assert.True(t, os.IsNotExist(err), "onedir version dir must be fully removed")
}

// TestOnedir_ErrorPaths covers the error branches of the onedir helpers using
// filesystem conditions (a file where a directory is expected, missing sources,
// malformed templates) rather than mocks.
func TestOnedir_ErrorPaths(t *testing.T) {
	// pathUnderFile returns a path whose PARENT is a regular file, so any
	// os.MkdirAll of the parent fails.
	pathUnderFile := func(t *testing.T) string {
		t.Helper()
		base := t.TempDir()
		fileAsParent := filepath.Join(base, "iamafile")
		require.NoError(t, os.WriteFile(fileAsParent, []byte("x"), 0o644))
		return filepath.Join(fileAsParent, "child")
	}

	t.Run("resolveEntrypoints defaults src to name", func(t *testing.T) {
		eps, err := (&Installer{}).resolveEntrypoints(&registry.Tool{
			Files: []registry.File{{Name: "tool"}}, // Src omitted.
		})
		require.NoError(t, err)
		require.Len(t, eps, 1)
		assert.Equal(t, "tool", eps[0].src)
	})

	t.Run("resolveEntrypoints propagates template error", func(t *testing.T) {
		_, err := (&Installer{}).resolveEntrypoints(&registry.Tool{
			Files: []registry.File{{Name: "t", Src: "{{ .Bad "}}, // Malformed template.
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("installFlat fails when dest dir cannot be created", func(t *testing.T) {
		err := (&Installer{}).installFlat(t.TempDir(), pathUnderFile(t), []entrypoint{{name: "t", src: "t"}})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("linkEntrypoint fails when parent cannot be created", func(t *testing.T) {
		tgt := filepath.Join(t.TempDir(), "real")
		require.NoError(t, os.WriteFile(tgt, []byte("x"), 0o755))
		err := linkEntrypoint(tgt, pathUnderFile(t))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("extractSymlink fails when parent cannot be created", func(t *testing.T) {
		base := t.TempDir()
		fileAsParent := filepath.Join(base, "iamafile")
		require.NoError(t, os.WriteFile(fileAsParent, []byte("x"), 0o644))
		err := extractSymlink(filepath.Join(fileAsParent, "child"), "target", base)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("extractHardLink fails when parent cannot be created", func(t *testing.T) {
		base := t.TempDir()
		writeFileUnder(t, base, "real", "data")
		fileAsParent := filepath.Join(base, "iamafile")
		require.NoError(t, os.WriteFile(fileAsParent, []byte("x"), 0o644))
		err := extractHardLink(filepath.Join(fileAsParent, "child"), "real", base)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("copyRegularFile fails on missing source", func(t *testing.T) {
		err := copyRegularFile(filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "dst"), 0o644)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("copyRegularFile fails when dest parent cannot be created", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "src")
		require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
		err := copyRegularFile(src, pathUnderFile(t), 0o644)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("copyTree fails on missing source", func(t *testing.T) {
		err := copyTree(filepath.Join(t.TempDir(), "nope"), t.TempDir())
		require.Error(t, err)
	})

	t.Run("newStagingDir fails when version dir cannot be created", func(t *testing.T) {
		_, err := newStagingDir(pathUnderFile(t))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("extractTarGz fails when staging dir cannot be created", func(t *testing.T) {
		archive := filepath.Join(t.TempDir(), "a.tar.gz")
		createTestTarGzArchive(t, archive, map[string]string{"x": "y"})
		err := (&Installer{}).extractTarGz(archive, pathUnderFile(t), &registry.Tool{Name: "x"})
		require.Error(t, err)
	})

	t.Run("extractZip fails when staging dir cannot be created", func(t *testing.T) {
		archive := filepath.Join(t.TempDir(), "a.zip")
		createTestZipArchive(t, archive, map[string]string{"x": "y"})
		err := (&Installer{}).extractZip(archive, pathUnderFile(t), &registry.Tool{Name: "x"})
		require.Error(t, err)
	})
}

// TestExtractSymlinkForOS_SymlinkFailure exercises the branch taken when the
// underlying os.Symlink call fails (forced here by a pre-existing non-empty
// directory at the link path): Windows skips with a warning, other platforms
// return an error.
func TestExtractSymlinkForOS_SymlinkFailure(t *testing.T) {
	newBlockedLink := func(t *testing.T) (dest, linkPath string) {
		t.Helper()
		dest = t.TempDir()
		linkPath = filepath.Join(dest, "occupied")
		// A non-empty directory at linkPath makes os.Remove + os.Symlink fail.
		require.NoError(t, os.MkdirAll(filepath.Join(linkPath, "child"), 0o755))
		return dest, linkPath
	}

	t.Run("windows skips with warning", func(t *testing.T) {
		dest, linkPath := newBlockedLink(t)
		assert.NoError(t, extractSymlinkForOS(linkPath, "target", dest, "windows"))
	})

	t.Run("non-windows returns an error", func(t *testing.T) {
		dest, linkPath := newBlockedLink(t)
		err := extractSymlinkForOS(linkPath, "target", dest, "linux")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})
}

// TestMoveEntrypointFileForOS_Windows covers the Windows `.exe` handling: an
// archive entry named without the extension is resolved to the `.exe` file.
func TestMoveEntrypointFileForOS_Windows(t *testing.T) {
	t.Run("resolves the .exe variant", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "tool") // Does not exist without .exe.
		require.NoError(t, os.WriteFile(src+windowsExeExt, []byte("BINARY"), 0o755))
		dst := filepath.Join(tmp, "out", "tool")

		require.NoError(t, moveEntrypointFileForOS(src, dst, "windows"))

		got, err := os.ReadFile(dst + windowsExeExt)
		require.NoError(t, err)
		assert.Equal(t, "BINARY", string(got))
	})

	t.Run("returns not-found when neither variant exists", func(t *testing.T) {
		tmp := t.TempDir()
		err := moveEntrypointFileForOS(filepath.Join(tmp, "ghost"), filepath.Join(tmp, "out", "ghost"), "windows")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrToolNotFound)
	})
}

func TestShouldPreserveTree_WalkError(t *testing.T) {
	// A nonexistent root makes WalkDir invoke the callback with an error, which
	// must propagate rather than silently returning false.
	_, err := shouldPreserveTree(filepath.Join(t.TempDir(), "does-not-exist"), []entrypoint{{name: "t", src: "t"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFileOperation)
}

func TestExtractFilesFromDir_ErrorPaths(t *testing.T) {
	t.Run("propagates entrypoint template errors", func(t *testing.T) {
		err := (&Installer{}).extractFilesFromDir(
			t.TempDir(),
			filepath.Join(t.TempDir(), "bin"),
			&registry.Tool{Files: []registry.File{{Name: "t", Src: "{{ .Bad "}}},
		)
		require.Error(t, err)
	})

	t.Run("propagates gate inspection errors", func(t *testing.T) {
		err := (&Installer{}).extractFilesFromDir(
			filepath.Join(t.TempDir(), "does-not-exist"),
			filepath.Join(t.TempDir(), "bin"),
			&registry.Tool{Files: []registry.File{{Name: "t", Src: "t"}}},
		)
		require.Error(t, err)
	})
}

func TestCopyRegularFile_OpenFileError(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
	// The destination is an existing directory, so OpenFile for writing fails.
	dstDir := t.TempDir()
	err := copyRegularFile(src, dstDir, 0o644)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFileOperation)
}

// TestExtractTarGz_PreservesHardLink verifies the tar TypeLink dispatch path in
// extractEntry recreates hard links end to end.
func TestExtractTarGz_PreservesHardLink(t *testing.T) {
	tmp := t.TempDir()
	archive := filepath.Join(tmp, "tool.tar.gz")
	writeTarGzTree(t, archive, []tarEntry{
		{name: "pkg/", dir: true},
		{name: "pkg/real", content: "PAYLOAD", mode: 0o755},
		{name: "pkg/hardlink", hardlink: "pkg/real"},
		{name: "pkg/extra.so", content: "LIB"}, // Forces onedir mode.
	})

	versionDir := filepath.Join(tmp, "version")
	binaryPath := filepath.Join(versionDir, "real")
	tool := &registry.Tool{Files: []registry.File{{Name: "real", Src: "pkg/real"}}}

	require.NoError(t, (&Installer{}).extractTarGz(archive, binaryPath, tool))

	// The hard link was materialized in the preserved tree with the same content.
	got, err := os.ReadFile(filepath.Join(versionDir, onedirTreeName, "pkg", "hardlink"))
	require.NoError(t, err)
	assert.Equal(t, "PAYLOAD", string(got))
}

func TestExtractHardLink(t *testing.T) {
	t.Run("links to an existing target", func(t *testing.T) {
		dest := t.TempDir()
		writeFileUnder(t, dest, "real.bin", "DATA")

		linkPath := filepath.Join(dest, "hard.bin")
		require.NoError(t, extractHardLink(linkPath, "real.bin", dest))

		got, err := os.ReadFile(linkPath)
		require.NoError(t, err)
		assert.Equal(t, "DATA", string(got))
	})

	t.Run("rejects a target that escapes dest", func(t *testing.T) {
		dest := t.TempDir()
		err := extractHardLink(filepath.Join(dest, "h"), "../../../../etc/passwd", dest)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("skips when the target is missing", func(t *testing.T) {
		dest := t.TempDir()
		linkPath := filepath.Join(dest, "h")
		// A missing hard-link target is skipped (not fatal) rather than crashing.
		require.NoError(t, extractHardLink(linkPath, "not-there", dest))
		_, err := os.Lstat(linkPath)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestCopyTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink copy requires privilege on Windows; onedir Windows support is tracked separately")
	}

	src := t.TempDir()
	writeFileUnder(t, src, "dir/file.txt", "CONTENT")
	require.NoError(t, os.Chmod(filepath.Join(src, "dir", "file.txt"), 0o755))
	require.NoError(t, os.Symlink("file.txt", filepath.Join(src, "dir", "link")))

	dst := filepath.Join(t.TempDir(), "copy")
	require.NoError(t, copyTree(src, dst))

	// Regular file copied with its mode preserved.
	got, err := os.ReadFile(filepath.Join(dst, "dir", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "CONTENT", string(got))
	info, err := os.Stat(filepath.Join(dst, "dir", "file.txt"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o100, "executable bit should be preserved")

	// Symlink copied as a symlink.
	linfo, err := os.Lstat(filepath.Join(dst, "dir", "link"))
	require.NoError(t, err)
	assert.NotZero(t, linfo.Mode()&os.ModeSymlink)
}

// TestMoveTree_CopyFallback forces the cross-filesystem branch of moveTree by
// renaming onto a non-empty destination (which fails), exercising the recursive
// copy fallback used when the staging dir is on a different device (e.g. .pkg).
func TestMoveTree_CopyFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink copy requires privilege on Windows; onedir Windows support is tracked separately")
	}

	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")

	writeFileUnder(t, src, "a/b.bin", "PAYLOAD")
	require.NoError(t, os.Symlink("b.bin", filepath.Join(src, "a", "link")))

	// A pre-existing, non-empty destination makes os.Rename fail, forcing the copy.
	require.NoError(t, os.MkdirAll(dst, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dst, "occupied"), []byte("z"), 0o644))

	require.NoError(t, moveTree(src, dst))

	// The source is removed and the tree is present at the destination.
	_, err := os.Stat(src)
	assert.True(t, os.IsNotExist(err), "source staging dir must be removed after copy fallback")
	got, err := os.ReadFile(filepath.Join(dst, "a", "b.bin"))
	require.NoError(t, err)
	assert.Equal(t, "PAYLOAD", string(got))
	linfo, err := os.Lstat(filepath.Join(dst, "a", "link"))
	require.NoError(t, err)
	assert.NotZero(t, linfo.Mode()&os.ModeSymlink)
}

// TestExtraction_FlatLayout_SingleBinary_Unchanged pins the invariant that a
// single-binary archive (no extra runtime payload) installs as a flat, regular
// file directly in the version dir, with no preserved-tree ("onedir") directory.
//
// This is a characterization test for the reduced-blast-radius guarantee: the
// onedir change must NOT alter how simple, single-binary tools (jq, terraform,
// kubectl, ...) are laid out on disk. It passes on the pre-change code (which
// always flattens) and must keep passing after the onedir change (the gate must
// keep single-file archives flat).
func TestExtraction_FlatLayout_SingleBinary_Unchanged(t *testing.T) {
	const script = "#!/bin/sh\necho hello\n"

	testCases := []struct {
		name    string
		ext     string
		build   func(t *testing.T, path string, files map[string]string)
		extract func(i *Installer, archivePath, binaryPath string, tool *registry.Tool) error
	}{
		{
			name:  "tar.gz",
			ext:   "tar.gz",
			build: createTestTarGzArchive,
			extract: func(i *Installer, archivePath, binaryPath string, tool *registry.Tool) error {
				return i.extractTarGz(archivePath, binaryPath, tool)
			},
		},
		{
			name:  "zip",
			ext:   "zip",
			build: createTestZipArchive,
			extract: func(i *Installer, archivePath, binaryPath string, tool *registry.Tool) error {
				return i.extractZip(archivePath, binaryPath, tool)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			archivePath := filepath.Join(tmpDir, "archive."+tc.ext)
			versionDir := filepath.Join(tmpDir, "version")
			binaryPath := filepath.Join(versionDir, "mytool")

			// Single-binary archive: the only payload is the binary itself.
			tc.build(t, archivePath, map[string]string{"mytool": script})

			installer := &Installer{}
			tool := &registry.Tool{Name: "mytool"}

			require.NoError(t, tc.extract(installer, archivePath, binaryPath, tool))

			// The binary must be a REGULAR file (not a symlink into a tree).
			info, err := os.Lstat(binaryPath)
			require.NoError(t, err)
			require.True(t, info.Mode().IsRegular(),
				"flat mode: binary must be a regular file, got mode %s", info.Mode())

			// Content must round-trip.
			got, err := os.ReadFile(binaryPath)
			require.NoError(t, err)
			require.Equal(t, script, string(got))

			// Flat mode must NOT create a preserved-tree (.pkg) directory.
			_, err = os.Stat(filepath.Join(versionDir, onedirTreeName))
			require.True(t, os.IsNotExist(err),
				"flat mode must not create a %q tree directory", onedirTreeName)

			// The version dir must contain exactly the one entrypoint.
			entries, err := os.ReadDir(versionDir)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			require.Equal(t, "mytool", entries[0].Name())
		})
	}
}
