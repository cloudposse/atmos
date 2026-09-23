package gitmirror

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWriteGitConfig covers WriteGitConfig's rendered content across three scenarios: one
// insteadOf rule per token in the userinfo form CustomGitDetector's token injection produces
// plus the anonymous https/ssh rules that cover callers which never inject a token at all; the
// anonymous-only rule still written when no tokens are registered; and a trailing slash on
// serverURL not producing a double slash before the repository path.
func TestWriteGitConfig(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		tokens    []string
		check     func(t *testing.T, content string)
	}{
		{
			name:      "tokens produce per-token insteadOf rules plus anonymous fallback rules",
			serverURL: "http://127.0.0.1:54321",
			tokens:    []string{"token-a", "token-b"},
			check: func(t *testing.T, content string) {
				t.Helper()

				want := `[url "http://x-access-token:token-a@127.0.0.1:54321/cloudposse/atmos.git"]
	insteadOf = https://x-access-token:token-a@github.com/cloudposse/atmos.git
[url "http://x-access-token:token-b@127.0.0.1:54321/cloudposse/atmos.git"]
	insteadOf = https://x-access-token:token-b@github.com/cloudposse/atmos.git
[url "http://127.0.0.1:54321/cloudposse/atmos.git"]
	insteadOf = https://github.com/cloudposse/atmos.git
	insteadOf = ssh://git@github.com/cloudposse/atmos.git
`
				require.Equal(t, want, content)
			},
		},
		{
			name:      "no tokens still writes the anonymous-only rule",
			serverURL: "http://127.0.0.1:9999",
			tokens:    nil,
			check: func(t *testing.T, content string) {
				t.Helper()

				want := `[url "http://127.0.0.1:9999/cloudposse/atmos.git"]
	insteadOf = https://github.com/cloudposse/atmos.git
	insteadOf = ssh://git@github.com/cloudposse/atmos.git
`
				require.Equal(t, want, content)
			},
		},
		{
			name:      "trailing slash on serverURL does not produce a double slash",
			serverURL: "http://127.0.0.1:54321/",
			tokens:    nil,
			check: func(t *testing.T, content string) {
				t.Helper()

				require.Contains(t, content, `[url "http://127.0.0.1:54321/cloudposse/atmos.git"]`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "gitconfig")
			require.NoError(t, WriteGitConfig(path, tt.serverURL, tt.tokens))

			content, err := os.ReadFile(path)
			require.NoError(t, err)

			tt.check(t, string(content))
		})
	}
}
