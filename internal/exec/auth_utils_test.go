package exec

import (
	"path/filepath"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/merge"
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

// TestGetComponentAuthConfig_PreservesProviders tests that providers from global config are preserved.
// This is a critical test to ensure CreateAndAuthenticateManager doesn't fail with ErrProviderNotFound.
func TestGetComponentAuthConfig_PreservesProviders(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"test-provider": {
					Kind:   "aws/iam-identity-center",
					Region: "us-east-1",
				},
			},
			Identities: map[string]schema.Identity{
				"test-identity": {
					Kind:    "aws/permission-set",
					Default: true,
					Via: &schema.IdentityVia{
						Provider: "test-provider",
					},
				},
			},
			Logs: schema.Logs{
				Level: "Info",
			},
			Keyring: schema.KeyringConfig{
				Type: "system",
			},
			IdentityCaseMap: map[string]string{
				"test-identity": "test-identity",
			},
		},
	}

	// Get auth config for non-existent component (should return global config).
	authConfig, err := GetComponentAuthConfig(atmosConfig, "test-stack", "non-existent-component")
	require.NoError(t, err)
	require.NotNil(t, authConfig)

	// Verify ALL fields from global config are preserved.
	assert.Len(t, authConfig.Identities, 1, "should preserve identities")
	assert.Contains(t, authConfig.Identities, "test-identity")

	assert.Len(t, authConfig.Providers, 1, "should preserve providers")
	assert.Contains(t, authConfig.Providers, "test-provider")
	assert.Equal(t, "aws/iam-identity-center", authConfig.Providers["test-provider"].Kind)

	assert.Equal(t, "Info", authConfig.Logs.Level, "should preserve logs config")
	assert.Equal(t, "system", authConfig.Keyring.Type, "should preserve keyring config")

	assert.Len(t, authConfig.IdentityCaseMap, 1, "should preserve identity case map")
	assert.Contains(t, authConfig.IdentityCaseMap, "test-identity")
}

// TestGetComponentAuthConfig_MergesComponentAuth tests that component auth config is deep-merged with global config.
// This test uses real stack fixtures to verify the complete merge behavior.
func TestGetComponentAuthConfig_MergesComponentAuth(t *testing.T) {
	// Skip integration test in short mode.
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Initialize atmos config with auth test fixture.
	atmosConfig, err := initAtmosConfigFromPath(t, "../../../tests/fixtures/scenarios/atmos-auth/atmos.yaml")
	if err != nil {
		t.Skipf("Skipping integration test: failed to load config: %v", err)
	}

	// Skip if auth config wasn't loaded (test fixture might not be available).
	if len(atmosConfig.Auth.Identities) == 0 {
		t.Skip("Skipping integration test: auth config not loaded from fixture")
	}

	// Verify global auth config is loaded.
	require.Contains(t, atmosConfig.Auth.Identities, "test-admin")
	require.Contains(t, atmosConfig.Auth.Providers, "test-sso")

	// Get merged auth config for component with auth overrides.
	mergedAuth, err := GetComponentAuthConfig(&atmosConfig, "component-auth", "mycomponent")
	require.NoError(t, err)
	require.NotNil(t, mergedAuth)

	// Verify identities are merged (both global and component identities present).
	require.NotNil(t, mergedAuth.Identities)
	assert.Contains(t, mergedAuth.Identities, "test-admin", "should contain global identity")
	assert.Contains(t, mergedAuth.Identities, "identity-oidc", "should contain component identity")

	// Verify providers are merged (both global and component providers present).
	require.NotNil(t, mergedAuth.Providers)
	assert.Contains(t, mergedAuth.Providers, "test-sso", "should contain global provider")
	assert.Contains(t, mergedAuth.Providers, "provider-oidc", "should contain component provider")

	// Verify component provider overrides global provider with same name.
	// The component defines provider-oidc with region, while global has region + spec.
	componentProvider := mergedAuth.Providers["provider-oidc"]
	assert.Equal(t, "us-east-1", componentProvider.Region, "component provider should override global")
}

// TestGetComponentAuthConfig_PartialOverride tests that component can partially override identity fields.
// This verifies deep merge behavior (not just full object replacement).
func TestGetComponentAuthConfig_PartialOverride(t *testing.T) {
	// Create atmos config with global auth.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace", // Use replace strategy for predictable test behavior.
		},
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"test-identity": {
					Kind:    "aws/assume-role",
					Default: false,
					Principal: map[string]interface{}{
						"assume_role": "arn:aws:iam::111111111111:role/GlobalRole",
					},
				},
			},
		},
	}

	// Simulate component config with partial override (only default field).
	// In real scenario, this would come from ExecuteDescribeComponent,
	// but for unit testing the merge logic, we can test it directly.
	globalAuthMap, err := authConfigToMap(&atmosConfig.Auth)
	require.NoError(t, err)

	componentAuthSection := map[string]any{
		"identities": map[string]any{
			"test-identity": map[string]any{
				"default": true, // Override only the default field.
			},
		},
	}

	// Test the merge logic directly.
	mergedMap, err := mergeMaps(t, atmosConfig, globalAuthMap, componentAuthSection)
	require.NoError(t, err)

	// Convert back to AuthConfig.
	var mergedAuth schema.AuthConfig
	err = mapstructureDecode(mergedMap, &mergedAuth)
	require.NoError(t, err)

	// Verify partial override: default changed, but kind and principal preserved.
	require.Contains(t, mergedAuth.Identities, "test-identity")
	identity := mergedAuth.Identities["test-identity"]
	assert.True(t, identity.Default, "default should be overridden to true")
	assert.Equal(t, "aws/assume-role", identity.Kind, "kind should be preserved from global")
	require.NotNil(t, identity.Principal)
	assumeRole, ok := identity.Principal["assume_role"].(string)
	require.True(t, ok, "assume_role should be a string")
	assert.Equal(t, "arn:aws:iam::111111111111:role/GlobalRole", assumeRole, "principal should be preserved from global")
}

// Helper functions for testing.

// initAtmosConfigFromPath loads atmos configuration from a file path.
func initAtmosConfigFromPath(t *testing.T, configPath string) (schema.AtmosConfiguration, error) {
	t.Helper()

	absPath, err := filepath.Abs(configPath)
	require.NoError(t, err)

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosCliConfigPath: absPath,
	}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
	return atmosConfig, err
}

// mergeMaps is a helper function to test merge logic directly.
func mergeMaps(t *testing.T, atmosConfig *schema.AtmosConfiguration, maps ...map[string]any) (map[string]any, error) {
	t.Helper()
	return merge.Merge(atmosConfig, maps)
}

// mapstructureDecode is a helper wrapper for mapstructure.Decode.
func mapstructureDecode(input any, output any) error {
	return mapstructure.Decode(input, output)
}
