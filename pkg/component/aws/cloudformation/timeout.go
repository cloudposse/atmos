package cloudformation

import (
	"context"
	"time"
)

// withDefaultTimeout supplies a wait budget only when the caller has not already
// supplied an operation deadline. Existing deadlines also cancel stalled SDK calls.
func withDefaultTimeout(ctx context.Context, fallback time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, fallback)
}
