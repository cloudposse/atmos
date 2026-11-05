package downloader

import (
	"context"
	"os"
	"path/filepath"
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
	src := mock.URL() + "/cloudposse/atmos/raw/main/README.md"
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

// Raw content downloads use a separate service from the REST API. Exhausting the
// REST budget must not prevent mixins or stack imports from being downloaded.
func TestFileDownloader_RawContentIgnoresRESTQuota(t *testing.T) {
	for _, metadata := range []bool{false, true} {
		name := "fetch"
		if metadata {
			name = "metadata"
		}
		t.Run(name, func(t *testing.T) {
			mock := httpmock.NewGitHubMockServer(t)
			mock.Setenv(t)
			mock.SetRateLimit(0, time.Now().Add(time.Hour))
			const path = "cloudposse/terraform-null-label/0.25.0/exports/context.tf"
			const content = "variable \"namespace\" {}\n"
			mock.RegisterFile(path, content)
			dest := filepath.Join(t.TempDir(), "context.tf")
			fd := NewGoGetterDownloader(nil, WithHTTPClient(mock.HTTPClient()))
			var err error
			if metadata {
				_, err = fd.(ContextFileDownloader).FetchWithMetadataContext(context.Background(), "https://raw.githubusercontent.com/"+path, dest, ClientModeFile, time.Second)
			} else {
				err = fd.Fetch("https://raw.githubusercontent.com/"+path, dest, ClientModeFile, time.Second)
			}
			require.NoError(t, err)
			downloaded, err := os.ReadFile(dest)
			require.NoError(t, err)
			assert.Equal(t, content, string(downloaded))
			assert.Zero(t, mock.RequestCount("/api/v3/rate_limit"), "raw downloads must not wait for REST quota")
			assert.Positive(t, mock.RequestCount("/"+path), "raw content must be fetched over HTTP")
		})
	}
}
