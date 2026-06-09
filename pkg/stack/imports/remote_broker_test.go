package imports

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/broker"
	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeInsteadOfBroker is a credential broker that exports a github/sts-style insteadOf rewrite when
// provisioned. It stands in for the real Atmos Pro broker so the import-resolution ordering can be
// exercised without CI or network.
type fakeInsteadOfBroker struct {
	env map[string]string
}

func (fakeInsteadOfBroker) Name() string                            { return "fake-insteadof-broker" }
func (fakeInsteadOfBroker) Enabled(*schema.AtmosConfiguration) bool { return true }
func (f fakeInsteadOfBroker) Provision(context.Context, *schema.AtmosConfiguration) (map[string]string, error) {
	return f.env, nil
}

// TestRemoteImporter_GitSubdir_ProvisionsBrokerBeforeDetect reproduces the in-process shadowing bug:
// a git-subdir import is resolved by detecting the git source URL BEFORE downloading, so unless the
// credential broker is provisioned first, the detector runs without the broker's GIT_CONFIG_*
// insteadOf rewrite and bakes the ambient GITHUB_TOKEN into the URL — shadowing the minted token.
//
// With the fix, resolveGitSubdir provisions the broker before detecting, so the detector sees the
// rewrite for github.com/acme and skips ambient injection. The URL handed to the downloader then
// carries no userinfo, letting git's insteadOf (with the minted token) win.
func TestRemoteImporter_GitSubdir_ProvisionsBrokerBeforeDetect(t *testing.T) {
	restore := broker.SwapRegistryForTest()
	t.Cleanup(restore)

	const (
		mintedToken  = "ghs_minted"
		ambientToken = "ambient-wrong"
	)
	base := "https://x-access-token:" + mintedToken + "@github.com/acme/"
	broker.Register(fakeInsteadOfBroker{env: map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "url." + base + ".insteadOf",
		"GIT_CONFIG_VALUE_0": "https://github.com/acme/",
	}})

	// Register the env keys the broker will os.Setenv so the test framework restores them afterward
	// (the broker writes them outside t.Setenv tracking, so pre-registering prevents leakage).
	t.Setenv("GIT_CONFIG_COUNT", "")
	t.Setenv("GIT_CONFIG_KEY_0", "")
	t.Setenv("GIT_CONFIG_VALUE_0", "")
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")

	testCache, err := cache.NewFileCache("test", cache.WithBaseDir(t.TempDir()))
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	mockDownloader := downloader.NewMockFileDownloader(ctrl)

	var fetchedURL string
	mockDownloader.EXPECT().
		Fetch(gomock.Any(), gomock.Any(), downloader.ClientModeDir, gomock.Any()).
		DoAndReturn(func(src, _ string, _ downloader.ClientMode, _ time.Duration) error {
			fetchedURL = src
			return errors.New("stop after capturing the detected source URL")
		})

	importer, err := NewRemoteImporter(
		&schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{InjectGithubToken: true, GithubToken: ambientToken},
		},
		WithCache(testCache),
		WithDownloader(mockDownloader),
	)
	require.NoError(t, err)

	// Error is expected — the mock downloader stops resolution right after capturing the URL.
	_, _ = importer.Resolve("git::https://github.com/acme/repo.git//stacks/catalog/queue?ref=main")

	require.NotEmpty(t, fetchedURL, "downloader.Fetch must have been called with the detected source URL")
	assert.NotContains(t, fetchedURL, ambientToken,
		"broker must be provisioned before the eager detect so the ambient token is not baked into the URL")
	assert.NotContains(t, fetchedURL, "@",
		"the detected source URL must carry no userinfo so git's insteadOf rewrite (minted token) wins")
}
