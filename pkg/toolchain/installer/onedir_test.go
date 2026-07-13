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
// Sidecar manifest / real-path exposure (no Atmos-created symlinks).
//
// Atmos avoids symlinks by design (see the onedirTreeName doc). These tests
// run on every platform, including a real windows-latest CI runner (Atmos
// already runs `go test ./...` there via the acceptance-test job), so they are
// NOT Windows-skipped.
// ---------------------------------------------------------------------------

func TestOnedirManifest_WriteReadRoundtrip(t *testing.T) {
	versionDir := t.TempDir()
	manifest := onedirManifest{
		Entrypoints: map[string]string{
			"node": filepath.Join(onedirTreeName, "node-v1.2.3", "bin", "node"),
			"npm":  filepath.Join(onedirTreeName, "node-v1.2.3", "bin", "npm"),
		},
		Primary: "node",
	}

	require.NoError(t, writeOnedirManifest(versionDir, manifest))

	got, ok := readOnedirManifest(versionDir)
	require.True(t, ok)
	assert.Equal(t, manifest, got)
}

func TestWriteOnedirManifest_ErrorPaths(t *testing.T) {
	t.Run("fails when the version dir cannot be created", func(t *testing.T) {
		base := t.TempDir()
		fileAsParent := filepath.Join(base, "iamafile")
		require.NoError(t, os.WriteFile(fileAsParent, []byte("x"), 0o644))

		err := writeOnedirManifest(filepath.Join(fileAsParent, "child"), onedirManifest{Primary: "tool"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("fails when the manifest path is occupied by a directory", func(t *testing.T) {
		versionDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(versionDir, onedirManifestName), 0o755))

		err := writeOnedirManifest(versionDir, onedirManifest{Primary: "tool"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})
}

func TestReadOnedirManifest_AbsentOrCorrupt(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, ok := readOnedirManifest(t.TempDir())
		assert.False(t, ok, "a flat install (no manifest file) must report false")
	})

	t.Run("corrupt JSON", func(t *testing.T) {
		versionDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(versionDir, onedirManifestName), []byte("{not json"), 0o644))
		_, ok := readOnedirManifest(versionDir)
		assert.False(t, ok)
	})
}

func TestVersionDirFromBinaryPath(t *testing.T) {
	t.Run("flat install: no manifest anywhere, returns the immediate parent", func(t *testing.T) {
		binDir := t.TempDir()
		versionDir := filepath.Join(binDir, "hashicorp", "terraform", "1.9.8")
		binaryPath := filepath.Join(versionDir, "terraform")

		assert.Equal(t, versionDir, versionDirFromBinaryPath(binDir, binaryPath))
	})

	t.Run("onedir install: walks up to the directory holding the manifest", func(t *testing.T) {
		binDir := t.TempDir()
		versionDir := filepath.Join(binDir, "nodejs", "node", "24.18.0")
		require.NoError(t, writeOnedirManifest(versionDir, onedirManifest{Primary: "node"}))
		// The resolved binary lives several levels deep inside the preserved tree.
		binaryPath := filepath.Join(versionDir, onedirTreeName, "node-v24.18.0-darwin-arm64", "bin", "node")

		assert.Equal(t, versionDir, versionDirFromBinaryPath(binDir, binaryPath))
	})

	t.Run("legacy alternative-path layout: no manifest, falls back to immediate parent", func(t *testing.T) {
		binDir := t.TempDir()
		// This mirrors FindBinaryPath's legacy "<binDir>/<version>/<name>" layout
		// (no owner/repo nesting), which predates onedir and has no manifest.
		versionDir := filepath.Join(binDir, "1.9.8")
		binaryPath := filepath.Join(versionDir, "terraform")

		assert.Equal(t, versionDir, versionDirFromBinaryPath(binDir, binaryPath))
	})

	t.Run("binaryPath outside binDir walks to the filesystem root and falls back", func(t *testing.T) {
		// binDir is NOT an ancestor of binaryPath at all, so the upward walk
		// never encounters it and must stop at the filesystem root instead of
		// looping forever, still returning the safe fallback.
		binDir := filepath.Join(t.TempDir(), "unrelated-bindir")
		binaryPath := filepath.Join(t.TempDir(), "elsewhere", "tool")

		assert.Equal(t, filepath.Dir(binaryPath), versionDirFromBinaryPath(binDir, binaryPath))
	})
}

func TestGetBinaryPath_OnedirManifest(t *testing.T) {
	t.Run("explicit binaryName is unaffected by a manifest", func(t *testing.T) {
		binDir := t.TempDir()
		inst := &Installer{binDir: binDir}
		versionDir := filepath.Join(binDir, "nodejs", "node", "24.18.0")
		require.NoError(t, writeOnedirManifest(versionDir, onedirManifest{
			Entrypoints: map[string]string{"node": filepath.Join(onedirTreeName, "bin", "node")},
			Primary:     "node",
		}))

		got := inst.GetBinaryPath("nodejs", "node", "24.18.0", "node")
		assert.Equal(t, filepath.Join(versionDir, "node"), got, "an explicit binaryName must bypass manifest resolution")
	})

	t.Run("empty binaryName resolves the primary entrypoint via the manifest", func(t *testing.T) {
		binDir := t.TempDir()
		inst := &Installer{binDir: binDir}
		versionDir := filepath.Join(binDir, "nodejs", "node", "24.18.0")
		require.NoError(t, writeOnedirManifest(versionDir, onedirManifest{
			Entrypoints: map[string]string{
				"node": filepath.Join(onedirTreeName, "node-v24.18.0", "bin", "node"),
				"npm":  filepath.Join(onedirTreeName, "node-v24.18.0", "bin", "npm"),
			},
			Primary: "node",
		}))

		got := inst.GetBinaryPath("nodejs", "node", "24.18.0", "")
		assert.Equal(t, filepath.Join(versionDir, onedirTreeName, "node-v24.18.0", "bin", "node"), got)
	})

	t.Run("manifest missing the primary key falls through to auto-detect", func(t *testing.T) {
		binDir := t.TempDir()
		inst := &Installer{binDir: binDir}
		versionDir := filepath.Join(binDir, "acme", "tool", "1.0.0")
		// A manifest that (incorrectly) omits its declared Primary from
		// Entrypoints must not panic; GetBinaryPath falls through to the
		// existing auto-detect/fallback behavior.
		require.NoError(t, writeOnedirManifest(versionDir, onedirManifest{
			Entrypoints: map[string]string{"other": "somewhere"},
			Primary:     "tool",
		}))

		got := inst.GetBinaryPath("acme", "tool", "1.0.0", "")
		assert.Equal(t, filepath.Join(versionDir, "tool"), got, "falls back to the repo-name fallback path")
	})

	t.Run("flat install (no manifest) is unaffected", func(t *testing.T) {
		binDir := t.TempDir()
		inst := &Installer{binDir: binDir}

		got := inst.GetBinaryPath("hashicorp", "terraform", "1.9.8", "")
		assert.Equal(t, filepath.Join(binDir, "hashicorp", "terraform", "1.9.8", "terraform"), got)
	})
}

// TestOnedirInstall_NoSymlinksInVersionDirRoot is an explicit regression
// tripwire for the exact concern raised in review: Atmos must not create ANY
// symlink to expose onedir entrypoints. It installs an aws-cli-shaped and a
// node-shaped (with an in-archive symlink) package and asserts that no
// symlink exists anywhere directly under the version-dir root — only inside
// the preserved .pkg tree (where the archive's OWN symlinks may legitimately
// live, reproduced by extractSymlink).
func TestOnedirInstall_NoSymlinksInVersionDirRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this fixture's node archive carries a real in-archive symlink for npm, requiring symlink creation to build the fixture on Unix; Windows node archives ship .cmd files instead, so the scenario does not apply there")
	}

	tmp := t.TempDir()
	archive := filepath.Join(tmp, "node.tar.gz")
	writeTarGzTree(t, archive, []tarEntry{
		{name: "node-v1.2.3/bin/node", content: "NODE-BINARY", mode: 0o755},
		{name: "node-v1.2.3/bin/npm", link: "../lib/node_modules/npm/bin/npm-cli.js"},
		{name: "node-v1.2.3/lib/node_modules/npm/bin/npm-cli.js", content: "NPM-CLI", mode: 0o644},
	})

	versionDir := filepath.Join(tmp, "version")
	binaryPath := filepath.Join(versionDir, "node")
	tool := &registry.Tool{Files: []registry.File{
		{Name: "node", Src: "node-v1.2.3/bin/node"},
		{Name: "npm", Src: "node-v1.2.3/bin/npm"},
	}}
	require.NoError(t, (&Installer{}).extractTarGz(archive, binaryPath, tool))

	entries, err := os.ReadDir(versionDir)
	require.NoError(t, err)
	for _, e := range entries {
		info, err := os.Lstat(filepath.Join(versionDir, e.Name()))
		require.NoError(t, err)
		assert.Zerof(t, info.Mode()&os.ModeSymlink,
			"Atmos must not create a symlink at the version-dir root: found one at %q", e.Name())
	}
	// Sanity: the manifest (not a symlink) is what's actually there.
	assert.FileExists(t, filepath.Join(versionDir, onedirManifestName))
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

// TestExtractSymlinkForOS_Validation covers the single symlink-creation sink's
// containment guards (what CodeQL flags as "arbitrary file write via archive
// symlinks"). The rejection cases return before any filesystem mutation, so
// they run on every platform (no symlink privilege needed); the success cases
// assert the re-derived relative target and are Unix-only.
func TestExtractSymlinkForOS_Validation(t *testing.T) {
	t.Run("rejects an empty target", func(t *testing.T) {
		root := t.TempDir()
		err := extractSymlinkForOS(filepath.Join(root, "link"), "", root, runtime.GOOS)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("rejects an absolute target", func(t *testing.T) {
		root := t.TempDir()
		err := extractSymlinkForOS(filepath.Join(root, "link"), filepath.FromSlash("/etc/passwd"), root, runtime.GOOS)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("rejects a target escaping root", func(t *testing.T) {
		root := t.TempDir()
		err := extractSymlinkForOS(filepath.Join(root, "node", "bin", "npm"), "../../../../etc/passwd", root, runtime.GOOS)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})

	t.Run("rejects a link path outside root", func(t *testing.T) {
		root := t.TempDir()
		// The symlink's own location is outside root (sibling of root).
		outside := filepath.Join(filepath.Dir(root), "outside-link")
		err := extractSymlinkForOS(outside, "x", root, runtime.GOOS)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
		_, statErr := os.Lstat(outside)
		assert.True(t, os.IsNotExist(statErr), "no link may be created outside root")
	})

	t.Run("creates a re-derived relative link for a valid target", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privilege on Windows")
		}
		root := t.TempDir()
		link := filepath.Join(root, "node", "bin", "npm")
		require.NoError(t, extractSymlinkForOS(link, "../lib/npm-cli.js", root, runtime.GOOS))
		got, err := os.Readlink(link)
		require.NoError(t, err)
		assert.Equal(t, filepath.FromSlash("../lib/npm-cli.js"), got)
	})

	t.Run("cleans a redundant in-root target", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privilege on Windows")
		}
		root := t.TempDir()
		link := filepath.Join(root, "node", "bin", "npm")
		require.NoError(t, extractSymlinkForOS(link, "./sub/../other", root, runtime.GOOS))
		got, err := os.Readlink(link)
		require.NoError(t, err)
		assert.Equal(t, "other", got)
	})
}

// TestExtractTarGz_RejectsSymlinkEscapingRoot is an end-to-end security
// regression: a tar containing a symlink whose target escapes the extraction
// root must be rejected, and nothing may be written outside the destination.
func TestExtractTarGz_RejectsSymlinkEscapingRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privilege on Windows; the guard is covered on all platforms by TestSafeSymlinkTarget")
	}

	tmp := t.TempDir()
	outside := filepath.Join(tmp, "outside")
	require.NoError(t, os.MkdirAll(outside, 0o755))

	archive := filepath.Join(tmp, "evil.tar.gz")
	writeTarGzTree(t, archive, []tarEntry{
		{name: "pkg/tool", content: "BIN", mode: 0o755},
		{name: "pkg/extra.so", content: "LIB"}, // forces onedir mode
		{name: "pkg/escape", link: "../../../outside/pwned"},
	})

	versionDir := filepath.Join(tmp, "version")
	binaryPath := filepath.Join(versionDir, "tool")
	tool := &registry.Tool{Files: []registry.File{{Name: "tool", Src: "pkg/tool"}}}

	err := (&Installer{}).extractTarGz(archive, binaryPath, tool)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFileOperation)

	// Nothing was created at the escape destination.
	_, statErr := os.Lstat(filepath.Join(outside, "pwned"))
	assert.True(t, os.IsNotExist(statErr), "escaping symlink must not be created outside the destination")
}

// ---------------------------------------------------------------------------
// onedir installs (#2743 aws-cli shape, #2744 node shape).
// ---------------------------------------------------------------------------

// TestExtractFilesFromDir_Onedir_PreservesRuntimeSiblings reproduces the #2743
// shape: a bundled binary with a runtime shared-library sibling. The fix must
// preserve the whole tree and co-locate the sibling with the resolved binary.
//
// This also verifies the exposure mechanism itself: Atmos creates NO symlink
// (or any file) at the root entrypoint path — symlinks are avoided by design
// (see the onedirTreeName doc) — so this test needs no Windows skip.
func TestExtractFilesFromDir_Onedir_PreservesRuntimeSiblings(t *testing.T) {
	tmp := t.TempDir()
	staging := filepath.Join(tmp, "staging")
	writeFileUnder(t, staging, "aws/dist/aws", "MAIN-BINARY")
	writeFileUnder(t, staging, "aws/dist/aws_completer", "COMPLETER")
	writeFileUnder(t, staging, "aws/dist/libpython3.so", "SHARED-LIB") // The dropped-by-old-code sibling.

	inst := &Installer{binDir: tmp}
	versionDir := filepath.Join(tmp, "aws", "aws-cli", "2.35.15")
	binaryPath := filepath.Join(versionDir, "aws")
	tool := &registry.Tool{Files: []registry.File{
		{Name: "aws", Src: "aws/dist/aws"},
		{Name: "aws_completer", Src: "aws/dist/aws_completer"},
	}}

	require.NoError(t, inst.extractFilesFromDir(staging, binaryPath, tool))

	// The complete tree is preserved under .pkg, including the runtime sibling.
	assert.FileExists(t, filepath.Join(versionDir, onedirTreeName, "aws", "dist", "libpython3.so"))

	// No symlink (or anything else) is created at the root entrypoint path.
	_, lerr := os.Lstat(binaryPath)
	assert.True(t, os.IsNotExist(lerr), "onedir must not create anything at the root entrypoint path")

	// Exposure goes through the sidecar manifest instead.
	manifest, ok := readOnedirManifest(versionDir)
	require.True(t, ok, "onedir install must write a manifest")
	assert.Equal(t, "aws", manifest.Primary)
	assert.Equal(t, filepath.Join(onedirTreeName, "aws", "dist", "aws"), manifest.Entrypoints["aws"])
	assert.Equal(t, filepath.Join(onedirTreeName, "aws", "dist", "aws_completer"), manifest.Entrypoints["aws_completer"])

	// GetBinaryPath resolves the primary entrypoint to its real, executable path.
	resolved := inst.GetBinaryPath("aws", "aws-cli", "2.35.15", "")
	got, err := os.ReadFile(resolved)
	require.NoError(t, err)
	assert.Equal(t, "MAIN-BINARY", string(got))

	// The critical #2743 guarantee: the shared library sits next to the resolved
	// binary, so it loads at runtime.
	assert.FileExists(t, filepath.Join(filepath.Dir(resolved), "libpython3.so"))
}

// TestInstallOnedirForOS_UsesWindowsExeEntrypoint verifies that Windows Aqua
// registry sources can omit `.exe` while the archive contains it. This mirrors
// nodejs/node's `files[].src` behavior. No symlink privilege is needed (or
// used) for this, so it runs on every platform, including a real Windows
// runner in CI.
func TestInstallOnedirForOS_UsesWindowsExeEntrypoint(t *testing.T) {
	tmp := t.TempDir()
	staging := filepath.Join(tmp, "staging")
	writeFileUnder(t, staging, "node/bin/node.exe", "NODE")
	writeFileUnder(t, staging, "node/lib/runtime.dll", "RUNTIME")

	versionDir := filepath.Join(tmp, "version")
	binaryPath := filepath.Join(versionDir, "node.exe")
	eps := []entrypoint{{name: "node", src: "node/bin/node"}}

	require.NoError(t, (&Installer{}).installOnedirForOS(staging, binaryPath, eps, "windows"))

	// No symlink/file is created at the root entrypoint path.
	_, lerr := os.Lstat(binaryPath)
	assert.True(t, os.IsNotExist(lerr))

	manifest, ok := readOnedirManifest(versionDir)
	require.True(t, ok)
	assert.Equal(t, "node", manifest.Primary)
	assert.Equal(t, filepath.Join(onedirTreeName, "node", "bin", "node.exe"), manifest.Entrypoints["node"])

	got, err := os.ReadFile(filepath.Join(versionDir, manifest.Entrypoints["node"]))
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
// Uses no symlinks (the existing install is represented purely via the
// manifest + tree, matching how a real prior install would look), so this
// needs no Windows skip.
func TestInstallOnedir_FailedReinstallPreservesExistingTree(t *testing.T) {
	tmp := t.TempDir()
	versionDir := filepath.Join(tmp, "version")
	treeDir := filepath.Join(versionDir, onedirTreeName)
	writeFileUnder(t, treeDir, "old/tool", "KNOWN-GOOD")
	require.NoError(t, writeOnedirManifest(versionDir, onedirManifest{
		Entrypoints: map[string]string{"tool": filepath.Join(onedirTreeName, "old", "tool")},
		Primary:     "tool",
	}))

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

	// The old tree and manifest must still be intact after a failed reinstall;
	// replacing .pkg before validating every entrypoint caused this to break.
	manifest, ok := readOnedirManifest(versionDir)
	require.True(t, ok, "manifest must survive a failed reinstall")
	assert.Equal(t, "tool", manifest.Primary)
	got, err := os.ReadFile(filepath.Join(versionDir, manifest.Entrypoints["tool"]))
	require.NoError(t, err)
	assert.Equal(t, "KNOWN-GOOD", string(got))
	assert.FileExists(t, filepath.Join(treeDir, "old", "tool"))
}

// TestInstallOnedir_MoveFailureRestoresExistingTree verifies the atomic-reinstall
// rollback: if the tree move fails AFTER validation (disk full, interrupted
// cross-device copy, ...), the previously-installed tree is restored rather than
// left destroyed. The failure is injected via moveTreeFunc so the rollback path
// is exercised deterministically on every platform.
func TestInstallOnedir_MoveFailureRestoresExistingTree(t *testing.T) {
	tmp := t.TempDir()
	versionDir := filepath.Join(tmp, "version")
	treeDir := filepath.Join(versionDir, onedirTreeName)
	writeFileUnder(t, treeDir, "old/tool", "KNOWN-GOOD")
	require.NoError(t, writeOnedirManifest(versionDir, onedirManifest{
		Entrypoints: map[string]string{"tool": filepath.Join(onedirTreeName, "old", "tool")},
		Primary:     "tool",
	}))

	staging := filepath.Join(tmp, "staging")
	writeFileUnder(t, staging, "new/tool", "REPLACEMENT")
	eps := []entrypoint{{name: "tool", src: "new/tool"}} // valid: passes entrypoint validation.

	// Force the move to fail after the existing tree has been renamed aside.
	orig := moveTreeFunc
	t.Cleanup(func() { moveTreeFunc = orig })
	moveTreeFunc = func(src, dst string) error { return assert.AnError }

	err := (&Installer{}).installOnedir(staging, filepath.Join(versionDir, "tool"), eps)
	require.Error(t, err)

	// The known-good tree must be restored and no backup residue left behind.
	got, err := os.ReadFile(filepath.Join(treeDir, "old", "tool"))
	require.NoError(t, err)
	assert.Equal(t, "KNOWN-GOOD", string(got))
	_, statErr := os.Stat(treeDir + onedirBackupSuffix)
	assert.True(t, os.IsNotExist(statErr), "backup must not be left behind after rollback")

	// The pre-existing manifest is untouched and still resolves.
	manifest, ok := readOnedirManifest(versionDir)
	require.True(t, ok)
	assert.Equal(t, "tool", manifest.Primary)
}

// TestInstallOnedir_ReinstallReplacesTreeAndClearsBackup verifies the happy-path
// reinstall over an existing tree: the new tree is installed, the old one is
// gone, and no backup directory residue remains.
func TestInstallOnedir_ReinstallReplacesTreeAndClearsBackup(t *testing.T) {
	tmp := t.TempDir()
	versionDir := filepath.Join(tmp, "version")
	treeDir := filepath.Join(versionDir, onedirTreeName)
	writeFileUnder(t, treeDir, "old/tool", "OLD")
	require.NoError(t, writeOnedirManifest(versionDir, onedirManifest{
		Entrypoints: map[string]string{"tool": filepath.Join(onedirTreeName, "old", "tool")},
		Primary:     "tool",
	}))

	staging := filepath.Join(tmp, "staging")
	writeFileUnder(t, staging, "new/tool", "NEW")
	eps := []entrypoint{{name: "tool", src: "new/tool"}}

	require.NoError(t, (&Installer{}).installOnedir(staging, filepath.Join(versionDir, "tool"), eps))

	// New tree in place, old content gone, no backup residue.
	assert.NoFileExists(t, filepath.Join(treeDir, "old", "tool"))
	assert.FileExists(t, filepath.Join(treeDir, "new", "tool"))
	_, statErr := os.Stat(treeDir + onedirBackupSuffix)
	assert.True(t, os.IsNotExist(statErr), "backup must be cleared after a successful reinstall")

	manifest, ok := readOnedirManifest(versionDir)
	require.True(t, ok)
	got, err := os.ReadFile(filepath.Join(versionDir, manifest.Entrypoints["tool"]))
	require.NoError(t, err)
	assert.Equal(t, "NEW", string(got))
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

	// No root symlink/file is created; entrypoints are exposed via the manifest.
	manifest, ok := readOnedirManifest(versionDir)
	require.True(t, ok)

	// node resolves to the real binary.
	got, err := os.ReadFile(filepath.Join(versionDir, manifest.Entrypoints["node"]))
	require.NoError(t, err)
	assert.Equal(t, "NODE-BINARY", string(got))

	// The in-archive symlink was RECREATED inside the preserved tree (not dropped).
	internal := filepath.Join(versionDir, onedirTreeName, "node-v1.2.3", "bin", "npm")
	linfo, err := os.Lstat(internal)
	require.NoError(t, err)
	require.NotZero(t, linfo.Mode()&os.ModeSymlink, "in-archive symlink must be recreated")

	// npm entrypoint chains through the recreated symlink to the real CLI script.
	got, err = os.ReadFile(filepath.Join(versionDir, manifest.Entrypoints["npm"]))
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

	// Entrypoint resolves via the manifest, chaining through the recreated zip
	// symlink to the real binary.
	manifest, ok := readOnedirManifest(versionDir)
	require.True(t, ok)
	got, err := os.ReadFile(filepath.Join(versionDir, manifest.Entrypoints["tool"]))
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
// directory, including the preserved tree and the sidecar manifest. Uses no
// symlinks (matching how Atmos actually exposes onedir entrypoints), so this
// needs no Windows skip.
func TestUninstall_RemovesOnedirTree(t *testing.T) {
	tmp := t.TempDir()
	inst := &Installer{binDir: tmp}

	versionDir := filepath.Join(tmp, "acme", "tool", "1.0.0")
	require.NoError(t, os.MkdirAll(filepath.Join(versionDir, onedirTreeName, "dist"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, onedirTreeName, "dist", "tool"), []byte("BIN"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, onedirTreeName, "dist", "lib.so"), []byte("LIB"), 0o644))
	require.NoError(t, writeOnedirManifest(versionDir, onedirManifest{
		Entrypoints: map[string]string{"tool": filepath.Join(onedirTreeName, "dist", "tool")},
		Primary:     "tool",
	}))

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
