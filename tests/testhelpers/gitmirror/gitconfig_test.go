package gitmirror

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWriteGitConfig verifies the exact rendered content for two tokens: one insteadOf rule per
// token, in the userinfo form CustomGitDetector's token injection produces, plus the anonymous
// https/ssh rules that cover callers which never inject a token at all.
func TestWriteGitConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gitconfig")
	require.NoError(t, WriteGitConfig(path, "http://127.0.0.1:54321", []string{"token-a", "token-b"}))

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	want := `[url "http://x-access-token:token-a@127.0.0.1:54321/cloudposse/atmos.git"]
	insteadOf = https://x-access-token:token-a@github.com/cloudposse/atmos.git
[url "http://x-access-token:token-b@127.0.0.1:54321/cloudposse/atmos.git"]
	insteadOf = https://x-access-token:token-b@github.com/cloudposse/atmos.git
[url "http://127.0.0.1:54321/cloudposse/atmos.git"]
	insteadOf = https://github.com/cloudposse/atmos.git
	insteadOf = ssh://git@github.com/cloudposse/atmos.git
`
	require.Equal(t, want, string(content))
}

// TestWriteGitConfig_NoTokens verifies the anonymous-only rule is still written when no tokens
// are registered, so a caller relying solely on AllowAnonymous still gets redirected.
func TestWriteGitConfig_NoTokens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gitconfig")
	require.NoError(t, WriteGitConfig(path, "http://127.0.0.1:9999", nil))

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	want := `[url "http://127.0.0.1:9999/cloudposse/atmos.git"]
	insteadOf = https://github.com/cloudposse/atmos.git
	insteadOf = ssh://git@github.com/cloudposse/atmos.git
`
	require.Equal(t, want, string(content))
}

// TestWriteGitConfig_TrimsTrailingSlash verifies a trailing slash on serverURL doesn't produce a
// double slash before the repository path.
func TestWriteGitConfig_TrimsTrailingSlash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gitconfig")
	require.NoError(t, WriteGitConfig(path, "http://127.0.0.1:54321/", nil))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), `[url "http://127.0.0.1:54321/cloudposse/atmos.git"]`)
}
