package aws

// Tests for the best-effort cross-process lock on the SSO token cache (sso_cache_lock.go).

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/filelock"
)

// errNoCacheDir stands in for an unavailable XDG cache directory.
var errNoCacheDir = errors.New("no cache directory")

// TestWithTokenCacheLock_RunsWithoutCachePath verifies that fn still runs, once,
// when the cache path (and so the lock file) cannot be determined.
func TestWithTokenCacheLock_RunsWithoutCachePath(t *testing.T) {
	p := newPollTestProvider()
	p.cacheStorage = &mockCacheStorage{getXDGCacheDirFunc: func(string, os.FileMode) (string, error) {
		return "", errNoCacheDir
	}}

	runs := 0
	p.withTokenCacheLock(context.Background(), func() error {
		runs++
		return nil
	})
	assert.Equal(t, 1, runs)
}

// TestWithTokenCacheLock_RunsWhenLockIsHeld verifies that a lock held by
// another process past the deadline does not block authentication: fn runs
// once without the lock.
func TestWithTokenCacheLock_RunsWhenLockIsHeld(t *testing.T) {
	p := newRefreshTestProvider(t, "https://unused.example.com")
	tokenPath, err := p.getTokenCachePath()
	require.NoError(t, err)

	held := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- filelock.New(tokenPath+".lock").WithExclusive(context.Background(), func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	runs := 0
	p.withTokenCacheLock(ctx, func() error {
		runs++
		return nil
	})
	assert.Equal(t, 1, runs, "fn runs without the lock once acquisition times out")

	close(release)
	require.NoError(t, <-holderDone)
}

// TestPollForAccessToken_NormalizesZeroInterval verifies that a missing polling
// interval is treated as one second rather than busy-polling.
func TestPollForAccessToken_NormalizesZeroInterval(t *testing.T) {
	srv := newFakeOIDCServer(t, tokenReply("access", ""))
	p := newPollTestProvider()
	recorder := &recordingWait{}
	p.pollWait = recorder.wait

	register, auth := pollTestInputs(0)
	result, err := p.pollForAccessToken(context.Background(), newFakeOIDCClient(srv.URL), register, auth)
	require.NoError(t, err)
	assert.Equal(t, "access", result.AccessToken)
	assert.Equal(t, []time.Duration{time.Second}, recorder.waits)
}
