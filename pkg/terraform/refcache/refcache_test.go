package refcache

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile guards for the configuration fields that scope a key.
var _ = schema.AtmosConfiguration{BasePath: "", CliConfigPath: "", AuthManager: nil}

func baseKey() Key {
	return NewKey(&schema.AtmosConfiguration{BasePath: "base", CliConfigPath: "config"}, "stack", "component")
}

func TestNewKey(t *testing.T) {
	key := baseKey()
	assert.Equal(t, Key{BasePath: "base", ConfigPath: "config", Stack: "stack", Component: "component"}, key)
	assert.Equal(t, Key{Stack: "stack", Component: "component"}, NewKey(nil, "stack", "component"), "a nil configuration is allowed")
}

// Every field changes the cached result, so changing any one must change the key.
func TestKeyFieldsChangeEquality(t *testing.T) {
	ctrl := gomock.NewController(t)
	manager := authtypes.NewMockAuthManager(ctrl)
	tests := []struct {
		name   string
		mutate func(*Key)
	}{
		{"base path", func(k *Key) { k.BasePath = "other" }},
		{"config path", func(k *Key) { k.ConfigPath = "other" }},
		{"stack", func(k *Key) { k.Stack = "other" }},
		{"component", func(k *Key) { k.Component = "other" }},
		{"invocation", func(k *Key) { k.Invocation = manager }},
		{"auth", func(k *Key) { k.Auth = AuthScope(nil, nil, true) }},
		{"inspection", func(k *Key) { k.Inspection = true }},
		{"raw describe", func(k *Key) { k.RawDescribe = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := baseKey()
			tt.mutate(&changed)
			assert.NotEqual(t, baseKey(), changed)

			var cache sync.Map
			cache.Store(baseKey(), "base")
			_, found := cache.Load(changed)
			assert.False(t, found, "a changed key must miss")
			value, found := cache.Load(baseKey())
			assert.True(t, found, "an unchanged key must hit")
			assert.Equal(t, "base", value)
		})
	}
}

func TestNewKeyUncomparableInvocationNeverShared(t *testing.T) {
	config := &schema.AtmosConfiguration{AuthManager: []string{"opaque"}}
	first := NewKey(config, "stack", "component")
	second := NewKey(config, "stack", "component")
	require.NotEqual(t, first, second)

	var cache sync.Map
	cache.Store(first, "value") // Must not panic: the key stays comparable.
	_, found := cache.Load(second)
	assert.False(t, found)
}

func TestAuthScopeNoCredentials(t *testing.T) {
	assert.Nil(t, AuthScope(nil, nil, false))
	assert.NotNil(t, AuthScope(nil, nil, true), "disabled authentication is its own scope")
}

func TestAuthScopeContextIsolation(t *testing.T) {
	first := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "same-name", EndpointURL: "http://first"}}
	copyOfFirst := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "same-name", EndpointURL: "http://first"}}
	second := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "same-name", EndpointURL: "http://second"}}
	scope := AuthScope(first, nil, false)
	require.Equal(t, scope, AuthScope(copyOfFirst, nil, false), "equal content shares a scope")
	require.NotEqual(t, scope, AuthScope(second, nil, false))
	require.NotEqual(t, scope, AuthScope(nil, nil, false))
	require.NotEqual(t, scope, AuthScope(first, nil, true), "disabled differs from enabled")
}

func TestAuthScopeManagerIsolation(t *testing.T) {
	ctrl := gomock.NewController(t)
	first := authtypes.NewMockAuthManager(ctrl)
	second := authtypes.NewMockAuthManager(ctrl)
	info := &schema.ConfigAndStacksInfo{Identity: "same-name"}
	first.EXPECT().GetStackInfo().Return(info).AnyTimes()
	second.EXPECT().GetStackInfo().Return(info).AnyTimes()
	scope := AuthScope(nil, first, false)
	require.Equal(t, scope, AuthScope(nil, first, false))
	require.False(t, scope == AuthScope(nil, second, false), "scopes compare manager identity, not deep structural equality")
	info.Identity = "another-name"
	require.NotEqual(t, scope, AuthScope(nil, first, false))
	info.Identity = "same-name"
	info.AuthDisabled = true
	require.NotEqual(t, scope, AuthScope(nil, first, false))
	info.AuthDisabled = false
	info.AuthContext = &schema.AuthContext{Azure: &schema.AzureAuthContext{SubscriptionID: "other-account"}}
	require.NotEqual(t, scope, AuthScope(nil, first, false))
}

func TestAuthScopeUnencodableContextDoesNotCollide(t *testing.T) {
	context := &schema.AuthContext{GCP: &schema.GCPAuthContext{TokenExpiry: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}
	first := AuthScope(context, nil, false)
	require.NotEqual(t, first, AuthScope(context, nil, false))
	require.NotEqual(t, first, AuthScope(nil, nil, false))
}

func TestAuthScopeUncomparableManagerDoesNotCollide(t *testing.T) {
	// Callers historically accept opaque manager values. Uncomparable ones must bypass
	// reuse instead of panicking when used as a sync.Map key.
	uncomparable := []string{"opaque"}
	first := AuthScope(nil, uncomparable, false)
	require.NotEqual(t, first, AuthScope(nil, uncomparable, false))

	var cache sync.Map
	cache.Store(Key{Auth: first}, "value")
}

func TestDeleteTarget(t *testing.T) {
	var cache sync.Map
	var targets []Key
	for _, auth := range []any{nil, AuthScope(nil, nil, true)} {
		for _, inspection := range []bool{false, true} {
			for _, raw := range []bool{false, true} {
				for _, base := range []string{"head", "base"} {
					key := NewKey(&schema.AtmosConfiguration{BasePath: base}, "dev", "target")
					key.Auth, key.Inspection, key.RawDescribe = auth, inspection, raw
					targets = append(targets, key)
					cache.Store(key, "target")
				}
			}
		}
	}
	require.Len(t, targets, 16)
	others := []Key{
		NewKey(nil, "other", "target"),
		NewKey(nil, "dev", "other"),
		NewKey(nil, "target", "dev"),
	}
	for _, key := range others {
		cache.Store(key, "other")
	}
	cache.Store("not-a-key", "other")

	DeleteTarget(&cache, "dev", "target")

	for _, key := range targets {
		_, found := cache.Load(key)
		assert.False(t, found, "every variant of the target is removed: %+v", key)
	}
	for _, key := range others {
		_, found := cache.Load(key)
		assert.True(t, found, "unrelated target is retained: %+v", key)
	}
	_, found := cache.Load("not-a-key")
	assert.True(t, found, "entries of another key shape are retained")
}

func TestKindsFor(t *testing.T) {
	assert.Equal(t, Kinds{Component: "atmos.Component", State: "terraform.state", StaticState: "terraform.state.static"}, KindsFor(false))
	assert.Equal(t, Kinds{Component: "inspect:atmos.Component", State: "inspect:terraform.state", StaticState: "inspect:terraform.state.static"}, KindsFor(true))

	execution, inspection := KindsFor(false), KindsFor(true)
	for _, kind := range []string{execution.Component, execution.State, execution.StaticState} {
		assert.NotContains(t, []string{inspection.Component, inspection.State, inspection.StaticState}, kind)
	}
	assert.NotEqual(t, execution.State, execution.StaticState)
	assert.NotEqual(t, execution.Component, execution.State)
}
