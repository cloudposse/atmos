package deferred

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValueCacheIsolation(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	ConfigureAuth(ac, "")
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
	ConfigureAuth(ac, "")
	require.NotSame(t, second, CacheFor(ac, info), "another invocation must resolve fresh values")
}

func TestValueCacheRejectsComputedAndUnsupportedValues(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	ConfigureAuth(ac, "")
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
	ConfigureAuth(ac, "explicit")
	require.Nil(t, CacheFor(ac, info))
}

func TestResolveAuthClearsStaleContext(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "no manager", true: "disabled"}[disabled], func(t *testing.T) {
			ctrl := gomock.NewController(t)
			factory := NewMockAuthFactory(ctrl)
			ac := &schema.AtmosConfiguration{DeferredAuth: NewAuthResolver(AuthOptions{Factory: factory, Disabled: disabled})}
			info := &schema.ConfigAndStacksInfo{AuthManager: types.NewMockAuthManager(ctrl), AuthContext: &schema.AuthContext{}}
			if !disabled {
				factory.EXPECT().Create(ac, gomock.Any(), "").Return(nil, nil)
			}
			require.NoError(t, ResolveAuth(ac, info))
			require.Nil(t, info.AuthManager)
			require.Nil(t, info.AuthContext)
		})
	}
}
