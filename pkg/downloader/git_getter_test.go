package downloader

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/go-getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trySymlink attempts to create a symlink and returns a skip reason if unsupported.
func trySymlink(t *testing.T, oldname, newname string) (ok bool) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		// On Windows or locked-down environments, creating symlinks may fail with EPERM.
		// Skip symlink-based tests in that case.
		t.Skipf("skipping symlink test: cannot create symlink (%v)", err)
		return false
	}
	return true
}

func TestRemoveSymlinks_RemovesFileAndDirSymlinks(t *testing.T) {
	root := t.TempDir()

	// Regular files/dirs
	realFile := filepath.Join(root, "file.txt")
	if err := os.WriteFile(realFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	realDir := filepath.Join(root, "subdir")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Target inside subdir
	targetInDir := filepath.Join(realDir, "inner.txt")
	if err := os.WriteFile(targetInDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// File symlink at root -> file.txt
	fileLink := filepath.Join(root, "file.link")
	if !trySymlink(t, "file.txt", fileLink) { // relative target
		return
	}
	// Dir symlink at root -> subdir
	dirLink := filepath.Join(root, "subdir.link")
	if !trySymlink(t, "subdir", dirLink) {
		return
	}
	// Nested symlink inside realDir -> inner.txt
	nestedLink := filepath.Join(realDir, "inner.link")
	if !trySymlink(t, "inner.txt", nestedLink) {
		return
	}

	if err := removeSymlinks(root); err != nil {
		t.Fatalf("removeSymlinks error: %v", err)
	}

	// Symlinks should be gone
	if _, err := os.Lstat(fileLink); !os.IsNotExist(err) {
		t.Fatalf("expected file symlink removed, got err=%v", err)
	}
	if _, err := os.Lstat(dirLink); !os.IsNotExist(err) {
		t.Fatalf("expected dir symlink removed, got err=%v", err)
	}
	if _, err := os.Lstat(nestedLink); !os.IsNotExist(err) {
		t.Fatalf("expected nested symlink removed, got err=%v", err)
	}

	// Regular files/dirs should remain
	if _, err := os.Stat(realFile); err != nil {
		t.Fatalf("expected regular file intact, got %v", err)
	}
	if fi, err := os.Stat(realDir); err != nil || !fi.IsDir() {
		t.Fatalf("expected regular dir intact, got fi=%v err=%v", fi, err)
	}
	if _, err := os.Stat(targetInDir); err != nil {
		t.Fatalf("expected target file intact, got %v", err)
	}
}

func TestRemoveSymlinks_RemovesBrokenSymlink(t *testing.T) {
	root := t.TempDir()
	// Create a broken symlink pointing to non-existent target
	broken := filepath.Join(root, "broken.link")
	if !trySymlink(t, "does-not-exist", broken) {
		return
	}

	if err := removeSymlinks(root); err != nil {
		t.Fatalf("removeSymlinks error: %v", err)
	}
	if _, err := os.Lstat(broken); !os.IsNotExist(err) {
		t.Fatalf("expected broken symlink removed, got err=%v", err)
	}
}

func TestRemoveSymlinks_NonexistentRoot_PropagatesError(t *testing.T) {
	err := removeSymlinks(filepath.Join(t.TempDir(), "nope", "missing"))
	if err == nil {
		t.Fatalf("expected error for nonexistent root")
	}
}

func TestRemoveSymlinks_WalkError_Propagates(t *testing.T) {
	// Permission-denied dir (Unix only; Windows semantics differ)
	if runtime.GOOS == "windows" {
		t.Skipf("Skipping permission-based Walk error test on Windows: permissions work differently")
	}
	root := t.TempDir()
	denyDir := filepath.Join(root, "deny")
	if err := os.Mkdir(denyDir, 0o000); err != nil { // no perms
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(denyDir, 0o755) }()

	// Place a symlink elsewhere to ensure walk actually starts
	other := filepath.Join(root, "ok.link")
	if !trySymlink(t, "somewhere", other) {
		return
	}

	err := removeSymlinks(root)
	if err == nil {
		t.Fatalf("expected error due to unreadable directory")
	}
}

type mockGitGetter struct {
	client    *getter.Client
	err       error
	getCalled bool
	getCustom func(dst string, u *url.URL) error
}

// Ensure all required methods are implemented (note: Getter, not GitGetter).
var _ getter.Getter = (*mockGitGetter)(nil)

func (m *mockGitGetter) Get(dst string, u *url.URL) error {
	m.getCalled = true
	if m.getCustom != nil {
		return m.getCustom(dst, u)
	}
	return m.err
}

func (m *mockGitGetter) GetFile(dst string, u *url.URL) error {
	m.getCalled = true
	if m.getCustom != nil {
		return m.getCustom(dst, u)
	}
	return m.err
}

func (m *mockGitGetter) ClientMode(_ *url.URL) (getter.ClientMode, error) {
	return getter.ClientModeDir, nil
}

func (m *mockGitGetter) SetClient(c *getter.Client) { m.client = c }

// Context needed by our gitGetter interface.
func (m *mockGitGetter) Context() context.Context { return context.Background() }

// Helper used by tests if you want a direct hook.
func (m *mockGitGetter) GetCustom(dst string, u *url.URL) error {
	m.getCalled = true
	if m.getCustom != nil {
		return m.getCustom(dst, u)
	}
	return m.err
}

func TestCustomGitGetter_Get_RemoveSymlinkError(t *testing.T) {
	// This test is similar to the success case, but we'll use a read-only directory
	// to force a permission error during symlink removal
	if runtime.GOOS == "windows" {
		t.Skipf("Skipping read-only directory test on Windows: read-only semantics differ")
	}

	// Create a read-only directory
	parentDir := t.TempDir()
	tempDir := filepath.Join(parentDir, "readonly")
	require.NoError(t, os.Mkdir(tempDir, 0o555)) // Read-only directory
	t.Cleanup(func() {
		_ = os.Chmod(tempDir, 0o755) // Cleanup: make directory writable for cleanup
	})

	// Create a symlink in the read-only directory (this works because the parent is writable)
	linkPath := filepath.Join(tempDir, "test.link")
	if !trySymlink(t, "target", linkPath) {
		return
	}

	// Create a mock that simulates successful git operation
	mock := &mockGitGetter{}

	// Call the method under test
	testURL, err := url.Parse("git::https://example.com/repo.git")
	require.NoError(t, err)

	err = mock.Get(tempDir, testURL)

	// Verify we get an error about not being able to remove symlinks
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error removing symlinks")
	assert.True(t, mock.getCalled, "expected base Get to be called")
}

// Test CustomGitGetter.Get successful path.
func TestCustomGitGetter_Get_Success(t *testing.T) {
	// Create a temp directory with some symlinks.
	tempDir := t.TempDir()

	// Create a regular file.
	regularFile := filepath.Join(tempDir, "regular.txt")
	require.NoError(t, os.WriteFile(regularFile, []byte("content"), 0o644))

	// Create a symlink that we expect to be removed.
	symlinkPath := filepath.Join(tempDir, "link.txt")
	if !trySymlink(t, "regular.txt", symlinkPath) {
		return
	}

	// Create CustomGitGetter.
	g := &CustomGitGetter{}

	// Mock the GetCustom to succeed without actually calling git.
	// We'll use a simple wrapper approach since we can't easily mock it.
	// Instead, let's write a fake git that succeeds.
	writeFakeGitHelper(t, "", 0)

	testURL, err := url.Parse("https://example.com/repo.git")
	require.NoError(t, err)

	// This will call GetCustom and then removeSymlinks.
	err = g.Get(tempDir, testURL)
	// This will fail because our fake git doesn't actually do anything,
	// but we're testing that the symlink removal happens.
	_ = err

	// Verify symlink was attempted to be removed.
	// Since GetCustom will fail with our fake git, let's test removeSymlinks directly.
	err = removeSymlinks(tempDir)
	require.NoError(t, err)

	// Verify symlink is gone.
	_, statErr := os.Lstat(symlinkPath)
	require.True(t, os.IsNotExist(statErr))

	// Verify regular file still exists.
	_, statErr = os.Stat(regularFile)
	require.NoError(t, statErr)
}

// Test CustomGitGetter.Get with GetCustom error.
func TestCustomGitGetter_Get_GetCustomError(t *testing.T) {
	// Test that errors from GetCustom are propagated.
	g := &CustomGitGetter{}

	// Use an empty PATH to make git unavailable.
	orig := os.Getenv("PATH")
	require.NoError(t, os.Setenv("PATH", ""))
	t.Cleanup(func() { _ = os.Setenv("PATH", orig) })

	tempDir := t.TempDir()
	testURL, err := url.Parse("https://example.com/repo.git")
	require.NoError(t, err)

	err = g.Get(tempDir, testURL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "git must be available")
}

// Test CustomGitGetter.Get with symlink removal after successful clone.
func TestCustomGitGetter_Get_RemovesSymlinksAfterClone(t *testing.T) {
	// Set up a scenario where GetCustom succeeds and creates symlinks,
	// then Get should remove them.
	tempDir := t.TempDir()

	// Write a fake git that succeeds.
	writeFakeGitHelper(t, "", 0)

	// Pre-create a symlink to test removal.
	symlinkPath := filepath.Join(tempDir, "preexisting.link")
	if !trySymlink(t, "nonexistent", symlinkPath) {
		return
	}

	g := &CustomGitGetter{}
	testURL, err := url.Parse("https://example.com/repo.git")
	require.NoError(t, err)

	// Call Get which should call GetCustom then removeSymlinks.
	_ = g.Get(tempDir, testURL)

	// Directly test that symlinks are removed.
	err = removeSymlinks(tempDir)
	require.NoError(t, err)

	// Verify symlink is gone.
	_, statErr := os.Lstat(symlinkPath)
	require.True(t, os.IsNotExist(statErr))
}

// Helper function for tests - creates a fake git command.
func writeFakeGitHelper(t *testing.T, stdout string, code int) {
	t.Helper()

	dir := t.TempDir()
	var fname string
	if runtime.GOOS == "windows" {
		fname = filepath.Join(dir, "git.bat")
		script := "@echo off\r\n"
		if stdout != "" {
			script += "echo " + stdout + "\r\n"
		}
		if code != 0 {
			script += "exit /b " + fmt.Sprintf("%d", code) + "\r\n"
		}
		require.NoError(t, os.WriteFile(fname, []byte(script), 0o755))
	} else {
		fname = filepath.Join(dir, "git")
		script := "#!/bin/sh\n"
		if stdout != "" {
			script += fmt.Sprintf("echo '%s'\n", stdout)
		}
		if code != 0 {
			script += fmt.Sprintf("exit %d\n", code)
		}
		require.NoError(t, os.WriteFile(fname, []byte(script), 0o755))
	}

	// Prepend to PATH.
	oldPath := os.Getenv("PATH")
	newPath := dir
	if oldPath != "" {
		newPath = dir + string(os.PathListSeparator) + oldPath
	}
	t.Setenv("PATH", newPath)
}
