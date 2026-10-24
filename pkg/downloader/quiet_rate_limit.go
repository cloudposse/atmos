package downloader

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/github"
)

func (fd *fileDownloader) waitForMetadataRateLimit(ctx context.Context, check func(context.Context) (*github.RateLimitStatus, error)) error {
	if factory, ok := fd.clientFactory.(*goGetterClientFactory); ok && (factory.onRetry != nil || factory.progress != nil) {
		return waitForRateLimitQuietly(ctx, check)
	}
	return github.WaitForRateLimit(ctx, MinRateLimitRemaining)
}

// waitForRateLimitQuietly keeps the same reset threshold as the interactive waiter
// while allowing the caller to remain the sole owner of terminal progress.
func waitForRateLimitQuietly(ctx context.Context, check func(context.Context) (*github.RateLimitStatus, error)) error {
	status, err := check(ctx)
	if err != nil || status == nil || status.Remaining >= MinRateLimitRemaining {
		return ctx.Err()
	}
	wait := time.Until(status.ResetAt)
	if wait <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
