package oci

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
)

type retryObserverKey struct{}

// WithRetryObserver routes retry notices to the caller and suppresses terminal
// diagnostics during this operation. Errors are still returned to the caller.
// The observer is scoped to this context, including provenance resolution.
func WithRetryObserver(ctx context.Context, observer func(int)) context.Context {
	defer perf.Track(nil, "oci.WithRetryObserver")()
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, retryObserverKey{}, observer)
}

func observed(ctx context.Context) bool {
	return ctx.Value(retryObserverKey{}) != nil
}

func reportRetry(ctx context.Context, attempt int) bool {
	observer, ok := ctx.Value(retryObserverKey{}).(func(int))
	if ok {
		observer(attempt)
	}
	return ok
}
