package toolchain

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/filelock"
)

func TestToolVersionsLocksStayOutsideProject(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("ATMOS_XDG_DATA_HOME", dataDir)
	project := t.TempDir()
	manifest := filepath.Join(project, ".tool-versions")
	require.NoError(t, os.WriteFile(manifest, []byte("terraform 1.0.0\n"), 0o644))
	legacy := manifest + ".lock"
	require.NoError(t, os.WriteFile(legacy, []byte("legacy lock"), 0o644))

	versions, err := LoadToolVersions(manifest)
	require.NoError(t, err)
	require.NoError(t, SaveToolVersions(manifest, versions))
	entries, err := os.ReadDir(project)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	contents, err := os.ReadFile(legacy)
	require.NoError(t, err)
	assert.Equal(t, "legacy lock", string(contents))
	locks, err := os.ReadDir(filepath.Join(dataDir, "atmos", "locks", "tool-versions"))
	require.NoError(t, err)
	require.Len(t, locks, 1, "the same lock remains after both reader and writer release")
	assert.Regexp(t, `^[a-f0-9]{64}\.lock$`, locks[0].Name())
}

func TestToolVersionsMissingReadDoesNotCreateDirectories(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	t.Setenv("ATMOS_XDG_DATA_HOME", dataDir)
	project := filepath.Join(root, "missing", "project")
	_, err := LoadToolVersions(filepath.Join(project, ".tool-versions"))
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.NoDirExists(t, filepath.Join(root, "missing"))
	assert.NoDirExists(t, dataDir)

	require.NoError(t, SaveToolVersions(filepath.Join(project, ".tool-versions"), &ToolVersions{Tools: map[string][]string{"terraform": {"1.0.0"}}}))
	assert.FileExists(t, filepath.Join(project, ".tool-versions"))
	assert.NoFileExists(t, filepath.Join(project, ".tool-versions.lock"))
}

func TestToolVersionsReadOnlyProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix directory permissions")
	}
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	project := t.TempDir()
	manifest := filepath.Join(project, ".tool-versions")
	require.NoError(t, os.WriteFile(manifest, []byte("terraform 1.0.0\n"), 0o444))
	require.NoError(t, os.Chmod(project, 0o555))
	t.Cleanup(func() { require.NoError(t, os.Chmod(project, 0o755)) })
	versions, err := LoadToolVersions(manifest)
	require.NoError(t, err)
	assert.Equal(t, []string{"1.0.0"}, versions.Tools["terraform"])
	assert.NoFileExists(t, manifest+".lock")
	if os.Geteuid() != 0 {
		err := SaveToolVersions(filepath.Join(project, "missing", ".tool-versions"), versions)
		require.Error(t, err)
		assert.ErrorContains(t, err, "create .tool-versions directory")
		assert.NoDirExists(t, filepath.Join(project, "missing"))
	}
}

func TestToolVersionsLockAliases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privileges on Windows")
	}
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	require.NoError(t, os.Mkdir(realDir, 0o755))
	aliasDir := filepath.Join(root, "alias")
	require.NoError(t, os.Symlink(realDir, aliasDir))
	manifest := filepath.Join(realDir, "nested", ".tool-versions")
	link := filepath.Join(root, "manifest-link")
	require.NoError(t, os.Symlink(filepath.Join("real", "nested", ".tool-versions"), link))
	cwd, err := os.Getwd()
	require.NoError(t, err)
	relative, err := filepath.Rel(cwd, manifest)
	require.NoError(t, err)
	aliases := []string{manifest, relative, filepath.Join(aliasDir, "nested", ".tool-versions"), link}
	before, err := toolVersionsLock(manifest)
	require.NoError(t, err)
	for _, path := range aliases {
		lock, err := toolVersionsLock(path)
		require.NoError(t, err)
		assert.Equal(t, before.Path(), lock.Path(), path)
	}
	require.NoError(t, SaveToolVersions(manifest, &ToolVersions{Tools: map[string][]string{}}))
	for _, path := range aliases {
		lock, err := toolVersionsLock(path)
		require.NoError(t, err)
		assert.Equal(t, before.Path(), lock.Path(), path)
	}
}

func TestToolVersionsLockFailuresPreventAccess(t *testing.T) {
	for _, failure := range []string{"data directory", "lock file", "manifest directory", "symlink loop"} {
		t.Run(failure, func(t *testing.T) {
			dataDir := t.TempDir()
			t.Setenv("ATMOS_XDG_DATA_HOME", dataDir)
			manifest := filepath.Join(t.TempDir(), ".tool-versions")
			require.NoError(t, os.WriteFile(manifest, []byte("terraform 1.0.0\n"), 0o644))
			switch failure {
			case "data directory":
				require.NoError(t, os.WriteFile(filepath.Join(dataDir, "atmos"), nil, 0o644))
			case "lock file":
				if runtime.GOOS == "windows" || os.Geteuid() == 0 {
					t.Skip("requires Unix permissions and an unprivileged user")
				}
				lock, err := toolVersionsLock(manifest)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(lock.Path(), nil, 0o000))
			case "manifest directory":
				manifest = filepath.Join(manifest, "child")
			case "symlink loop":
				if runtime.GOOS == "windows" {
					t.Skip("symlinks require privileges on Windows")
				}
				manifest += "-loop"
				require.NoError(t, os.Symlink(manifest, manifest))
			}
			called := false
			callback := func() error { called = true; return nil }
			assert.Error(t, withToolVersionsLock(manifest, callback))
			assert.False(t, called)
			assert.Error(t, withToolVersionsSharedLock(manifest, callback))
			assert.False(t, called)
			_, err := LoadToolVersions(manifest)
			assert.Error(t, err)
		})
	}
}

// TestToolVersionsLockProcess is re-executed by the contention test. Each child
// uses the public read/add operations and its own OS lock handles.
func TestToolVersionsLockProcess(t *testing.T) {
	if os.Getenv("ATMOS_TEST_TOOL_VERSIONS_PROCESS") != "1" {
		return
	}
	manifest := os.Getenv("ATMOS_TEST_TOOL_VERSIONS_PATH")
	role := os.Getenv("ATMOS_TEST_TOOL_VERSIONS_ROLE")
	if role == "hold-reader" {
		require.NoError(t, withToolVersionsSharedLock(manifest, func() error {
			_, err := io.WriteString(os.Stdout, "locked\n")
			if err != nil {
				return err
			}
			_, err = io.Copy(io.Discard, os.Stdin)
			return err
		}))
		return
	}
	for i := 0; i < 12; i++ {
		if role == "reader" {
			versions, err := LoadToolVersions(manifest)
			require.NoError(t, err)
			require.Equal(t, []string{"1.0.0"}, versions.Tools["test/seed"])
		} else {
			require.NoError(t, AddToolToVersions(manifest, "test/"+role, fmt.Sprintf("1.0.%d", i)))
		}
	}
}

func TestToolVersionsSeparateProcessReadersAndWriters(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("ATMOS_XDG_DATA_HOME", dataDir)
	project := t.TempDir()
	manifest := filepath.Join(project, ".tool-versions")
	require.NoError(t, SaveToolVersions(manifest, &ToolVersions{Tools: map[string][]string{"test/seed": {"1.0.0"}}}))
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	roles := []string{"writer-a", "writer-b", "writer-c", "reader", "reader"}
	commands := make([]*exec.Cmd, 0, len(roles))
	outputs := make([]*bytes.Buffer, 0, len(roles))
	alias := filepath.Join(project, strings.ToUpper(filepath.Base(manifest)))
	originalInfo, err := os.Stat(manifest)
	require.NoError(t, err)
	aliasInfo, aliasErr := os.Stat(alias)
	if aliasErr != nil || !os.SameFile(originalInfo, aliasInfo) {
		alias = manifest
	}
	for i, role := range roles {
		path := manifest
		if i%2 != 0 {
			path = alias
		}
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestToolVersionsLockProcess$")
		cmd.Env = append(os.Environ(), "ATMOS_TEST_TOOL_VERSIONS_PROCESS=1", "ATMOS_TEST_TOOL_VERSIONS_PATH="+path, "ATMOS_TEST_TOOL_VERSIONS_ROLE="+role)
		output := &bytes.Buffer{}
		cmd.Stdout, cmd.Stderr = output, output
		require.NoError(t, cmd.Start())
		commands = append(commands, cmd)
		outputs = append(outputs, output)
	}
	for i, cmd := range commands {
		assert.NoError(t, cmd.Wait(), "%s: %s", roles[i], outputs[i].String())
	}
	versions, err := LoadToolVersions(manifest)
	require.NoError(t, err)
	for _, role := range roles[:3] {
		assert.Len(t, versions.Tools["test/"+role], 12, "no read-modify-write updates may be lost")
	}
	entries, err := os.ReadDir(project)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestToolVersionsSharedLockAllowsReadersAndExcludesWriters(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	manifest := filepath.Join(t.TempDir(), ".tool-versions")
	lock, err := toolVersionsLock(manifest)
	require.NoError(t, err)
	require.NoError(t, withToolVersionsSharedLock(manifest, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, lock.WithShared(ctx, func() error { return nil }))
		writeCtx, writeCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer writeCancel()
		err := lock.WithExclusive(writeCtx, func() error {
			t.Error("writer entered while reader held the lock")
			return nil
		})
		assert.ErrorIs(t, err, filelock.ErrAcquire)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		return nil
	}))
}

func TestToolVersionsSharedLockAcrossProcesses(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	manifest := filepath.Join(t.TempDir(), ".tool-versions")
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestToolVersionsLockProcess$")
	cmd.Env = append(os.Environ(), "ATMOS_TEST_TOOL_VERSIONS_PROCESS=1", "ATMOS_TEST_TOOL_VERSIONS_PATH="+manifest, "ATMOS_TEST_TOOL_VERSIONS_ROLE=hold-reader")
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	defer stdin.Close()
	require.NoError(t, cmd.Start())
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "locked\n", ready, "child must hold a shared OS lock before parent proceeds")
	lock, err := toolVersionsLock(manifest)
	require.NoError(t, err)
	require.NoError(t, lock.WithShared(ctx, func() error { return nil }))
	writeCtx, writeCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer writeCancel()
	assert.ErrorIs(t, lock.WithExclusive(writeCtx, func() error {
		t.Error("writer entered while another process held a reader lock")
		return nil
	}), filelock.ErrAcquire)
	require.NoError(t, stdin.Close())
	require.NoError(t, cmd.Wait())
	require.NoError(t, lock.WithExclusive(ctx, func() error { return nil }))
}

func TestToolVersionsLockCaseAliases(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	project := filepath.Join(root, "CaseProject")
	require.NoError(t, os.Mkdir(project, 0o755))
	preserveCase, err := toolVersionsPreserveCase(project)
	require.NoError(t, err)
	// Unknown filesystems deliberately serialize distinct case variants. Native
	// sensitive filesystems must still retain independent manifest locks.
	foldKeys := !preserveCase

	initial, err := toolVersionsLock(filepath.Join(project, "Nested", ".tool-versions"))
	require.NoError(t, err)
	for _, state := range []string{"missing parents", "missing manifest", "existing manifest"} {
		t.Run(state, func(t *testing.T) {
			manifest := filepath.Join(project, "Nested", ".tool-versions")
			alias := filepath.Join(project, "NESTED", ".TOOL-VERSIONS")
			if state == "missing manifest" {
				require.NoError(t, os.Mkdir(filepath.Dir(manifest), 0o755))
			}
			if state == "existing manifest" {
				require.NoError(t, SaveToolVersions(manifest, &ToolVersions{Tools: map[string][]string{"terraform": {"1.0"}}}))
			}
			before := snapshotToolVersionsProject(t, project)
			original, err := toolVersionsLock(manifest)
			require.NoError(t, err)
			assert.Equal(t, initial.Path(), original.Path(), "creating the manifest must not change the lock")
			variant, err := toolVersionsLock(alias)
			require.NoError(t, err)
			if foldKeys {
				assert.Equal(t, original.Path(), variant.Path())
				parentAlias, err := toolVersionsLock(filepath.Join(root, strings.ToUpper(filepath.Base(project)), "NESTED", ".TOOL-VERSIONS"))
				require.NoError(t, err)
				assert.Equal(t, original.Path(), parentAlias.Path())
			} else {
				assert.NotEqual(t, original.Path(), variant.Path(), "distinct case-sensitive manifests must retain distinct keys")
			}
			require.NoError(t, original.WithShared(context.Background(), func() error {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				err := variant.WithExclusive(ctx, func() error { return nil })
				if foldKeys {
					assert.ErrorIs(t, err, filelock.ErrAcquire)
				} else {
					assert.NoError(t, err, "case-sensitive manifests must lock independently")
				}
				return nil
			}))
			after := snapshotToolVersionsProject(t, project)
			assert.Equal(t, before, after, "lock resolution must preserve every project path and file's bytes")
		})
	}
}

func TestToolVersionsCasePolicyNeverSplitsFilesystemAliases(t *testing.T) {
	root := t.TempDir()
	probe := filepath.Join(root, "CaseProbe")
	require.NoError(t, os.WriteFile(probe, nil, 0o644))
	original, err := os.Stat(probe)
	require.NoError(t, err)
	variant, variantErr := os.Stat(filepath.Join(root, "CASEPROBE"))
	expected := variantErr != nil || !os.SameFile(original, variant)
	actual, err := toolVersionsPreserveCase(root)
	require.NoError(t, err)
	if actual {
		assert.True(t, expected, "case-preserving lock keys require proven case-sensitive lookup")
	}
	if !expected {
		assert.False(t, actual, "real filesystem aliases must always share lock keys")
	}

	_, err = toolVersionsPreserveCase(filepath.Join(root, "missing"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = toolVersionsPreserveCase("invalid\x00directory")
	require.Error(t, err, "invalid native path encodings must not be accepted")
}

func TestToolVersionsCaseSensitivityWithoutLinuxFlags(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux procfs does not implement FS_IOC_GETFLAGS")
	}
	const procDirectory = "/proc"
	if _, err := os.Stat(procDirectory); os.IsNotExist(err) {
		t.Skip("procfs is not mounted")
	}
	sensitive, err := toolVersionsPreserveCase(procDirectory)
	require.NoError(t, err, "a filesystem without casefold flags must remain usable")
	assert.True(t, sensitive)
}

func TestToolVersionsFallbackKeepsLockAcrossManifestCreation(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	project := t.TempDir()
	manifest := filepath.Join(project, "Nested", ".tool-versions")
	alias := filepath.Join(project, "NESTED", ".TOOL-VERSIONS")
	before, err := toolVersionsLockWithCasePolicy(manifest, toolVersionsFallbackCasePolicy)
	require.NoError(t, err)
	variant, err := toolVersionsLockWithCasePolicy(alias, toolVersionsFallbackCasePolicy)
	require.NoError(t, err)
	assert.Equal(t, before.Path(), variant.Path())
	entries, err := os.ReadDir(project)
	require.NoError(t, err)
	assert.Empty(t, entries, "metadata fallback must not create probe files or missing directories")

	require.NoError(t, before.WithExclusive(context.Background(), func() error {
		require.NoError(t, os.MkdirAll(filepath.Dir(manifest), 0o755))
		require.NoError(t, os.WriteFile(manifest, []byte("terraform 1.0\n"), 0o644))
		for _, name := range []string{manifest, alias} {
			after, err := toolVersionsLockWithCasePolicy(name, toolVersionsFallbackCasePolicy)
			require.NoError(t, err)
			assert.Equal(t, before.Path(), after.Path(), "creation must not replace a held lock's identity")
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			err = after.WithShared(ctx, func() error {
				t.Error("reader entered before the creating writer released its lock")
				return nil
			})
			cancel()
			assert.ErrorIs(t, err, filelock.ErrAcquire)
		}
		return nil
	}))
	require.NoError(t, variant.WithShared(context.Background(), func() error { return nil }))
	assert.NoFileExists(t, manifest+".lock")
	_, err = toolVersionsFallbackCasePolicy(filepath.Join(project, "absent"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = toolVersionsFallbackCasePolicy("invalid\x00path")
	require.Error(t, err, "fallback metadata failures must propagate")
}

// snapshotToolVersionsProject compares meaningful project state rather than
// opaque DirEntry metadata, whose access timestamps can legitimately change.
func snapshotToolVersionsProject(t *testing.T, root string) map[string]string {
	t.Helper()
	entries := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			entries[path] = "directory"
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entries[path] = entry.Type().String() + ":" + string(content)
		return nil
	})
	require.NoError(t, err)
	return entries
}
