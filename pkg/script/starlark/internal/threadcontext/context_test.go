package threadcontext

import (
	"context"
	"github.com/stretchr/testify/assert"
	"go.starlark.net/starlark"
	"testing"
)

func TestCallingContext(t *testing.T) {
	t.Parallel()
	thread := &starlark.Thread{}
	assert.Equal(t, context.Background(), Get(thread))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	Set(thread, ctx)
	assert.Same(t, ctx, Get(thread))
	cancel()
	assert.ErrorIs(t, Get(thread).Err(), context.Canceled)
}
