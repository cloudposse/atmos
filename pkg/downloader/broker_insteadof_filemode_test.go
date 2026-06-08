package downloader

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// writeBrokerGitConfig writes a gitconfig in the github/sts file-mode shape (per-owner insteadOf,
// https + ssh) — exactly what pkg/auth/integrations/github writeGitConfigFile produces — and
// returns its path.
func writeBrokerGitConfig(t *testing.T, host, owner, token string) string {
	t.Helper()
	base := "https://x-access-token:" + token + "@" + host + "/" + owner + "/"
	content := "[url \"" + base + "\"]\n" +
		"\tinsteadOf = https://" + host + "/" + owner + "/\n" +
		"\tinsteadOf = ssh://git@" + host + "/" + owner + "/\n"
	path := filepath.Join(t.TempDir(), "git.config")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// setBrokerIncludePath sets GIT_CONFIG_* exactly as the github/sts broker exports them in file mode:
// a single include.path entry pointing at the on-disk gitconfig that carries the insteadOf rewrites
// (the rewrites are inside the file, not in the env).
func setBrokerIncludePath(t *testing.T, configPath string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "include.path")
	t.Setenv("GIT_CONFIG_VALUE_0", configPath)
}

// TestCustomGitDetector_FileModeInsteadOf_NotShadowed is the file-mode analog of
// TestCustomGitDetector_BrokerInsteadOf_NotShadowed: when the broker materializes its rewrite via an
// include.path gitconfig (GitConfigModeFile), the detector must still recognize the coverage and skip
// ambient injection for the matched owner, while a non-covered owner is still injected.
func TestCustomGitDetector_FileModeInsteadOf_NotShadowed(t *testing.T) {
	const (
		mintedOwner  = "example-org"
		otherOwner   = "other-org"
		ambientToken = "wrong-ambient"
		mintedToken  = "ghs_minted"
	)

	configPath := writeBrokerGitConfig(t, "github.com", mintedOwner, mintedToken)
	setBrokerIncludePath(t, configPath)
	// Clear any ambient brokered token so resolveToken uses only the configured ambient token.
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			InjectGithubToken: true,
			GithubToken:       ambientToken,
		},
	}

	t.Run("matched owner is not shadowed (file-mode include.path)", func(t *testing.T) {
		src := "github.com/" + mintedOwner + "/private-repo//stacks/catalog/queue?ref=main"
		detector := NewCustomGitDetector(atmosConfig, src)
		finalURL, detected, err := detector.Detect(src, "")
		require.NoError(t, err)
		require.True(t, detected)

		assert.NotContains(t, finalURL, ambientToken,
			"ambient token must NOT be injected when a file-mode broker insteadOf covers this owner")
		assert.NotContains(t, finalURL, "@",
			"URL must carry no userinfo so git's include.path insteadOf rewrite can match and win")
	})

	t.Run("different owner is still injected", func(t *testing.T) {
		src := "github.com/" + otherOwner + "/repo?ref=main"
		detector := NewCustomGitDetector(atmosConfig, src)
		finalURL, detected, err := detector.Detect(src, "")
		require.NoError(t, err)
		require.True(t, detected)

		assert.Contains(t, finalURL, ambientToken,
			"a non-covered owner has no insteadOf coverage, so the ambient token is still injected")
	})
}

// TestBrokerInsteadOfMatchesURL_FileMode is a direct table-driven test of the include.path branch of
// the guard: it must resolve the referenced gitconfig and match only an https insteadOf for the same
// host+owner.
func TestBrokerInsteadOfMatchesURL_FileMode(t *testing.T) {
	httpsConfig := writeBrokerGitConfig(t, "github.com", "acme", "tok") // https + ssh insteadOf.

	sshOnlyConfig := filepath.Join(t.TempDir(), "ssh.config")
	require.NoError(t, os.WriteFile(sshOnlyConfig,
		[]byte("[url \"ssh://git@github.com/acme/\"]\n\tinsteadOf = ssh://git@github.com/acme/\n"), 0o600))

	tests := []struct {
		name   string
		path   string
		urlStr string
		want   bool
	}{
		{"https insteadOf in file matches same host+owner", httpsConfig, "https://github.com/acme/repo", true},
		{"different owner does not match", httpsConfig, "https://github.com/other/repo", false},
		{"different host does not match", httpsConfig, "https://gitlab.com/acme/repo", false},
		{"ssh-only file does not match an https clone", sshOnlyConfig, "https://github.com/acme/repo", false},
		{"missing file does not match", filepath.Join(t.TempDir(), "nope.config"), "https://github.com/acme/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GIT_CONFIG_COUNT", "1")
			t.Setenv("GIT_CONFIG_KEY_0", "include.path")
			t.Setenv("GIT_CONFIG_VALUE_0", tt.path)

			parsed, err := url.Parse(tt.urlStr)
			require.NoError(t, err)
			got := brokerInsteadOfMatchesURL(parsed, strings.ToLower(parsed.Hostname()))
			assert.Equal(t, tt.want, got)
		})
	}
}
