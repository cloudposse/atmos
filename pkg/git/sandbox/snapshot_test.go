package sandbox

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	result, err := gitOutput(t.Context(), root, args...)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip(err)
	}
	// Both the fixture and its private sandbox must use LF regardless of host Git settings.
	config := filepath.Join(t.TempDir(), "gitconfig")
	require.NoError(t, os.WriteFile(config, []byte("[core]\n\tautocrlf = false\n\teol = lf\n"), 0o600))
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	git(t, root, "init", "-q")
	git(t, root, "config", "user.name", "Test")
	git(t, root, "config", "user.email", "test@localhost")
	git(t, root, "config", "core.hooksPath", filepath.Join(root, ".git", "disabled"))
	return root
}

func commit(t *testing.T, root string) {
	t.Helper()
	git(t, root, "add", ".")
	git(t, root, "-c", "commit.gpgsign=false", "commit", "-qm", "baseline")
}

func TestSnapshotProtectsPartialStaging(t *testing.T) {
	root := newRepo(t)
	name := platformFilename("name with spaces\nand newline.txt")
	path := filepath.Join(root, name)
	mustWrite(t, path, "one\ntwo\nthree\nfour\nfive\nsix\nseven\n")
	commit(t, root)
	staged := "one  \ntwo\nthree\nfour\nfive\nsix\nseven\n"
	mustWrite(t, path, staged)
	git(t, root, "add", "--", name)
	working := strings.ReplaceAll(staged, "seven", "unstaged")
	mustWrite(t, path, working)
	index := git(t, root, "diff", "--cached", "--binary")
	tmp := t.TempDir()
	if err := snapshot(t.Context(), root, tmp, "staged"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(tmp, name))
	if string(data) != staged {
		t.Fatalf("unstaged contents leaked: %q", data)
	}
	original, before, err := captureStates(t.Context(), root, tmp, []string{name})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(tmp, name), strings.ReplaceAll(staged, "one  ", "one"))
	changed, err := (&Sandbox{ctx: t.Context(), Root: root, Path: tmp, paths: []string{name}, original: original, before: before}).finish("check")
	if !changed || err != nil {
		t.Fatal(changed, err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != working {
		t.Fatal("read-only check modified working tree")
	}
	changed, err = (&Sandbox{ctx: t.Context(), Root: root, Path: tmp, paths: []string{name}, original: original, before: before}).finish("commit")
	if !changed || err != nil {
		t.Fatal(changed, err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != strings.ReplaceAll(working, "one  ", "one") {
		t.Fatalf("lost unstaged edit: %q", data)
	}
	if git(t, root, "diff", "--cached", "--binary") != index {
		t.Fatal("live index changed")
	}
}

func TestInitialCommitAndLinkedWorktree(t *testing.T) {
	root := newRepo(t)
	mustWrite(t, filepath.Join(root, "initial"), "initial\n")
	git(t, root, "add", ".")
	tmp := t.TempDir()
	if err := snapshot(t.Context(), root, tmp, "staged"); err != nil {
		t.Fatal(err)
	}
	git(t, tmp, "rev-parse", "HEAD")
	if got := git(t, tmp, "diff", "--cached", "--name-only"); got != "initial\n" {
		t.Fatal(got)
	}
	commit(t, root)
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, root, "worktree", "add", "-b", "linked", linked)
	mustWrite(t, filepath.Join(linked, "initial"), "linked staged\n")
	git(t, linked, "add", ".")
	mainIndex := git(t, root, "ls-files", "-s")
	if err := snapshot(t.Context(), linked, t.TempDir(), "staged"); err != nil {
		t.Fatal(err)
	}
	if git(t, root, "ls-files", "-s") != mainIndex {
		t.Fatal("main worktree index changed")
	}
}

func TestSnapshotModesRenamesDeletesAndBinary(t *testing.T) {
	root := newRepo(t)
	mustWrite(t, filepath.Join(root, "old"), "old\n")
	mustWrite(t, filepath.Join(root, "deleted"), "deleted\n")
	commit(t, root)
	git(t, root, "mv", "old", "new")
	git(t, root, "rm", "deleted")
	mustWrite(t, filepath.Join(root, "binary"), "\x00\xff\x01")
	git(t, root, "add", ".")
	git(t, root, "update-index", "--chmod=+x", "new")
	if err := os.Symlink("new", filepath.Join(root, "link")); err == nil {
		git(t, root, "add", "link")
	}
	tmp := t.TempDir()
	if err := snapshot(t.Context(), root, tmp, "staged"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "deleted")); !os.IsNotExist(err) {
		t.Fatal("deleted file present")
	}
	data, _ := os.ReadFile(filepath.Join(tmp, "binary"))
	if !bytes.Equal(data, []byte{0, 255, 1}) {
		t.Fatal(data)
	}
	if git(t, root, "ls-files", "-s") != git(t, tmp, "ls-files", "-s") {
		t.Fatal("index modes or objects differ")
	}
}

func TestConcurrentAndOverlappingEditsPreserved(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(map[bool]string{false: "overlap", true: "concurrent"}[concurrent], func(t *testing.T) {
			root := t.TempDir()
			base := fileState{Exists: true, Mode: 0o644, Data: []byte("base \n")}
			original := base
			original.Data = []byte("my edit\n")
			current := original
			if concurrent {
				current.Data = []byte("new edit\n")
			}
			if err := writeState(filepath.Join(root, "file"), current); err != nil {
				t.Fatal(err)
			}
			after := base
			after.Data = []byte("base\n")
			err := applyFix(t.Context(), root, "file", &fixStates{original, base, after})
			if err == nil || !strings.Contains(err.Error(), "original untouched") {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(filepath.Join(root, "file"))
			if !bytes.Equal(data, current.Data) {
				t.Fatal("user edits lost")
			}
		})
	}
}

func TestCanceledFixDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file")
	mustWrite(t, path, "before\n")
	before, _ := readState(path)
	after := before
	after.Data = []byte("after\n")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := applyFix(ctx, root, "file", &fixStates{before, before, after}); err == nil {
		t.Fatal("cancellation ignored")
	}
	data, _ := os.ReadFile(path)
	if !bytes.Equal(data, before.Data) {
		t.Fatal("canceled fix wrote file")
	}
}

func TestSelectionAndMissingBase(t *testing.T) {
	root := newRepo(t)
	mustWrite(t, filepath.Join(root, "file"), "before\n")
	commit(t, root)
	base := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
	mustWrite(t, filepath.Join(root, "file"), "after\n")
	commit(t, root)
	files, err := selectedFiles(t.Context(), root, "pr", base)
	if err != nil || len(files) != 1 || files[0] != "file" {
		t.Fatal(files, err)
	}
	if _, err := selectedFiles(t.Context(), root, "pr", "missing"); err == nil || !strings.Contains(err.Error(), "--base-ref") {
		t.Fatal(err)
	}
}

func TestSafePaths(t *testing.T) {
	root := t.TempDir()
	if _, err := safePath(root, "../escape"); err == nil {
		t.Fatal("traversal accepted")
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "linked")); err != nil {
		t.Skip(err)
	}
	if _, err := safePath(root, filepath.Join("linked", "file")); err == nil {
		t.Fatal("symlink parent accepted")
	}
}

func TestSnapshotHonorsCommitTemporaryIndex(t *testing.T) {
	root := newRepo(t)
	path := filepath.Join(root, "file")
	mustWrite(t, path, "baseline\n")
	commit(t, root)
	originalIndex := git(t, root, "ls-files", "-s")
	index := filepath.Join(t.TempDir(), "index")
	t.Setenv("GIT_INDEX_FILE", index)
	if _, err := sourceGitOutput(t.Context(), root, "read-tree", "HEAD"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, "temporary index content\n")
	if _, err := sourceGitOutput(t.Context(), root, "add", "file"); err != nil {
		t.Fatal(err)
	}
	files, err := selectedFiles(t.Context(), root, "staged", "")
	if err != nil || len(files) != 1 || files[0] != "file" {
		t.Fatal(files, err)
	}
	tmp := t.TempDir()
	if err := snapshot(t.Context(), root, tmp, "staged"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(tmp, "file"))
	if string(data) != "temporary index content\n" {
		t.Fatalf("wrong commit content: %q", data)
	}
	if git(t, root, "ls-files", "-s") != originalIndex {
		t.Fatal("normal index changed")
	}
}

// TestSnapshotPreservesStagedSymlinkTimestamps keeps timestamp updates inside the private checkout.
func TestSnapshotPreservesStagedSymlinkTimestamps(t *testing.T) {
	for _, exists := range []bool{true, false} {
		t.Run(map[bool]string{true: "existing target", false: "dangling target"}[exists], func(t *testing.T) {
			root := newRepo(t)
			target := filepath.Join(t.TempDir(), "external")
			timestamp := time.Now().Add(-time.Hour).Truncate(time.Second)
			if exists {
				mustWrite(t, target, "external contents")
				require.NoError(t, os.Chtimes(target, timestamp, timestamp))
			}
			path := filepath.Join(root, "link")
			if err := os.Symlink(target, path); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			git(t, root, "config", "core.symlinks", "true")
			git(t, root, "add", "link")
			require.NoError(t, os.Remove(path))
			mustWrite(t, path, "unstaged regular file")
			index, err := os.ReadFile(filepath.Join(root, ".git", "index"))
			require.NoError(t, err)
			dest := t.TempDir()
			git(t, dest, "init", "-q")
			git(t, dest, "config", "core.symlinks", "true")
			require.NoError(t, snapshot(t.Context(), root, dest, "staged"))
			gotTarget, err := os.Readlink(filepath.Join(dest, "link"))
			require.NoError(t, err)
			require.Equal(t, target, gotTarget)
			info, err := os.Stat(target)
			if exists {
				require.NoError(t, err)
				require.Equal(t, timestamp, info.ModTime(), "external target timestamp changed")
			} else {
				require.ErrorIs(t, err, os.ErrNotExist)
			}
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "unstaged regular file", string(data))
			gotIndex, err := os.ReadFile(filepath.Join(root, ".git", "index"))
			require.NoError(t, err)
			require.Equal(t, index, gotIndex, "live index changed")
		})
	}
}
