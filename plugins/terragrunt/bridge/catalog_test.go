package atmosbridge

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyCatalogCommit(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}

	v := Environment(map[string]string{"PATH": os.Getenv("PATH")}, io.Discard, io.Discard)
	git("init", "-q")
	require.ErrorContains(t, verifyCatalogCommit(t.Context(), v, repo, strings.Repeat("0", 40)), "resolve catalog HEAD")
	git("-c", "user.name=Test", "-c", "user.email=test@example.org", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "snapshot")
	head := git("rev-parse", "HEAD")
	require.NoError(t, verifyCatalogCommit(t.Context(), v, repo, strings.ToUpper(head)))
	require.ErrorContains(t, verifyCatalogCommit(t.Context(), v, repo, strings.Repeat("0", 40)), "differs from pinned commit")
}
