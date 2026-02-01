package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestHandleMergeError tests all branches of the handleMergeError fallback function.
func TestHandleMergeError(t *testing.T) {
	tests := []struct {
		name                 string
		componentAuthSection map[string]any
		globalAuthSection    map[string]any
		expectedResult       map[string]any
		expectAuthSet        bool
	}{
		{
			name: "component auth non-empty - returns component auth",
			componentAuthSection: map[string]any{
				"providers": map[string]any{
					"aws-sso": map[string]any{"kind": "aws"},
				},
			},
			globalAuthSection: map[string]any{
				"identities": map[string]any{
					"dev": map[string]any{"kind": "aws/assume-role"},
				},
			},
			expectedResult: map[string]any{
				"providers": map[string]any{
					"aws-sso": map[string]any{"kind": "aws"},
				},
			},
			expectAuthSet: true,
		},
		{
			name:                 "component auth empty - returns global auth",
			componentAuthSection: map[string]any{},
			globalAuthSection: map[string]any{
				"identities": map[string]any{
					"dev": map[string]any{"kind": "aws/assume-role"},
				},
			},
			expectedResult: map[string]any{
				"identities": map[string]any{
					"dev": map[string]any{"kind": "aws/assume-role"},
				},
			},
			expectAuthSet: true,
		},
		{
			name:                 "both empty - returns empty map",
			componentAuthSection: map[string]any{},
			globalAuthSection:    map[string]any{},
			expectedResult:       map[string]any{},
			expectAuthSet:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentSection := map[string]any{}

			result := handleMergeError(componentSection, tt.globalAuthSection, tt.componentAuthSection)

			assert.Equal(t, tt.expectedResult, result)

			if tt.expectAuthSet {
				assert.Equal(t, tt.expectedResult, componentSection[cfg.AuthSectionName],
					"componentSection[auth] should be updated with the fallback result")
			} else {
				_, exists := componentSection[cfg.AuthSectionName]
				assert.False(t, exists, "componentSection[auth] should not be set when both are empty")
			}
		})
	}
}

// TestBuildGlobalAuthSection tests building the global auth section from AtmosConfiguration.
func TestBuildGlobalAuthSection(t *testing.T) {
	tests := []struct {
		name           string
		atmosConfig    *schema.AtmosConfiguration
		expectedKeys   []string
		unexpectedKeys []string
	}{
		{
			name:           "empty auth config",
			atmosConfig:    &schema.AtmosConfiguration{},
			expectedKeys:   nil,
			unexpectedKeys: []string{"providers", "identities", "logs", "keyring"},
		},
		{
			name: "providers only",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers: map[string]schema.Provider{
						"aws-sso": {Kind: "aws/iam-identity-center", Region: "us-east-1"},
					},
				},
			},
			expectedKeys:   []string{"providers"},
			unexpectedKeys: []string{"identities", "logs", "keyring"},
		},
		{
			name: "identities only",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Identities: map[string]schema.Identity{
						"dev": {Kind: "aws/assume-role"},
					},
				},
			},
			expectedKeys:   []string{"identities"},
			unexpectedKeys: []string{"providers", "logs", "keyring"},
		},
		{
			name: "logs with level only",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Logs: schema.Logs{Level: "Debug"},
				},
			},
			expectedKeys:   []string{"logs"},
			unexpectedKeys: []string{"providers", "identities", "keyring"},
		},
		{
			name: "logs with file only",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Logs: schema.Logs{File: "/tmp/auth.log"},
				},
			},
			expectedKeys:   []string{"logs"},
			unexpectedKeys: []string{"providers", "identities", "keyring"},
		},
		{
			name: "keyring only",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Keyring: schema.KeyringConfig{
						Type: "system",
					},
				},
			},
			expectedKeys:   []string{"keyring"},
			unexpectedKeys: []string{"providers", "identities", "logs"},
		},
		{
			name: "all sections populated",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers: map[string]schema.Provider{
						"sso": {Kind: "aws"},
					},
					Identities: map[string]schema.Identity{
						"dev": {Kind: "aws/assume-role"},
					},
					Logs: schema.Logs{
						Level: "Info",
						File:  "/var/log/auth.log",
					},
					Keyring: schema.KeyringConfig{
						Type: "file",
					},
				},
			},
			expectedKeys: []string{"providers", "identities", "logs", "keyring"},
		},
		{
			name: "empty providers map not included",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers:  map[string]schema.Provider{},
					Identities: map[string]schema.Identity{},
				},
			},
			unexpectedKeys: []string{"providers", "identities", "logs", "keyring"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildGlobalAuthSection(tt.atmosConfig)

			for _, key := range tt.expectedKeys {
				assert.Contains(t, result, key, "expected key %q in result", key)
			}
			for _, key := range tt.unexpectedKeys {
				assert.NotContains(t, result, key, "unexpected key %q in result", key)
			}
		})
	}
}

// TestBuildGlobalAuthSection_LogsContent verifies the logs map structure.
func TestBuildGlobalAuthSection_LogsContent(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Logs: schema.Logs{
				Level: "Debug",
				File:  "/tmp/auth.log",
			},
		},
	}

	result := buildGlobalAuthSection(atmosConfig)

	logs, ok := result["logs"].(map[string]any)
	require.True(t, ok, "logs should be map[string]any")
	assert.Equal(t, "Debug", logs["level"])
	assert.Equal(t, "/tmp/auth.log", logs["file"])
}

// TestBuildGlobalAuthSection_KeyringContent verifies keyring is stored as the full struct.
func TestBuildGlobalAuthSection_KeyringContent(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Keyring: schema.KeyringConfig{
				Type: "system",
				Spec: map[string]any{
					"service": "atmos",
				},
			},
		},
	}

	result := buildGlobalAuthSection(atmosConfig)

	keyring, ok := result["keyring"].(schema.KeyringConfig)
	require.True(t, ok, "keyring should be schema.KeyringConfig")
	assert.Equal(t, "system", keyring.Type)
	assert.Equal(t, "atmos", keyring.Spec["service"])
}

// TestGetComponentAuthSection tests extracting the auth section from component config.
func TestGetComponentAuthSection(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		expectedEmpty    bool
	}{
		{
			name:             "no auth section",
			componentSection: map[string]any{},
			expectedEmpty:    true,
		},
		{
			name: "auth section is nil",
			componentSection: map[string]any{
				cfg.AuthSectionName: nil,
			},
			expectedEmpty: true,
		},
		{
			name: "auth section is wrong type (string)",
			componentSection: map[string]any{
				cfg.AuthSectionName: "invalid",
			},
			expectedEmpty: true,
		},
		{
			name: "auth section is wrong type (int)",
			componentSection: map[string]any{
				cfg.AuthSectionName: 42,
			},
			expectedEmpty: true,
		},
		{
			name: "auth section is wrong type (slice)",
			componentSection: map[string]any{
				cfg.AuthSectionName: []string{"foo"},
			},
			expectedEmpty: true,
		},
		{
			name: "valid auth section",
			componentSection: map[string]any{
				cfg.AuthSectionName: map[string]any{
					"providers": map[string]any{
						"test": map[string]any{"kind": "aws"},
					},
				},
			},
			expectedEmpty: false,
		},
		{
			name: "empty but valid auth section",
			componentSection: map[string]any{
				cfg.AuthSectionName: map[string]any{},
			},
			expectedEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getComponentAuthSection(tt.componentSection)
			assert.NotNil(t, result, "should never return nil")

			if tt.expectedEmpty {
				assert.Empty(t, result)
			} else {
				assert.NotEmpty(t, result)
			}
		})
	}
}

// TestCreateAndAuthenticateAuthManager_NoStackOrComponent tests the code path
// where stack and/or component are empty, which skips ExecuteDescribeComponent.
func TestCreateAndAuthenticateAuthManager_NoStackOrComponent(t *testing.T) {
	tests := []struct {
		name      string
		stack     string
		component string
		identity  string
	}{
		{
			name:      "both empty",
			stack:     "",
			component: "",
			identity:  "",
		},
		{
			name:      "empty stack",
			stack:     "",
			component: "vpc",
			identity:  "",
		},
		{
			name:      "empty component",
			stack:     "dev-us-west-2",
			component: "",
			identity:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}
			info := &schema.ConfigAndStacksInfo{
				Stack:            tt.stack,
				ComponentFromArg: tt.component,
				Identity:         tt.identity,
			}

			// With no auth configured and no identity, should return nil, nil
			// (no authentication needed).
			authManager, err := createAndAuthenticateAuthManager(atmosConfig, info)
			assert.NoError(t, err)
			assert.Nil(t, authManager, "should return nil when no auth is configured")
		})
	}
}

// TestCreateAndAuthenticateAuthManager_DisabledAuth tests that passing the
// disabled identity value skips authentication entirely.
func TestCreateAndAuthenticateAuthManager_DisabledAuth(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"dev": {Kind: "aws/assume-role", Default: true},
			},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "",
		ComponentFromArg: "",
		Identity:         cfg.IdentityFlagDisabledValue,
	}

	authManager, err := createAndAuthenticateAuthManager(atmosConfig, info)
	assert.NoError(t, err)
	assert.Nil(t, authManager, "should return nil when identity is disabled")
}

// TestCreateAndAuthenticateAuthManager_NoAuthConfigured tests that when no auth
// identities or providers are configured, the function returns nil without error.
func TestCreateAndAuthenticateAuthManager_NoAuthConfigured(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers:  map[string]schema.Provider{},
			Identities: map[string]schema.Identity{},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "",
		ComponentFromArg: "",
		Identity:         "",
	}

	authManager, err := createAndAuthenticateAuthManager(atmosConfig, info)
	assert.NoError(t, err)
	assert.Nil(t, authManager, "should return nil when no auth configured and no identity specified")
	assert.Empty(t, info.Identity, "identity should remain empty when no auth")
}

// TestMergeGlobalAuthConfig_ComponentSectionUpdated verifies that componentSection["auth"]
// is updated after a successful merge.
func TestMergeGlobalAuthConfig_ComponentSectionUpdated(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"global-provider": {Kind: "aws", Region: "us-east-1"},
			},
		},
	}

	componentSection := map[string]any{
		cfg.AuthSectionName: map[string]any{
			"providers": map[string]any{
				"component-provider": map[string]any{"kind": "azure"},
			},
		},
	}

	result := mergeGlobalAuthConfig(atmosConfig, componentSection)

	// Verify componentSection["auth"] was updated to match the result.
	assert.Equal(t, result, componentSection[cfg.AuthSectionName],
		"componentSection[auth] should be updated with merged result")
}

// TestMergeGlobalAuthConfig_BothEmpty verifies that empty configs return empty map.
func TestMergeGlobalAuthConfig_BothEmpty(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	componentSection := map[string]any{}

	result := mergeGlobalAuthConfig(atmosConfig, componentSection)
	assert.Empty(t, result, "both empty should return empty map")
}

// TestHandleMergeError_ComponentSectionSideEffect verifies that handleMergeError
// updates componentSection as a side effect.
func TestHandleMergeError_ComponentSectionSideEffect(t *testing.T) {
	t.Run("sets component auth in componentSection", func(t *testing.T) {
		componentSection := map[string]any{"vars": map[string]any{"name": "test"}}
		componentAuth := map[string]any{"providers": map[string]any{"test": "value"}}

		handleMergeError(componentSection, map[string]any{}, componentAuth)

		assert.Equal(t, componentAuth, componentSection[cfg.AuthSectionName],
			"should set componentSection[auth] to componentAuth")
	})

	t.Run("sets global auth when no component auth", func(t *testing.T) {
		componentSection := map[string]any{"vars": map[string]any{"name": "test"}}
		globalAuth := map[string]any{"identities": map[string]any{"dev": "value"}}

		handleMergeError(componentSection, globalAuth, map[string]any{})

		assert.Equal(t, globalAuth, componentSection[cfg.AuthSectionName],
			"should set componentSection[auth] to globalAuth when componentAuth is empty")
	})

	t.Run("does not set auth when both empty", func(t *testing.T) {
		componentSection := map[string]any{"vars": map[string]any{"name": "test"}}

		handleMergeError(componentSection, map[string]any{}, map[string]any{})

		_, exists := componentSection[cfg.AuthSectionName]
		assert.False(t, exists, "should not set componentSection[auth] when both are empty")
	})
}

// TestStoreAutoDetectedIdentity tests the storeAutoDetectedIdentity function that stores
// the authenticated identity from the auth chain back into info.Identity when it was
// empty (auto-detected).
func TestStoreAutoDetectedIdentity(t *testing.T) {
	tests := []struct {
		name             string
		chain            []string
		initialIdentity  string
		expectedIdentity string
		nilAuthManager   bool
	}{
		{
			name:             "nil AuthManager - identity unchanged",
			nilAuthManager:   true,
			initialIdentity:  "",
			expectedIdentity: "",
		},
		{
			name:             "identity already set - no override",
			chain:            []string{"provider", "target-identity"},
			initialIdentity:  "explicit-identity",
			expectedIdentity: "explicit-identity",
		},
		{
			name:             "empty chain - identity unchanged",
			chain:            []string{},
			initialIdentity:  "",
			expectedIdentity: "",
		},
		{
			name:             "single element chain - stores identity",
			chain:            []string{"single-identity"},
			initialIdentity:  "",
			expectedIdentity: "single-identity",
		},
		{
			name:             "multi-element chain - stores last element",
			chain:            []string{"provider", "intermediate", "target-identity"},
			initialIdentity:  "",
			expectedIdentity: "target-identity",
		},
		{
			name:             "two-element chain - stores last element",
			chain:            []string{"provider", "dev-role"},
			initialIdentity:  "",
			expectedIdentity: "dev-role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{
				Identity: tt.initialIdentity,
			}

			if tt.nilAuthManager {
				storeAutoDetectedIdentity(nil, info)
				assert.Equal(t, tt.expectedIdentity, info.Identity)
				return
			}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockAuthMgr := types.NewMockAuthManager(ctrl)
			mockAuthMgr.EXPECT().GetChain().Return(tt.chain).AnyTimes()

			storeAutoDetectedIdentity(mockAuthMgr, info)
			assert.Equal(t, tt.expectedIdentity, info.Identity)
		})
	}
}

// TestGetMergedAuthConfig_EmptyStackOrComponent tests that getMergedAuthConfig
// returns global auth config when stack or component is empty.
func TestGetMergedAuthConfig_EmptyStackOrComponent(t *testing.T) {
	tests := []struct {
		name      string
		stack     string
		component string
	}{
		{
			name:      "empty stack",
			stack:     "",
			component: "vpc",
		},
		{
			name:      "empty component",
			stack:     "dev-us-west-2",
			component: "",
		},
		{
			name:      "both empty",
			stack:     "",
			component: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Identities: map[string]schema.Identity{
						"test-role": {Kind: "aws"},
					},
				},
			}

			info := &schema.ConfigAndStacksInfo{
				Stack:            tt.stack,
				ComponentFromArg: tt.component,
			}

			result, err := getMergedAuthConfig(atmosConfig, info)
			assert.NoError(t, err)
			assert.NotNil(t, result, "should return global auth config")
		})
	}
}

// TestMergeGlobalAuthConfig_GlobalOnlyProviders verifies merge when only global has providers.
func TestMergeGlobalAuthConfig_GlobalOnlyProviders(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"aws-sso": {Kind: "aws/iam-identity-center", Region: "us-east-1"},
			},
			Identities: map[string]schema.Identity{
				"dev": {Kind: "aws/assume-role"},
			},
		},
	}

	componentSection := map[string]any{}

	result := mergeGlobalAuthConfig(atmosConfig, componentSection)

	assert.NotEmpty(t, result)
	// Verify providers were included from global config.
	providers, ok := result["providers"].(map[string]interface{})
	require.True(t, ok, "providers should be map[string]interface{} after merge")
	assert.Contains(t, providers, "aws-sso")

	// Verify identities were included from global config.
	identities, ok := result["identities"].(map[string]interface{})
	require.True(t, ok, "identities should be map[string]interface{} after merge")
	assert.Contains(t, identities, "dev")
}

// TestMergeGlobalAuthConfig_ComponentOverridesGlobal verifies that component auth
// overrides global auth during merge.
func TestMergeGlobalAuthConfig_ComponentOverridesGlobal(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"shared": {Kind: "aws", Region: "us-east-1"},
			},
		},
	}

	componentSection := map[string]any{
		cfg.AuthSectionName: map[string]any{
			"providers": map[string]any{
				"shared": map[string]any{
					"kind":   "aws",
					"region": "eu-west-1",
				},
			},
		},
	}

	result := mergeGlobalAuthConfig(atmosConfig, componentSection)

	providers, ok := result["providers"].(map[string]interface{})
	require.True(t, ok)

	shared, ok := providers["shared"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "eu-west-1", shared["region"], "component should override global region")
}
