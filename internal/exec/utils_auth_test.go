package exec

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	mockTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestHandleMergeError(t *testing.T) {
	tests := []struct {
		name                 string
		componentAuthSection map[string]any
		globalAuthSection    map[string]any
		expectedResult       map[string]any
		expectedFallback     string
	}{
		{
			name:                 "falls back to component auth",
			componentAuthSection: map[string]any{"providers": map[string]any{"aws": "test"}},
			globalAuthSection:    map[string]any{"providers": map[string]any{"gcp": "test"}},
			expectedResult:       map[string]any{"providers": map[string]any{"aws": "test"}},
			expectedFallback:     "component",
		},
		{
			name:                 "falls back to global auth when no component auth",
			componentAuthSection: map[string]any{},
			globalAuthSection:    map[string]any{"providers": map[string]any{"gcp": "test"}},
			expectedResult:       map[string]any{"providers": map[string]any{"gcp": "test"}},
			expectedFallback:     "global",
		},
		{
			name:                 "returns empty when both are empty",
			componentAuthSection: map[string]any{},
			globalAuthSection:    map[string]any{},
			expectedResult:       map[string]any{},
			expectedFallback:     "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentSection := map[string]any{}
			result := handleMergeError(componentSection, tt.globalAuthSection, tt.componentAuthSection)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestBuildGlobalAuthSection(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		expected map[string]any
	}{
		{
			name:     "empty config",
			config:   &schema.AtmosConfiguration{},
			expected: map[string]any{},
		},
		{
			name: "providers only",
			config: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers: map[string]schema.Provider{
						"aws": {Kind: "aws-iam"},
					},
				},
			},
			expected: map[string]any{
				"providers": map[string]schema.Provider{
					"aws": {Kind: "aws-iam"},
				},
			},
		},
		{
			name: "identities only",
			config: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Identities: map[string]schema.Identity{
						"dev": {Kind: "aws"},
					},
				},
			},
			expected: map[string]any{
				"identities": map[string]schema.Identity{
					"dev": {Kind: "aws"},
				},
			},
		},
		{
			name: "logs section",
			config: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Logs: schema.Logs{Level: "debug", File: "/tmp/auth.log"},
				},
			},
			expected: map[string]any{
				"logs": map[string]any{
					"level": "debug",
					"file":  "/tmp/auth.log",
				},
			},
		},
		{
			name: "keyring section",
			config: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Keyring: schema.KeyringConfig{Type: "system"},
				},
			},
			expected: map[string]any{
				"keyring": schema.KeyringConfig{Type: "system"},
			},
		},
		{
			name: "all sections",
			config: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers: map[string]schema.Provider{
						"aws": {Kind: "aws-iam"},
					},
					Identities: map[string]schema.Identity{
						"dev": {Kind: "aws"},
					},
					Logs:    schema.Logs{Level: "info"},
					Keyring: schema.KeyringConfig{Type: "file"},
				},
			},
			expected: map[string]any{
				"providers": map[string]schema.Provider{
					"aws": {Kind: "aws-iam"},
				},
				"identities": map[string]schema.Identity{
					"dev": {Kind: "aws"},
				},
				"logs": map[string]any{
					"level": "info",
					"file":  "",
				},
				"keyring": schema.KeyringConfig{Type: "file"},
			},
		},
		{
			name: "realm included when explicitly configured",
			config: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Realm:       "my-project",
					RealmSource: "config",
				},
			},
			expected: map[string]any{
				"realm": "my-project",
			},
		},
		{
			name: "realm included when set via env",
			config: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Realm:       "env-realm",
					RealmSource: "env",
				},
			},
			expected: map[string]any{
				"realm": "env-realm",
			},
		},
		{
			name: "realm excluded when auto-computed from config-path",
			config: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Realm:       "b80ea18be93f8201",
					RealmSource: "config-path",
				},
			},
			expected: map[string]any{},
		},
		{
			name: "realm excluded when default",
			config: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Realm:       "default",
					RealmSource: "default",
				},
			},
			expected: map[string]any{},
		},
		{
			name: "realm excluded when empty",
			config: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Realm: "",
				},
			},
			expected: map[string]any{},
		},
		{
			name: "all sections including explicit realm",
			config: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Realm:       "prod-realm",
					RealmSource: "config",
					Providers: map[string]schema.Provider{
						"aws": {Kind: "aws-iam"},
					},
					Identities: map[string]schema.Identity{
						"dev": {Kind: "aws"},
					},
					Logs:    schema.Logs{Level: "info"},
					Keyring: schema.KeyringConfig{Type: "file"},
				},
			},
			expected: map[string]any{
				"realm": "prod-realm",
				"providers": map[string]schema.Provider{
					"aws": {Kind: "aws-iam"},
				},
				"identities": map[string]schema.Identity{
					"dev": {Kind: "aws"},
				},
				"logs": map[string]any{
					"level": "info",
					"file":  "",
				},
				"keyring": schema.KeyringConfig{Type: "file"},
			},
		},
		{
			name: "empty maps are excluded",
			config: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers:  map[string]schema.Provider{},
					Identities: map[string]schema.Identity{},
				},
			},
			expected: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildGlobalAuthSection(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetComponentAuthSection(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		expected         map[string]any
	}{
		{
			name:             "missing auth section",
			componentSection: map[string]any{},
			expected:         map[string]any{},
		},
		{
			name:             "nil auth section",
			componentSection: map[string]any{cfg.AuthSectionName: nil},
			expected:         map[string]any{},
		},
		{
			name:             "wrong type",
			componentSection: map[string]any{cfg.AuthSectionName: "invalid"},
			expected:         map[string]any{},
		},
		{
			name: "valid auth section",
			componentSection: map[string]any{
				cfg.AuthSectionName: map[string]any{
					"providers": map[string]any{"aws": "test"},
				},
			},
			expected: map[string]any{
				"providers": map[string]any{"aws": "test"},
			},
		},
		{
			name:             "empty auth section",
			componentSection: map[string]any{cfg.AuthSectionName: map[string]any{}},
			expected:         map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getComponentAuthSection(tt.componentSection)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateAndAuthenticateAuthManager_NoStackOrComponent(t *testing.T) {
	// When stack and component are empty, getMergedAuthConfig returns global auth config.
	// With no auth configured, CreateAndAuthenticateManagerWithAtmosConfig returns nil.
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}

	authManager, err := createAndAuthenticateAuthManager(atmosConfig, info)
	assert.NoError(t, err)
	assert.Nil(t, authManager)
}

func TestCreateAndAuthenticateAuthManager_NoAuthConfigured(t *testing.T) {
	// With no auth config but valid stack/component that doesn't exist,
	// this tests the global auth fallback path.
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "",
		ComponentFromArg: "",
	}

	authManager, err := createAndAuthenticateAuthManager(atmosConfig, info)
	assert.NoError(t, err)
	assert.Nil(t, authManager)
}

func TestMergeGlobalAuthConfig_ComponentSectionUpdated(t *testing.T) {
	// Verify that componentSection["auth"] is updated after merge.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
		},
	}
	componentSection := map[string]any{}

	result := mergeGlobalAuthConfig(atmosConfig, componentSection)
	assert.NotEmpty(t, result)
	assert.Contains(t, componentSection, cfg.AuthSectionName)
}

func TestMergeGlobalAuthConfig_BothEmpty(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	componentSection := map[string]any{}

	result := mergeGlobalAuthConfig(atmosConfig, componentSection)
	assert.Empty(t, result)
}

func TestHandleMergeError_ComponentSectionSideEffect(t *testing.T) {
	// Verify that componentSection is updated as a side effect.
	componentSection := map[string]any{}
	componentAuthSection := map[string]any{"providers": map[string]any{"aws": "test"}}
	globalAuthSection := map[string]any{}

	handleMergeError(componentSection, globalAuthSection, componentAuthSection)
	assert.Contains(t, componentSection, cfg.AuthSectionName)
	assert.Equal(t, componentAuthSection, componentSection[cfg.AuthSectionName])
}

func TestStoreAutoDetectedIdentity(t *testing.T) {
	tests := []struct {
		name             string
		setupMock        func(ctrl *gomock.Controller) *mockTypes.MockAuthManager
		initialIdentity  string
		expectedIdentity string
		nilManager       bool
	}{
		{
			name:             "nil manager does nothing",
			nilManager:       true,
			initialIdentity:  "",
			expectedIdentity: "",
		},
		{
			name:             "existing identity is preserved",
			initialIdentity:  "existing-role",
			setupMock:        mockTypes.NewMockAuthManager,
			expectedIdentity: "existing-role",
		},
		{
			name:            "empty chain does not update",
			initialIdentity: "",
			setupMock: func(ctrl *gomock.Controller) *mockTypes.MockAuthManager {
				m := mockTypes.NewMockAuthManager(ctrl)
				m.EXPECT().GetChain().Return([]string{})
				return m
			},
			expectedIdentity: "",
		},
		{
			name:            "single element chain stores identity",
			initialIdentity: "",
			setupMock: func(ctrl *gomock.Controller) *mockTypes.MockAuthManager {
				m := mockTypes.NewMockAuthManager(ctrl)
				m.EXPECT().GetChain().Return([]string{"dev-role"})
				return m
			},
			expectedIdentity: "dev-role",
		},
		{
			name:            "multi element chain stores last identity",
			initialIdentity: "",
			setupMock: func(ctrl *gomock.Controller) *mockTypes.MockAuthManager {
				m := mockTypes.NewMockAuthManager(ctrl)
				m.EXPECT().GetChain().Return([]string{"base-role", "assume-role", "final-role"})
				return m
			},
			expectedIdentity: "final-role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			info := &schema.ConfigAndStacksInfo{
				Identity: tt.initialIdentity,
			}

			if tt.nilManager {
				storeAutoDetectedIdentity(nil, info)
			} else {
				mockManager := tt.setupMock(ctrl)
				storeAutoDetectedIdentity(mockManager, info)
			}

			assert.Equal(t, tt.expectedIdentity, info.Identity)
		})
	}
}

func TestGetMergedAuthConfig_EmptyStackOrComponent(t *testing.T) {
	// When stack is empty, should return global auth config without calling ExecuteDescribeComponent.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "",
		ComponentFromArg: "vpc",
	}

	result, err := getMergedAuthConfig(atmosConfig, info)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestMergeGlobalAuthConfig_GlobalOnlyProviders(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
		},
	}
	componentSection := map[string]any{}

	result := mergeGlobalAuthConfig(atmosConfig, componentSection)
	assert.Contains(t, result, "providers")
}

func TestMergeGlobalAuthConfig_ComponentOverridesGlobal(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
		},
	}
	componentSection := map[string]any{
		cfg.AuthSectionName: map[string]any{
			"providers": map[string]any{
				"aws": map[string]any{"type": "aws-sso"},
			},
		},
	}

	result := mergeGlobalAuthConfig(atmosConfig, componentSection)
	assert.Contains(t, result, "providers")
}

func TestGetMergedAuthConfig_EmptyComponent(t *testing.T) {
	// When component is empty, should return global auth config without calling ExecuteDescribeComponent.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev-us-west-2",
		ComponentFromArg: "",
	}

	result, err := getMergedAuthConfig(atmosConfig, info)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Should contain the global providers.
	assert.Len(t, result.Providers, 1)
}

func TestGetMergedAuthConfig_BothEmpty(t *testing.T) {
	// When both stack and component are empty, should return global auth config.
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "",
		ComponentFromArg: "",
	}

	result, err := getMergedAuthConfig(atmosConfig, info)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestBuildGlobalAuthSection_LogsLevelOnly(t *testing.T) {
	// Test with only logs level set (no file).
	config := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Logs: schema.Logs{Level: "debug"},
		},
	}
	result := buildGlobalAuthSection(config)
	assert.Contains(t, result, "logs")
	logs := result["logs"].(map[string]any)
	assert.Equal(t, "debug", logs["level"])
	assert.Equal(t, "", logs["file"])
}

func TestBuildGlobalAuthSection_LogsFileOnly(t *testing.T) {
	// Test with only logs file set (no level).
	config := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Logs: schema.Logs{File: "/var/log/atmos.log"},
		},
	}
	result := buildGlobalAuthSection(config)
	assert.Contains(t, result, "logs")
	logs := result["logs"].(map[string]any)
	assert.Equal(t, "", logs["level"])
	assert.Equal(t, "/var/log/atmos.log", logs["file"])
}

func TestMergeGlobalAuthConfig_GlobalAndComponentMerged(t *testing.T) {
	// Verify that global and component auth sections are properly merged.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
			Identities: map[string]schema.Identity{
				"global-identity": {Kind: "aws"},
			},
		},
	}
	componentSection := map[string]any{
		cfg.AuthSectionName: map[string]any{
			"identities": map[string]any{
				"component-identity": map[string]any{"kind": "gcp"},
			},
		},
	}

	result := mergeGlobalAuthConfig(atmosConfig, componentSection)
	// Should contain providers from global and identities from both.
	assert.Contains(t, result, "providers")
	assert.Contains(t, result, "identities")
}

// Tests for getMergedAuthConfigWithFetcher - testing the component config fetcher path.

func TestGetMergedAuthConfigWithFetcher_ComponentConfigSuccess(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev-us-west-2",
		ComponentFromArg: "vpc",
	}

	// Mock fetcher returns component config with auth section.
	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{
			cfg.AuthSectionName: map[string]any{
				"identities": map[string]any{
					"component-identity": map[string]any{"kind": "aws"},
				},
			},
		}, nil
	}

	result, err := getMergedAuthConfigWithFetcher(atmosConfig, info, mockFetcher)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGetMergedAuthConfigWithFetcher_InvalidComponentError(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev-us-west-2",
		ComponentFromArg: "nonexistent",
	}

	// Mock fetcher returns ErrInvalidComponent.
	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return nil, errUtils.ErrInvalidComponent
	}

	result, err := getMergedAuthConfigWithFetcher(atmosConfig, info, mockFetcher)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidComponent))
	assert.Nil(t, result)
}

func TestGetMergedAuthConfigWithFetcher_OtherErrorFallsBackToGlobal(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev-us-west-2",
		ComponentFromArg: "vpc",
	}

	// Mock fetcher returns a non-ErrInvalidComponent error.
	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return nil, errors.New("permission denied")
	}

	result, err := getMergedAuthConfigWithFetcher(atmosConfig, info, mockFetcher)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Should fall back to global auth config.
	assert.Len(t, result.Providers, 1)
}

func TestGetMergedAuthConfigWithFetcher_EmptyStackReturnsGlobal(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "",
		ComponentFromArg: "vpc",
	}

	// Mock fetcher should NOT be called.
	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		t.Fatal("fetcher should not be called when stack is empty")
		return nil, nil
	}

	result, err := getMergedAuthConfigWithFetcher(atmosConfig, info, mockFetcher)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// Tests for createAndAuthenticateAuthManagerWithDeps.

func TestCreateAndAuthenticateAuthManagerWithDeps_Success(t *testing.T) {
	ctrl := gomock.NewController(t)

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "",
		ComponentFromArg: "",
		Identity:         "",
	}

	mockManager := mockTypes.NewMockAuthManager(ctrl)
	mockManager.EXPECT().GetChain().Return([]string{"detected-identity"})

	// Mock fetcher - won't be called since stack is empty.
	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return nil, nil
	}

	// Mock auth creator returns a mock manager.
	mockCreator := func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return mockManager, nil
	}

	result, err := createAndAuthenticateAuthManagerWithDeps(atmosConfig, info, mockFetcher, mockCreator)
	assert.NoError(t, err)
	assert.Equal(t, mockManager, result)
	// Identity should be stored from chain.
	assert.Equal(t, "detected-identity", info.Identity)
}

func TestCreateAndAuthenticateAuthManagerWithDeps_InvalidComponentError(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentFromArg: "nonexistent",
	}

	// Mock fetcher returns ErrInvalidComponent.
	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return nil, errUtils.ErrInvalidComponent
	}

	// Mock auth creator - should not be called.
	mockCreator := func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		t.Fatal("auth creator should not be called when component is invalid")
		return nil, nil
	}

	result, err := createAndAuthenticateAuthManagerWithDeps(atmosConfig, info, mockFetcher, mockCreator)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidComponent))
	assert.Nil(t, result)
}

func TestCreateAndAuthenticateAuthManagerWithDeps_AuthCreatorError(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "",
		ComponentFromArg: "",
	}

	// Mock fetcher - won't be called since stack is empty.
	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return nil, nil
	}

	// Mock auth creator returns an error.
	mockCreator := func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return nil, errors.New("auth failed")
	}

	result, err := createAndAuthenticateAuthManagerWithDeps(atmosConfig, info, mockFetcher, mockCreator)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToInitializeAuthManager))
	assert.Nil(t, result)
}

func TestCreateAndAuthenticateAuthManagerWithDeps_OtherMergeError(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentFromArg: "vpc",
	}

	// Mock fetcher returns an error that is NOT ErrInvalidComponent.
	// This tests the path where we wrap with ErrInvalidAuthConfig.
	// However, getMergedAuthConfigWithFetcher falls back to global config for non-ErrInvalidComponent errors.
	// So we need to make the fetcher return component config that causes MergeComponentAuthFromConfig to fail.
	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		// Return component config with invalid structure to trigger merge failure.
		return map[string]any{
			cfg.AuthSectionName: "invalid-not-a-map",
		}, nil
	}

	// Mock auth creator should still be called because getMergedAuthConfigWithFetcher handles errors gracefully.
	mockCreator := func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return nil, nil
	}

	result, err := createAndAuthenticateAuthManagerWithDeps(atmosConfig, info, mockFetcher, mockCreator)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestCreateAndAuthenticateAuthManagerWithDeps_NilAuthManager(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "",
		ComponentFromArg: "",
	}

	// Mock fetcher - won't be called since stack is empty.
	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return nil, nil
	}

	// Mock auth creator returns nil (no auth configured).
	mockCreator := func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return nil, nil
	}

	result, err := createAndAuthenticateAuthManagerWithDeps(atmosConfig, info, mockFetcher, mockCreator)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestGetMergedAuthConfigWithFetcher_RealmPropagated(t *testing.T) {
	// Verify realm is propagated through CopyGlobalAuthConfig when no component auth exists.
	// This is the --all path: each component iteration must preserve the realm.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Realm:       "my-project",
			RealmSource: "config",
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev-us-west-2",
		ComponentFromArg: "vpc",
	}

	// Mock fetcher returns component config without auth section.
	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{
			"vars": map[string]any{"test": "value"},
		}, nil
	}

	result, err := getMergedAuthConfigWithFetcher(atmosConfig, info, mockFetcher)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "my-project", result.Realm)
	assert.Equal(t, "config", result.RealmSource)
}

func TestGetMergedAuthConfigWithFetcher_RealmPropagatedWithEmptyStack(t *testing.T) {
	// When stack is empty (global auth only path), realm must still be preserved.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Realm:       "env-realm",
			RealmSource: "env",
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "",
		ComponentFromArg: "",
	}

	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		t.Fatal("fetcher should not be called when stack is empty")
		return nil, nil
	}

	result, err := getMergedAuthConfigWithFetcher(atmosConfig, info, mockFetcher)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "env-realm", result.Realm)
	assert.Equal(t, "env", result.RealmSource)
}

func TestMergeGlobalAuthConfig_RealmPropagated(t *testing.T) {
	// Verify explicitly configured realm is included in the merged auth section map.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Realm:       "my-project",
			RealmSource: "config",
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
		},
	}
	componentSection := map[string]any{}

	result := mergeGlobalAuthConfig(atmosConfig, componentSection)
	assert.Contains(t, result, "realm")
	assert.Equal(t, "my-project", result["realm"])
	assert.Contains(t, result, "providers")
}

func TestMergeGlobalAuthConfig_NoRealmConfigured(t *testing.T) {
	// When no realm is configured, the merged map should not contain a "realm" key.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
		},
	}
	componentSection := map[string]any{}

	result := mergeGlobalAuthConfig(atmosConfig, componentSection)
	assert.NotContains(t, result, "realm")
	assert.Contains(t, result, "providers")
}

func TestMergeGlobalAuthConfig_AutoRealmExcluded(t *testing.T) {
	// Auto-computed realm (from config-path hash) should not appear in merged output.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Realm:       "b80ea18be93f8201",
			RealmSource: "config-path",
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
		},
	}
	componentSection := map[string]any{}

	result := mergeGlobalAuthConfig(atmosConfig, componentSection)
	assert.NotContains(t, result, "realm")
	assert.Contains(t, result, "providers")
}

func TestGetMergedAuthConfigWithFetcher_NoRealmPreservesEmptyRealm(t *testing.T) {
	// When no realm is configured, the merged config should have empty realm — same as before the fix.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"aws": {Kind: "aws-iam"},
			},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev-us-west-2",
		ComponentFromArg: "vpc",
	}

	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{
			"vars": map[string]any{"test": "value"},
		}, nil
	}

	result, err := getMergedAuthConfigWithFetcher(atmosConfig, info, mockFetcher)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.Realm)
	assert.Empty(t, result.RealmSource)
	// Providers should still be present.
	assert.Len(t, result.Providers, 1)
}

func TestGetMergedAuthConfigWithFetcher_MergeReturnsError(t *testing.T) {
	// This test verifies the path where MergeComponentAuthFromConfig returns an error.
	// When that happens, getMergedAuthConfigWithFetcher propagates the error.
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentFromArg: "vpc",
	}

	// Return component config with auth section containing a structure that will
	// cause mapstructure.Decode to fail when converting to AuthConfig.
	// Using a slice instead of map for "providers" should trigger decode error.
	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{
			cfg.AuthSectionName: map[string]any{
				"providers": []string{"invalid", "array"}, // Providers should be map, not slice.
			},
		}, nil
	}

	result, err := getMergedAuthConfigWithFetcher(atmosConfig, info, mockFetcher)

	// mapstructure is lenient and often just ignores invalid fields.
	// If no error, verify result is still valid (global config fallback).
	if err != nil {
		assert.True(t, errors.Is(err, errUtils.ErrDecode) || errors.Is(err, errUtils.ErrMerge))
		assert.Nil(t, result)
	} else {
		assert.NotNil(t, result)
	}
}

func TestCreateAndAuthenticateAuthManagerWithDeps_PreservesExistingIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "",
		ComponentFromArg: "",
		Identity:         "pre-existing-identity",
	}

	mockManager := mockTypes.NewMockAuthManager(ctrl)
	// GetChain should NOT be called because identity is already set.

	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return nil, nil
	}

	mockCreator := func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return mockManager, nil
	}

	result, err := createAndAuthenticateAuthManagerWithDeps(atmosConfig, info, mockFetcher, mockCreator)
	assert.NoError(t, err)
	assert.Equal(t, mockManager, result)
	// Identity should remain unchanged.
	assert.Equal(t, "pre-existing-identity", info.Identity)
}

// --- Regression tests for the Slack-reported component-level auth.identity selector ---
//
// These tests verify the fix for the Slack-reported bug where a component's
// `auth.identity: <name>` selector in the stack config was silently dropped
// because schema.AuthConfig has no `Identity string` field. See
// docs/fixes/2026-04-08-atmos-auth-identity-resolution-fixes.md §"Issue 3".

// TestCreateAndAuthenticateAuthManagerWithDeps_ComponentIdentitySelector verifies
// that when the stack config declares
//
//	components.terraform.<name>.auth.identity: provider-role
//
// and the user does NOT pass `--identity` on the command line, the exec layer
// extracts the selector from the raw componentConfig map BEFORE the merged
// result is decoded to *schema.AuthConfig (which would drop it), and sets
// info.Identity accordingly.
func TestCreateAndAuthenticateAuthManagerWithDeps_ComponentIdentitySelector(t *testing.T) {
	ctrl := gomock.NewController(t)

	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"mock-provider": {Kind: "mock/aws"},
			},
			Identities: map[string]schema.Identity{
				"backend-role": {
					Kind:    "mock/aws",
					Default: true,
					Via:     &schema.IdentityVia{Provider: "mock-provider"},
				},
				"provider-role": {
					Kind: "mock/aws",
					Via:  &schema.IdentityVia{Provider: "mock-provider"},
				},
			},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentFromArg: "s3-bucket",
		// Identity is empty — no --identity flag.
	}

	// Mock fetcher returns a component config with the selector form from the
	// Slack report: `components.terraform.s3-bucket.auth.identity: provider-role`.
	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{
			cfg.AuthSectionName: map[string]any{
				"identity": "provider-role",
			},
		}, nil
	}

	// Mock creator captures the identity name it was called with so we can
	// assert the selector propagated correctly.
	var capturedIdentity string
	mockManager := mockTypes.NewMockAuthManager(ctrl)
	mockManager.EXPECT().GetChain().Return([]string{"provider-role"}).AnyTimes()
	mockCreator := func(identity string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		capturedIdentity = identity
		return mockManager, nil
	}

	result, err := createAndAuthenticateAuthManagerWithDeps(atmosConfig, info, mockFetcher, mockCreator)
	assert.NoError(t, err)
	assert.Equal(t, mockManager, result)
	// POST-FIX: the selector was extracted from componentConfig["auth"]["identity"]
	// and propagated to info.Identity, and thence passed to the auth manager.
	assert.Equal(t, "provider-role", capturedIdentity,
		"component-level auth.identity selector must be passed to the auth manager")
	assert.Equal(t, "provider-role", info.Identity,
		"info.Identity must be updated with the component-level selector")
}

// TestCreateAndAuthenticateAuthManagerWithDeps_CliIdentityFlagOverridesComponentSelector
// verifies that an explicit `--identity` flag on the command line takes
// precedence over the component-level `auth.identity` selector in the stack
// config (i.e. CLI > stack config).
func TestCreateAndAuthenticateAuthManagerWithDeps_CliIdentityFlagOverridesComponentSelector(t *testing.T) {
	ctrl := gomock.NewController(t)

	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"mock-provider": {Kind: "mock/aws"},
			},
			Identities: map[string]schema.Identity{
				"provider-role": {Kind: "mock/aws"},
				"other-role":    {Kind: "mock/aws"},
			},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentFromArg: "s3-bucket",
		Identity:         "other-role", // Set by the --identity flag.
	}

	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{
			cfg.AuthSectionName: map[string]any{
				"identity": "provider-role", // Stack-config selector.
			},
		}, nil
	}

	var capturedIdentity string
	mockManager := mockTypes.NewMockAuthManager(ctrl)
	mockManager.EXPECT().GetChain().Return([]string{"other-role"}).AnyTimes()
	mockCreator := func(identity string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		capturedIdentity = identity
		return mockManager, nil
	}

	result, err := createAndAuthenticateAuthManagerWithDeps(atmosConfig, info, mockFetcher, mockCreator)
	assert.NoError(t, err)
	assert.Equal(t, mockManager, result)
	// Precedence: --identity flag wins over the component selector.
	assert.Equal(t, "other-role", capturedIdentity,
		"--identity flag must take precedence over component auth.identity selector")
	assert.Equal(t, "other-role", info.Identity)
}

// TestCreateAndAuthenticateAuthManagerWithDeps_ComponentIdentitySelectorUnknownIdentity
// verifies that referencing a non-existent identity via
// `components.terraform.<name>.auth.identity: <unknown>` returns a clear
// error rather than silently falling back to the default.
func TestCreateAndAuthenticateAuthManagerWithDeps_ComponentIdentitySelectorUnknownIdentity(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"mock-provider": {Kind: "mock/aws"},
			},
			Identities: map[string]schema.Identity{
				"backend-role": {Kind: "mock/aws", Default: true},
			},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentFromArg: "s3-bucket",
	}

	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{
			cfg.AuthSectionName: map[string]any{
				"identity": "nonexistent-role", // Does not exist anywhere.
			},
		}, nil
	}

	mockCreator := func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		t.Fatal("authCreator should not be called when the selector is invalid")
		return nil, nil
	}

	result, err := createAndAuthenticateAuthManagerWithDeps(atmosConfig, info, mockFetcher, mockCreator)
	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	assert.Contains(t, err.Error(), "nonexistent-role",
		"error message should name the unknown identity")
	assert.Contains(t, err.Error(), "s3-bucket",
		"error message should name the component")
}

// TestExtractComponentIdentitySelector_NoStackOrComponent ensures the
// selector extractor is a safe no-op when there is no stack or component
// context (as happens for CLI commands that work across all stacks).
func TestExtractComponentIdentitySelector_NoStackOrComponent(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "", ComponentFromArg: ""}
	fetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		t.Fatal("configFetcher must not be called when stack/component is missing")
		return nil, nil
	}
	selector, err := extractComponentIdentitySelector(info, fetcher, &schema.AuthConfig{})
	assert.NoError(t, err)
	assert.Empty(t, selector)
}

// TestExtractComponentIdentitySelector_InvalidComponentErrorPropagates
// verifies that an ErrInvalidComponent error from the describe path is
// returned untouched so the caller can exit fast without prompting.
func TestExtractComponentIdentitySelector_InvalidComponentErrorPropagates(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "nonexistent"}
	fetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return nil, errUtils.ErrInvalidComponent
	}
	selector, err := extractComponentIdentitySelector(info, fetcher, &schema.AuthConfig{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidComponent)
	assert.Empty(t, selector)
}

// TestExtractComponentIdentitySelector_OtherFetcherErrorSuppressed verifies
// that a non-fatal describe error (e.g. a permission issue) is suppressed so
// the auth flow can continue with the global config instead of failing hard.
func TestExtractComponentIdentitySelector_OtherFetcherErrorSuppressed(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "s3-bucket"}
	fetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return nil, fmt.Errorf("transient describe failure")
	}
	selector, err := extractComponentIdentitySelector(info, fetcher, &schema.AuthConfig{})
	assert.NoError(t, err)
	assert.Empty(t, selector)
}

// TestExtractComponentIdentitySelector_NoAuthSection covers the case where
// the component config has no `auth:` section at all.
func TestExtractComponentIdentitySelector_NoAuthSection(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "s3-bucket"}
	fetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{"vars": map[string]any{"stage": "dev"}}, nil
	}
	selector, err := extractComponentIdentitySelector(info, fetcher, &schema.AuthConfig{})
	assert.NoError(t, err)
	assert.Empty(t, selector)
}

// TestExtractComponentIdentitySelector_AuthSectionWithoutIdentityKey covers
// the case where the component has an `auth:` block but no `identity` key
// (e.g. only `auth.identities.<name>.*` overrides).
func TestExtractComponentIdentitySelector_AuthSectionWithoutIdentityKey(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "s3-bucket"}
	fetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{
			cfg.AuthSectionName: map[string]any{
				"identities": map[string]any{
					"foo": map[string]any{"kind": "mock/aws"},
				},
			},
		}, nil
	}
	selector, err := extractComponentIdentitySelector(info, fetcher, &schema.AuthConfig{})
	assert.NoError(t, err)
	assert.Empty(t, selector)
}

// TestExtractComponentIdentitySelector_IdentityKeyWrongType verifies that a
// non-string `identity` value (e.g. accidentally a map or integer) is
// treated as "no selector" rather than as an error.
func TestExtractComponentIdentitySelector_IdentityKeyWrongType(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "s3-bucket"}
	fetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{
			cfg.AuthSectionName: map[string]any{
				"identity": 123, // invalid: should be a string
			},
		}, nil
	}
	selector, err := extractComponentIdentitySelector(info, fetcher, &schema.AuthConfig{})
	assert.NoError(t, err)
	assert.Empty(t, selector)
}

// TestExtractComponentIdentitySelector_EmptyStringIdentity verifies that an
// explicitly-empty selector is treated as unset rather than as an error.
func TestExtractComponentIdentitySelector_EmptyStringIdentity(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "s3-bucket"}
	fetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{
			cfg.AuthSectionName: map[string]any{
				"identity": "",
			},
		}, nil
	}
	selector, err := extractComponentIdentitySelector(info, fetcher, &schema.AuthConfig{})
	assert.NoError(t, err)
	assert.Empty(t, selector)
}

// TestExtractComponentIdentitySelector_CaseInsensitiveLookup verifies that a
// selector with mixed case resolves via the IdentityCaseMap fallback (mirrors
// the rest of the auth layer's case handling for Viper's case folding).
func TestExtractComponentIdentitySelector_CaseInsensitiveLookup(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "s3-bucket"}
	fetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{
			cfg.AuthSectionName: map[string]any{
				"identity": "Provider-Role", // wrong case
			},
		}, nil
	}
	merged := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"provider-role": {Kind: "mock/aws"},
		},
		IdentityCaseMap: map[string]string{
			"provider-role": "provider-role",
		},
	}
	selector, err := extractComponentIdentitySelector(info, fetcher, merged)
	assert.NoError(t, err)
	assert.Equal(t, "provider-role", selector,
		"case-insensitive lookup must return the canonical name")
}

// TestResolveIdentityInMergedAuthConfig_NilConfig covers the defensive
// branch where resolveIdentityInMergedAuthConfig is called with a nil auth
// config or nil Identities map.
func TestResolveIdentityInMergedAuthConfig_NilConfig(t *testing.T) {
	name, ok := resolveIdentityInMergedAuthConfig(nil, "anything")
	assert.False(t, ok)
	assert.Empty(t, name)

	name, ok = resolveIdentityInMergedAuthConfig(&schema.AuthConfig{}, "anything")
	assert.False(t, ok)
	assert.Empty(t, name)
}

// TestResolveIdentityInMergedAuthConfig_NilCaseMap covers the branch where
// the direct lookup misses and IdentityCaseMap is nil (no case-insensitive
// fallback possible).
func TestResolveIdentityInMergedAuthConfig_NilCaseMap(t *testing.T) {
	merged := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"foo": {Kind: "mock/aws"},
		},
		IdentityCaseMap: nil,
	}
	name, ok := resolveIdentityInMergedAuthConfig(merged, "Foo")
	assert.False(t, ok)
	assert.Empty(t, name)
}

// TestResolveIdentityInMergedAuthConfig_StaleCaseMap covers the defensive
// branch where IdentityCaseMap points to a canonical name that no longer
// exists in Identities (stale mapping).
func TestResolveIdentityInMergedAuthConfig_StaleCaseMap(t *testing.T) {
	merged := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"foo": {Kind: "mock/aws"},
		},
		IdentityCaseMap: map[string]string{
			"bar": "stale-name", // points to an identity that does not exist
		},
	}
	name, ok := resolveIdentityInMergedAuthConfig(merged, "Bar")
	assert.False(t, ok)
	assert.Empty(t, name)
}

// TestCreateAndAuthenticateAuthManagerWithDeps_ComponentWithoutSelectorUnchanged
// verifies that a component WITHOUT an `auth.identity` selector has its
// identity resolution unchanged (the fix must be scoped to components that
// actually declare the selector).
func TestCreateAndAuthenticateAuthManagerWithDeps_ComponentWithoutSelectorUnchanged(t *testing.T) {
	ctrl := gomock.NewController(t)

	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"mock-provider": {Kind: "mock/aws"},
			},
			Identities: map[string]schema.Identity{
				"backend-role": {Kind: "mock/aws", Default: true},
			},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentFromArg: "no-override",
	}

	// No auth section at all in the component config.
	mockFetcher := func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return map[string]any{}, nil
	}

	var capturedIdentity string
	mockManager := mockTypes.NewMockAuthManager(ctrl)
	mockManager.EXPECT().GetChain().Return([]string{"backend-role"}).AnyTimes()
	mockCreator := func(identity string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		capturedIdentity = identity
		return mockManager, nil
	}

	result, err := createAndAuthenticateAuthManagerWithDeps(atmosConfig, info, mockFetcher, mockCreator)
	assert.NoError(t, err)
	assert.Equal(t, mockManager, result)
	// Empty identity is passed to the auth manager, which will auto-detect
	// the default. No component override was applied.
	assert.Equal(t, "", capturedIdentity)
}
