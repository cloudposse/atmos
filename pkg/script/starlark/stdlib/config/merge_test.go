package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/script/starlark/internal/threadcontext"
)

func TestConfigMergeCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	thread := &starlark.Thread{}
	threadcontext.Set(ctx, thread)
	base := &configRecord{starlark.NewDict(0)}
	base.Freeze()
	method, err := base.Attr("merge")
	require.NoError(t, err)
	_, err = starlark.Call(thread, method, starlark.Tuple{base}, nil)
	require.ErrorIs(t, err, context.Canceled)
}
