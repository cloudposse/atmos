// Package threadcontext shares the calling task's cancellation context between
// the automation runtime and its standard-library bindings.
package threadcontext

import (
	"context"

	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/perf"
)

const key = "atmos.starlark.context"

// Set attaches the invocation or parallel task context before evaluation starts.
func Set(ctx context.Context, thread *starlark.Thread) {
	defer perf.Track(nil, "threadcontext.Set")()
	thread.SetLocal(key, ctx)
}

// Get returns the calling task's context. Standalone bindings without a host
// context use Background, like an ordinary Starlark thread.
func Get(thread *starlark.Thread) context.Context {
	defer perf.Track(nil, "threadcontext.Get")()
	if ctx, ok := thread.Local(key).(context.Context); ok {
		return ctx
	}
	return context.Background()
}
