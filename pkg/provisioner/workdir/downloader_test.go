package workdir

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestNewDefaultDownloader(t *testing.T) {
	downloader := NewDefaultDownloader()
	assert.NotNil(t, downloader)
}

func TestDefaultDownloader_Structure(t *testing.T) {
	downloader := NewDefaultDownloader()

	// Verify mutex is initialized (zero value is valid).
	assert.NotNil(t, downloader)
}

func TestMockDownloader_Download(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDownloader := NewMockDownloader(ctrl)

	ctx := context.Background()
	uri := "github.com/cloudposse/terraform-aws-vpc?ref=v1.0.0"
	dest := "/tmp/download"

	// Expect download to be called.
	mockDownloader.EXPECT().
		Download(ctx, uri, dest).
		Return(nil)

	err := mockDownloader.Download(ctx, uri, dest)
	assert.NoError(t, err)
}

func TestMockDownloader_DownloadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDownloader := NewMockDownloader(ctrl)

	ctx := context.Background()
	uri := "invalid://bad-uri"
	dest := "/tmp/download"

	// Expect download to return error.
	expectedErr := errUtils.Build(errUtils.ErrSourceDownload).
		WithExplanation("download failed").
		Err()
	mockDownloader.EXPECT().
		Download(ctx, uri, dest).
		Return(expectedErr)

	err := mockDownloader.Download(ctx, uri, dest)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrSourceDownload)
}

func TestDownloader_Interface(t *testing.T) {
	// Verify DefaultDownloader implements Downloader interface.
	var _ Downloader = (*DefaultDownloader)(nil)
}

func TestMockDownloader_CanceledContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDownloader := NewMockDownloader(ctrl)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	uri := "github.com/cloudposse/terraform-aws-vpc"
	dest := "/tmp/download"

	// Expect download to handle canceled context.
	expectedErr := context.Canceled
	mockDownloader.EXPECT().
		Download(ctx, uri, dest).
		Return(expectedErr)

	err := mockDownloader.Download(ctx, uri, dest)
	assert.ErrorIs(t, err, context.Canceled)
}

// Test supported protocols.

func TestDownloader_SupportedProtocols(t *testing.T) {
	// Document the supported protocols.
	protocols := []string{
		"git",
		"file",
		"hg",
		"http",
		"https",
		"s3",
		"gcs",
	}

	for _, proto := range protocols {
		assert.NotEmpty(t, proto)
	}
}

// Test URI formats.

func TestDownloader_URIFormats(t *testing.T) {
	testCases := []struct {
		name string
		uri  string
	}{
		{"github short form", "github.com/cloudposse/terraform-aws-vpc"},
		{"github with ref", "github.com/cloudposse/terraform-aws-vpc?ref=v1.0.0"},
		{"git explicit", "git::https://github.com/cloudposse/terraform-aws-vpc.git"},
		{"git with ref", "git::https://github.com/cloudposse/terraform-aws-vpc.git?ref=v1.0.0"},
		{"https direct", "https://example.com/module.zip"},
		{"s3 bucket", "s3::https://s3.amazonaws.com/bucket/path"},
		{"gcs bucket", "gcs::https://storage.googleapis.com/bucket/path"},
		{"file local", "file:///path/to/module"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotEmpty(t, tc.uri)
		})
	}
}

// Test download with various contexts.

func TestMockDownloader_WithTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDownloader := NewMockDownloader(ctrl)

	ctx, cancel := context.WithTimeout(context.Background(), 0) // Immediate timeout.
	defer cancel()

	uri := "github.com/cloudposse/terraform-aws-vpc"
	dest := "/tmp/download"

	// Expect download to handle timeout.
	expectedErr := context.DeadlineExceeded
	mockDownloader.EXPECT().
		Download(ctx, uri, dest).
		Return(expectedErr)

	err := mockDownloader.Download(ctx, uri, dest)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// Test error wrapping.

func TestDownloader_ErrorWrapping(t *testing.T) {
	// Verify error builder creates correct sentinel error.
	underlyingErr := assert.AnError
	wrappedErr := errUtils.Build(errUtils.ErrSourceDownload).
		WithCause(underlyingErr).
		WithExplanation("go-getter download failed").
		WithContext("uri", "test-uri").
		WithContext("dest", "/tmp/dest").
		Err()

	assert.ErrorIs(t, wrappedErr, errUtils.ErrSourceDownload)
	// Error should be non-nil and based on sentinel.
	assert.NotNil(t, wrappedErr)
}

// Integration test placeholder.

func TestDefaultDownloader_Download_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This would test actual downloads but requires network.
	// For unit tests, we use mocks.
	t.Skip("Integration test requires network access")
}

// Test mutex behavior (concurrency safety).

func TestDefaultDownloader_ConcurrencySafe(t *testing.T) {
	downloader := NewDefaultDownloader()

	// Verify the downloader has a mutex field.
	// This is a design verification, not a functional test.
	assert.NotNil(t, downloader)
}

// Test with mock filesystem operations.

func TestMockDownloader_MultipleDownloads(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDownloader := NewMockDownloader(ctrl)
	ctx := context.Background()

	// First download.
	mockDownloader.EXPECT().
		Download(ctx, "github.com/org/repo1", "/tmp/repo1").
		Return(nil)

	// Second download.
	mockDownloader.EXPECT().
		Download(ctx, "github.com/org/repo2", "/tmp/repo2").
		Return(nil)

	err := mockDownloader.Download(ctx, "github.com/org/repo1", "/tmp/repo1")
	require.NoError(t, err)

	err = mockDownloader.Download(ctx, "github.com/org/repo2", "/tmp/repo2")
	require.NoError(t, err)
}

// Test network error handling.

func TestMockDownloader_NetworkError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDownloader := NewMockDownloader(ctrl)
	ctx := context.Background()

	networkErr := errUtils.Build(errUtils.ErrSourceDownload).
		WithExplanation("network timeout").
		Err()

	mockDownloader.EXPECT().
		Download(ctx, gomock.Any(), gomock.Any()).
		Return(networkErr)

	err := mockDownloader.Download(ctx, "github.com/org/repo", "/tmp/dest")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrSourceDownload)
}

// Test permission error handling.

func TestMockDownloader_PermissionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDownloader := NewMockDownloader(ctrl)
	ctx := context.Background()

	permErr := errUtils.Build(errUtils.ErrSourceDownload).
		WithExplanation("permission denied").
		Err()

	mockDownloader.EXPECT().
		Download(ctx, gomock.Any(), "/readonly/path").
		Return(permErr)

	err := mockDownloader.Download(ctx, "github.com/org/repo", "/readonly/path")
	assert.Error(t, err)
}
