package deferred

import (
	"testing"

	"github.com/stretchr/testify/require"

	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValueCacheIsolation(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	authdeferred.ConfigureAuth(ac, "")
	info := &schema.ConfigAndStacksInfo{Stack: "dev", Component: "example", ComponentSection: map[string]any{
		"auth": map[string]any{"identities": map[string]any{"local": map[string]any{"kind": "aws/user", "default": true, "credentials": "first-account"}}},
	}}
	first := CacheFor(ac, info)
	first.Store("terraform.state", map[string]any{"id": "resolved"})
	require.Same(t, first, CacheFor(ac, info))
	value, found := first.Load("terraform.state")
	require.True(t, found)
	require.Equal(t, map[string]any{"id": "resolved"}, value)
	_, found = first.Load("atmos.Component")
	require.False(t, found, "state and template results use separate namespaces")

	info.ComponentSection["auth"].(map[string]any)["identities"].(map[string]any)["local"].(map[string]any)["credentials"] = "second-account"
	require.NotSame(t, first, CacheFor(ac, info), "identical names with different credentials must not share results")
	second := CacheFor(ac, info)
	info.Stack = "prod"
	require.NotSame(t, second, CacheFor(ac, info))
	second = CacheFor(ac, info)
	info.Component = "other"
	require.NotSame(t, second, CacheFor(ac, info))
	second = CacheFor(ac, info)
	info.AuthDisabled = true
	require.NotSame(t, second, CacheFor(ac, info))
	second = CacheFor(ac, info)
	authdeferred.ConfigureAuth(ac, "")
	require.NotSame(t, second, CacheFor(ac, info), "another invocation must resolve fresh values")
}

func TestValueCacheRejectsComputedAndUnsupportedValues(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	authdeferred.ConfigureAuth(ac, "")
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}}
	cache := CacheFor(ac, info)
	cache.Store("result", map[string]any{"siblings": []any{"resolved", degradation.AtmosComputedValue{}}})
	_, found := cache.Load("result")
	require.False(t, found)
	info.ComponentSection["unsupported"] = make(chan int)
	require.Nil(t, CacheFor(ac, info))
	require.Nil(t, CacheFor(nil, info))
	require.Nil(t, CacheFor(ac, nil))
	var disabled *ValueCache
	disabled.Store("result", "ignored")
	_, found = disabled.Load("result")
	require.False(t, found)
	authdeferred.ConfigureAuth(ac, "explicit")
	require.Nil(t, CacheFor(ac, info))
}

func TestValueCacheContextFollowsInvocation(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	authdeferred.ConfigureAuth(ac, "")
	info := &schema.ConfigAndStacksInfo{Stack: "dev", Component: "example"}
	cache := CacheFor(ac, info)
	cache.Store("result", "original")

	// Nested evaluations copy configuration but retain the invocation's values.
	nested := *ac
	require.Same(t, cache, CacheFor(&nested, info))

	// A replacement resolver begins a new invocation without modifying the old one.
	authdeferred.ConfigureAuth(&nested, "")
	fresh := CacheFor(&nested, info)
	require.NotSame(t, cache, fresh)
	_, found := fresh.Load("result")
	require.False(t, found)
	require.Same(t, cache, CacheFor(ac, info))
	value, found := cache.Load("result")
	require.True(t, found)
	require.Equal(t, "original", value)
}
