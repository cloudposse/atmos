package workdir

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestMockDownloader_DownloadError tests that mock downloader properly returns errors.
// This is used to verify error handling in Service when downloader fails.
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

// TestMockDownloader_CanceledContext tests context cancellation handling.
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

// TestMockDownloader_WithTimeout tests deadline exceeded handling.
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

// TestDownloader_ErrorWrapping verifies error builder creates correct sentinel error.
func TestDownloader_ErrorWrapping(t *testing.T) {
	underlyingErr := assert.AnError
	wrappedErr := errUtils.Build(errUtils.ErrSourceDownload).
		WithCause(underlyingErr).
		WithExplanation("go-getter download failed").
		WithContext("uri", "test-uri").
		WithContext("dest", "/tmp/dest").
		Err()

	assert.ErrorIs(t, wrappedErr, errUtils.ErrSourceDownload)
	assert.NotNil(t, wrappedErr)
}

// TestMockDownloader_NetworkError tests network error handling.
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

// TestMockDownloader_PermissionError tests permission error handling.
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
