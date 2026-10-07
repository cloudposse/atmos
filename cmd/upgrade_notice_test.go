package cmd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests/testhelpers/httpmock"
)

func TestCheckForAtmosUpdateNoticeAndDailyCache(t *testing.T) {
	_ = NewTestKit(t)
	mock := httpmock.NewGitHubMockServer(t)
	mock.Setenv(t)
	for _, key := range []string{"ATMOS_PRO_GITHUB_TOKEN", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN", "ATMOS_GITHUB_CLI"} {
		t.Setenv(key, "")
	}
	mock.RegisterRelease("cloudposse", "atmos", httpmock.ReleaseSpec{TagName: "v9999.0.0"})
	config := schema.AtmosConfiguration{BasePathAbsolute: t.TempDir()}
	config.Toolchain.InstallPath = "tools"
	config.Version.Check.Enabled = true
	config.Version.Check.Frequency = "1d"
	started := time.Now().Unix()
	stdout, stderr := captureStdoutStderr(t, func() {
		ensureIOInitialized(t)
		CheckForAtmosUpdateAndPrintMessage(config)
	})
	assert.Empty(t, stdout, "automatic notices must preserve command stdout")
	assert.Contains(t, stderr, "Update available!")
	assert.Contains(t, stderr, "9999.0.0")
	assert.Contains(t, stderr, "https://atmos.tools/install")
	assert.NotContains(t, stderr, "Run:", "a test binary has no established installation owner")
	cache, err := cfg.LoadCache()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, cache.LastChecked, started)
	assert.Len(t, mock.Requests(), 1)

	stdout, stderr = captureStdoutStderr(t, func() {
		ensureIOInitialized(t)
		CheckForAtmosUpdateAndPrintMessage(config)
	})
	assert.Empty(t, stdout)
	assert.Empty(t, stderr, "daily cache must suppress repeat notices")
	assert.Len(t, mock.Requests(), 1, "daily cache must suppress repeat API calls")
}
