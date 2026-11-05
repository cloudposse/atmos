package exec

import (
	"testing"

	"github.com/stretchr/testify/require"

	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
)

func TestDeferredTargetDisabledManagerIsStable(t *testing.T) {
	ac, _, _ := setupDeferredCacheTarget(t)
	authdeferred.ConfigureAuth(ac, "false")
	first, cache, err := deferredTargetAuthAndCache(ac, "target", "dev")
	require.NoError(t, err)
	require.NotNil(t, first)
	for range 5 {
		next, nextCache, err := deferredTargetAuthAndCache(ac, "target", "dev")
		require.NoError(t, err)
		require.Same(t, cache, nextCache)
		require.Same(t, first, next, "output cache keys retain manager identity")
	}
	authdeferred.ConfigureAuth(ac, "false")
	next, nextCache, err := deferredTargetAuthAndCache(ac, "target", "dev")
	require.NoError(t, err)
	require.NotSame(t, cache, nextCache)
	require.NotSame(t, first, next, "another invocation must not reuse ambient-auth outputs")
}
