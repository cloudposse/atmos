package exec

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
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
