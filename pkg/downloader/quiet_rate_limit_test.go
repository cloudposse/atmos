package downloader

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/github"
)

func TestQuietRateLimitWaitKeepsThresholdAndCancellation(t *testing.T) {
	cases := []struct {
		name     string
		status   *github.RateLimitStatus
		checkErr error
		cancel   bool
	}{
		{name: "check unavailable", checkErr: errors.New("network unavailable")},
		{name: "status unavailable"},
		{name: "sufficient budget", status: &github.RateLimitStatus{Remaining: MinRateLimitRemaining, ResetAt: time.Now().Add(time.Hour)}},
		{name: "already reset", status: &github.RateLimitStatus{Remaining: 0, ResetAt: time.Now().Add(-time.Second)}},
		{name: "wait interrupted", status: &github.RateLimitStatus{Remaining: 0, ResetAt: time.Now().Add(time.Hour)}, cancel: true},
		{name: "wait finishes", status: &github.RateLimitStatus{Remaining: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			checked := false
			err := waitForRateLimitQuietly(ctx, func(context.Context) (*github.RateLimitStatus, error) {
				checked = true
				if tc.cancel {
					cancel()
				}
				if tc.name == "wait finishes" {
					tc.status.ResetAt = time.Now().Add(time.Millisecond)
				}
				return tc.status, tc.checkErr
			})
			assert.True(t, checked)
			if tc.cancel {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
			}
			if tc.name == "wait finishes" {
				assert.False(t, time.Now().Before(tc.status.ResetAt), "must wait until reset")
			}
		})
	}
}

func TestMetadataRateLimitChoosesCallerOwnedProgress(t *testing.T) {
	for _, observed := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy display", true: "observer display"}[observed], func(t *testing.T) {
			interactiveCalls, quietCalls := 0, 0
			previous := github.RateLimitWaiter
			github.RateLimitWaiter = func(context.Context, int) error { interactiveCalls++; return nil }
			t.Cleanup(func() { github.RateLimitWaiter = previous })
			factory := &goGetterClientFactory{}
			if observed {
				factory.onRetry = func(int) {}
			}
			fd := NewFileDownloader(factory).(*fileDownloader)
			require.NoError(t, fd.waitForMetadataRateLimit(context.Background(), func(context.Context) (*github.RateLimitStatus, error) {
				quietCalls++
				return &github.RateLimitStatus{Remaining: MinRateLimitRemaining}, nil
			}))
			if observed {
				assert.Equal(t, 0, interactiveCalls)
				assert.Equal(t, 1, quietCalls)
			} else {
				assert.Equal(t, 1, interactiveCalls)
				assert.Equal(t, 0, quietCalls)
			}
		})
	}
}
