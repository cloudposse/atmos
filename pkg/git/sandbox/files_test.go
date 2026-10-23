package sandbox

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFileStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	state := fileState{Data: []byte("contents"), Exists: true, Mode: 0o755}
	if err := writeState(path, state); err != nil {
		t.Fatal(err)
	}
	got, err := readState(path)
	if err != nil || !got.Exists || string(got.Data) != string(state.Data) || (runtime.GOOS != "windows" && got.Mode != state.Mode) {
		t.Fatal(got, err)
	}
	if err := writeState(path, fileState{}); err != nil {
		t.Fatal(err)
	}
	if err := writeState(path, fileState{}); err != nil {
		t.Fatal(err)
	}
	got, err = readState(path)
	if err != nil || got.Exists {
		t.Fatal(got, err)
	}
	if _, err := readState(dir); err == nil {
		t.Fatal("directory accepted as a file")
	}
	if err := os.Symlink("target", path); err != nil {
		t.Skip(err)
	}
	state, err = readState(path)
	if err != nil || string(state.Data) != "target" {
		t.Fatal(state, err)
	}
	if err := writeState(filepath.Join(dir, "copy"), state); err != nil {
		t.Fatal(err)
	}
	got, err = readState(filepath.Join(dir, "copy"))
	if err != nil || !equalState(got, state) {
		t.Fatal(got, err)
	}
}

func TestRecoveryPatchUsesRepositoryPaths(t *testing.T) {
	root := newRepo(t)
	name := "nested/file with space\nand newline.txt"
	before := fileState{Data: []byte("before \n"), Exists: true, Mode: 0o644}
	after := before
	after.Data = []byte("before\n")
	err := saveConflict(t.Context(), name, before, after, "test conflict")
	if err == nil {
		t.Fatal("expected recovery diagnostic")
	}
	_, location, ok := strings.Cut(err.Error(), "Review ")
	if !ok {
		t.Fatal(err)
	}
	patch, _, _ := strings.Cut(location, " (before/after")
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(patch)) })
	mustWrite(t, filepath.Join(root, filepath.FromSlash(name)), string(before.Data))
	git(t, root, "apply", "--check", patch)
}

func mustWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
