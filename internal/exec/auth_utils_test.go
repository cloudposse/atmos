package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// newTestAtmosConfigWithAuth creates a test AtmosConfiguration with a global identity.
func newTestAtmosConfigWithAuth() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"global-identity": {
					Default: true,
				},
			},
		},
	}
}

// assertGlobalIdentity asserts that authConfig contains only the global-identity.
func assertGlobalIdentity(t *testing.T, authConfig *schema.AuthConfig) {
	assert.Len(t, authConfig.Identities, 1)
	assert.Contains(t, authConfig.Identities, "global-identity")
}

// TestGetComponentAuthConfig_GlobalOnly tests that global auth config is returned when no component config exists.
func TestGetComponentAuthConfig_GlobalOnly(t *testing.T) {
	atmosConfig := newTestAtmosConfigWithAuth()

	// Get auth config for non-existent component.
	authConfig, err := GetComponentAuthConfig(atmosConfig, "test-stack", "non-existent-component")
	require.NoError(t, err)
	require.NotNil(t, authConfig)

	// Should return global identity only.
	assertGlobalIdentity(t, authConfig)
	assert.True(t, authConfig.Identities["global-identity"].Default)
}

// TestGetComponentAuthConfig_EmptyStack tests handling of empty stack parameter.
func TestGetComponentAuthConfig_EmptyStack(t *testing.T) {
	atmosConfig := newTestAtmosConfigWithAuth()

	// Get auth config with empty stack.
	authConfig, err := GetComponentAuthConfig(atmosConfig, "", "component")
	require.NoError(t, err)
	require.NotNil(t, authConfig)

	// Should return global config.
	assertGlobalIdentity(t, authConfig)
}

// TestGetComponentAuthConfig_EmptyComponent tests handling of empty component parameter.
func TestGetComponentAuthConfig_EmptyComponent(t *testing.T) {
	atmosConfig := newTestAtmosConfigWithAuth()

	// Get auth config with an empty component.
	authConfig, err := GetComponentAuthConfig(atmosConfig, "test-stack", "")
	require.NoError(t, err)
	require.NotNil(t, authConfig)

	// Should return global config.
	assertGlobalIdentity(t, authConfig)
}

// TestGetComponentAuthConfig_NoGlobalAuth tests that empty auth config is returned when no global auth exists.
func TestGetComponentAuthConfig_NoGlobalAuth(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// No global auth config set.
	authConfig, err := GetComponentAuthConfig(atmosConfig, "test-stack", "component")
	require.NoError(t, err)
	require.NotNil(t, authConfig)

	// Should return empty auth config.
	assert.Nil(t, authConfig.Identities)
}
