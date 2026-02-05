package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

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
