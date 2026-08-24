package git

import (
	"os"
	"path/filepath"
	"testing"
)

func gitTestArgs(args ...string) []string {
	base := []string{
		"-c", "commit.gpgsign=false",
		"-c", "tag.gpgsign=false",
		"-c", "gpg.format=openpgp",
	}
	return append(base, args...)
}

func markGitWorktree(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("marking git worktree: %v", err)
	}
}
