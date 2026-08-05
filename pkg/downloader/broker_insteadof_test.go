package downloader

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// setBrokerInsteadOf sets the GIT_CONFIG_* environment variables exactly as the github/sts auth
// broker exports them for a single owner (https + ssh insteadOf rewrites), so detector behavior can
// be exercised end-to-end without a real broker or network.
func setBrokerInsteadOf(t *testing.T, host, owner, token string) {
	t.Helper()
	base := "https://x-access-token:" + token + "@" + host + "/" + owner + "/"
	key := "url." + base + ".insteadOf"
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", key)
	t.Setenv("GIT_CONFIG_VALUE_0", "https://"+host+"/"+owner+"/")
	t.Setenv("GIT_CONFIG_KEY_1", key)
	t.Setenv("GIT_CONFIG_VALUE_1", "ssh://git@"+host+"/"+owner+"/")
}

// TestCustomGitDetector_BrokerInsteadOf_NotShadowed reproduces the broker-shadowing failure: the
// github/sts broker exports an insteadOf rewrite carrying the correct minted token, but
// CustomGitDetector injects the wrong ambient token into the URL, whose userinfo then prevents git's
// insteadOf from matching. With the fix, injection is skipped for the matched owner (git's rewrite
// wins) while a different, non-minted owner still gets the ambient token injected.
func TestCustomGitDetector_BrokerInsteadOf_NotShadowed(t *testing.T) {
	const (
		mintedOwner  = "example-org"
		otherOwner   = "other-org"
		ambientToken = "wrong-ambient"
		mintedToken  = "ghs_minted"
	)

	setBrokerInsteadOf(t, "github.com", mintedOwner, mintedToken)
	// Clear any ambient brokered token so resolveToken uses only the configured ambient token.
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			InjectGithubToken: true,
			GithubToken:       ambientToken,
		},
	}

	t.Run("matched owner is not shadowed", func(t *testing.T) {
		src := "github.com/" + mintedOwner + "/private-repo//stacks/catalog/queue?ref=main"
		detector := NewCustomGitDetector(atmosConfig, src)
		finalURL, detected, err := detector.Detect(src, "")
		require.NoError(t, err)
		require.True(t, detected)

		assert.NotContains(t, finalURL, ambientToken,
			"ambient token must NOT be injected when a broker insteadOf covers this owner")
		assert.NotContains(t, finalURL, "@",
			"URL must carry no userinfo so git's insteadOf rewrite can match and win")
	})

	t.Run("different owner is still injected", func(t *testing.T) {
		src := "github.com/" + otherOwner + "/repo?ref=main"
		detector := NewCustomGitDetector(atmosConfig, src)
		finalURL, detected, err := detector.Detect(src, "")
		require.NoError(t, err)
		require.True(t, detected)

		assert.Contains(t, finalURL, ambientToken,
			"a non-minted owner has no insteadOf coverage, so the ambient token is still injected")
		assert.True(t, strings.Contains(finalURL, "x-access-token:"+ambientToken+"@"),
			"injection should use x-access-token userinfo for github")
	})
}
