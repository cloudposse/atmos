package downloader

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
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
		t.Skip("permission-based Walk error test skipped on Windows")
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
