package installer

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCopyOnedirCommandPreservesArchiveSymlink(t *testing.T) {
	directory := t.TempDir()
	writeFileUnder(t, directory, "lib/runtime", "BINARY")
	writeFileUnder(t, directory, "lib/support", "SUPPORT")
	require.NoError(t, os.Mkdir(filepath.Join(directory, "bin"), 0o755))
	err := os.Symlink(filepath.Join("..", "lib", "runtime"), filepath.Join(directory, "bin", "source"))
	if err != nil && runtime.GOOS == "windows" {
		t.Skipf("runner cannot create archive symlinks: %v", err)
	}
	require.NoError(t, err)
	root, err := os.OpenRoot(directory)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	require.NoError(t, copyOnedirCommand(root, "bin/source", "bin/command"))
	resolved, err := filepath.EvalSymlinks(filepath.Join(directory, "bin", "command"))
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(resolved), "support"))
	require.NoError(t, err)
	require.Equal(t, "SUPPORT", string(data), "renamed commands must retain executable-relative runtime files")
	require.FileExists(t, filepath.Join(directory, "bin", "source"))
	err = copyOnedirCommand(root, "bin/source", "bin/command")
	require.ErrorIs(t, err, ErrFileOperation)
	require.ErrorIs(t, err, os.ErrExist)
}

func TestExposeOnedirCommands(t *testing.T) {
	for _, test := range []struct {
		name, goos, source, command, exposed string
	}{
		{"renamed", "linux", "bin/upstream", "compiler", "bin/compiler"},
		{"unchanged", "linux", "bin/compiler", "compiler", "bin/compiler"},
		{"windows extension", "windows", "bin/upstream.exe", "compiler", "bin/compiler.exe"},
		{"windows explicit extension", "windows", "bin/upstream.exe", "compiler.exe", "bin/compiler.exe"},
		{"windows case", "windows", "bin/Compiler.EXE", "compiler", "bin/Compiler.EXE"},
		{"windows script", "windows", "bin/upstream.cmd", "compiler.cmd", "bin/compiler.cmd"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeFileUnder(t, root, test.source, "BINARY")
			eps := []entrypoint{{name: test.command, src: filepath.FromSlash(test.source)}}
			actual, err := exposeOnedirCommands(root, eps, test.goos)
			require.NoError(t, err)
			require.Equal(t, []entrypoint{{name: test.command, src: filepath.FromSlash(test.exposed)}}, actual)
			require.Equal(t, filepath.FromSlash(test.source), eps[0].src, "caller-owned declarations must remain unchanged")
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(test.exposed)))
			require.NoError(t, err)
			require.Equal(t, "BINARY", string(data))
			require.FileExists(t, filepath.Join(root, filepath.FromSlash(test.source)))
		})
	}
}

func TestExposeOnedirCommandsRejectsInvalidNames(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../escape", "nested/name", `nested\name`} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFileUnder(t, root, "bin/source", "ORIGINAL")
			_, err := exposeOnedirCommands(root, []entrypoint{{name: name, src: "bin/source"}}, "linux")
			require.ErrorIs(t, err, ErrFileOperation)
			data, err := os.ReadFile(filepath.Join(root, "bin", "source"))
			require.NoError(t, err)
			require.Equal(t, "ORIGINAL", string(data))
		})
	}
}

func TestExposeOnedirCommandsRejectsMissingTree(t *testing.T) {
	_, err := exposeOnedirCommands(filepath.Join(t.TempDir(), "missing"), nil, "linux")
	require.ErrorIs(t, err, ErrFileOperation)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestExposeOnedirCommandsRejectsMissingCommand(t *testing.T) {
	_, err := exposeOnedirCommands(t.TempDir(), []entrypoint{{name: "command", src: "missing"}}, "linux")
	require.ErrorIs(t, err, ErrFileOperation)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, "failed to make command missing executable")
}

func TestCopyOnedirCommandFailures(t *testing.T) {
	directory := t.TempDir()
	writeFileUnder(t, directory, "source", "SOURCE")
	writeFileUnder(t, directory, "occupied", "DO-NOT-OVERWRITE")
	require.NoError(t, os.Mkdir(filepath.Join(directory, "directory"), 0o755))
	root, err := os.OpenRoot(directory)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	for _, test := range []struct{ source, destination string }{
		{"missing", "command"},
		{"directory", "command"},
		{"source", "occupied"},
		{"source", "missing/command"},
		{"../outside", "command"},
		{"source", "../outside"},
	} {
		t.Run(test.source+" to "+test.destination, func(t *testing.T) {
			err := copyOnedirCommand(root, test.source, test.destination)
			require.ErrorIs(t, err, ErrFileOperation)
		})
	}
	data, err := os.ReadFile(filepath.Join(directory, "occupied"))
	require.NoError(t, err)
	require.Equal(t, "DO-NOT-OVERWRITE", string(data))
	require.NoFileExists(t, filepath.Join(directory, "command"))
}

func TestInstallOnedirCommandCollisionRollsBack(t *testing.T) {
	versionDir := filepath.Join(t.TempDir(), "owner", "tool", "1.0")
	binaryPath := filepath.Join(versionDir, "compiler")
	eps := []entrypoint{{name: "compiler", src: filepath.Join("bin", "upstream")}}
	instance := &Installer{}
	old := t.TempDir()
	writeFileUnder(t, old, "bin/upstream", "OLD-BINARY")
	writeFileUnder(t, old, "include/types.proto", "OLD-INCLUDE")
	require.NoError(t, instance.installOnedirForOS(old, binaryPath, eps, deferredEntries{}, "linux"))
	manifestBefore, err := os.ReadFile(filepath.Join(versionDir, onedirManifestName))
	require.NoError(t, err)

	staging := t.TempDir()
	writeFileUnder(t, staging, "bin/upstream", "NEW-BINARY")
	writeFileUnder(t, staging, "bin/compiler", "COLLISION")
	err = instance.installOnedirForOS(staging, binaryPath, eps, deferredEntries{}, "linux")
	require.ErrorIs(t, err, ErrFileOperation)
	require.ErrorIs(t, err, os.ErrExist)
	manifestAfter, err := os.ReadFile(filepath.Join(versionDir, onedirManifestName))
	require.NoError(t, err)
	require.Equal(t, manifestBefore, manifestAfter)
	for path, expected := range map[string]string{
		"bin/compiler": "OLD-BINARY", "bin/upstream": "OLD-BINARY", "include/types.proto": "OLD-INCLUDE",
	} {
		data, err := os.ReadFile(filepath.Join(versionDir, onedirTreeName, filepath.FromSlash(path)))
		require.NoError(t, err)
		require.Equal(t, expected, string(data))
	}
	require.NoDirExists(t, filepath.Join(versionDir, onedirTreeName+onedirBackupSuffix))
}
