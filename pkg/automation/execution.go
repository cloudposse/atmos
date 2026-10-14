package automation

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecutionPolicy bounds an operation and its retry waits with one deadline.
// A nil Retry runs once. Clock is optional and controls retry backoff only.
type ExecutionPolicy struct {
	Timeout time.Duration
	Retry   *schema.RetryConfig
	Clock   retry.Clock
}

// Execute runs a context-aware operation using the shared timeout and retry
// policy. The caller decides which failures are safe to retry.
func (p ExecutionPolicy) Execute(ctx context.Context, operation func(context.Context) error, shouldRetry func(error) bool) error {
	defer perf.Track(nil, "automation.ExecutionPolicy.Execute")()

	if p.Timeout < 0 {
		return fmt.Errorf("%w: timeout must not be negative", errUtils.ErrAutomation)
	}
	if err := retry.Validate(p.Retry); err != nil {
		return fmt.Errorf("%w: invalid retry policy: %w", errUtils.ErrAutomation, err)
	}
	if p.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Retry == nil {
		return operation(ctx)
	}
	return retry.New(*p.Retry, retry.WithClock(p.Clock)).ExecuteWithPredicate(ctx, func() error {
		return operation(ctx)
	}, shouldRetry)
}
