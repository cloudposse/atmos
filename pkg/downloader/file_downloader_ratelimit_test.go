package downloader

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/tests/testhelpers/httpmock"
)

// rateLimitTestTimeout bounds how long a rate-limit pre-check test is allowed to take before
// it is considered to have blocked (as opposed to proceeding immediately, per
// pkg/github.waitForRateLimitImpl's "reset already passed" branch).
const rateLimitTestTimeout = 2 * time.Second

// TestFileDownloader_Fetch_GitHubURL_RateLimitPreCheckDoesNotBlockWhenAlreadyReset asserts the
// current behavior of Fetch's rate-limit pre-check (pkg/downloader/file_downloader.go:
// isGitHubHTTPURL + github.WaitForRateLimit) when the configured budget is exhausted
// (remaining=0) but the reset time has already passed: it must not add a real wait before the
// actual download proceeds. The mock's request log confirms the pre-check actually fired
// (hit GET /api/v3/rate_limit) rather than this being a false pass from the check being
// skipped entirely.
func TestFileDownloader_Fetch_GitHubURL_RateLimitPreCheckDoesNotBlockWhenAlreadyReset(t *testing.T) {
	mock := httpmock.NewGitHubMockServer(t)
	mock.Setenv(t)
	mock.SetRateLimit(0, time.Now().Add(-time.Minute))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockDownloadClient(ctrl)
	mockFactory := NewMockClientFactory(ctrl)
	src := "https://raw.githubusercontent.com/cloudposse/atmos/main/README.md"
	mockFactory.EXPECT().NewClient(gomock.Any(), src, "dest", ClientModeFile).Return(mockClient, nil)
	mockClient.EXPECT().Get().Return(nil)

	fd := NewFileDownloader(mockFactory)

	start := time.Now()
	err := fd.Fetch(src, "dest", ClientModeFile, 30*time.Second)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Less(t, elapsed, rateLimitTestTimeout, "an already-passed rate-limit reset must not block the fetch")
	assert.Positive(t, mock.RequestCount("/api/v3/rate_limit"), "isGitHubHTTPURL(src) should have triggered the rate-limit pre-check")
}

// TestFileDownloader_Fetch_NonGitHubURL_SkipsRateLimitPreCheck confirms the pre-check is scoped
// to GitHub URLs (isGitHubHTTPURL): a non-GitHub source must never touch the mock's rate-limit
// endpoint at all.
func TestFileDownloader_Fetch_NonGitHubURL_SkipsRateLimitPreCheck(t *testing.T) {
	mock := httpmock.NewGitHubMockServer(t)
	mock.Setenv(t)
	mock.SetRateLimit(0, time.Now().Add(time.Hour))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockDownloadClient(ctrl)
	mockFactory := NewMockClientFactory(ctrl)
	src := "https://example.com/asset.tar.gz"
	mockFactory.EXPECT().NewClient(gomock.Any(), src, "dest", ClientModeFile).Return(mockClient, nil)
	mockClient.EXPECT().Get().Return(nil)

	fd := NewFileDownloader(mockFactory)
	err := fd.Fetch(src, "dest", ClientModeFile, 30*time.Second)

	require.NoError(t, err)
	assert.Equal(t, 0, mock.RequestCount("/api/v3/rate_limit"))
}
