package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestManager_GetStackInfo_ReturnsStackInfo(t *testing.T) {
	stackInfo := &schema.ConfigAndStacksInfo{}

	m := &manager{
		stackInfo: stackInfo,
	}

	result := m.GetStackInfo()
	assert.Equal(t, stackInfo, result)
}

func TestManager_GetStackInfo_ReturnsNil(t *testing.T) {
	m := &manager{
		stackInfo: nil,
	}

	result := m.GetStackInfo()
	assert.Nil(t, result)
}

func TestManager_GetChain_ReturnsChain(t *testing.T) {
	m := &manager{
		chain: []string{"provider1", "identity1", "identity2"},
	}

	chain := m.GetChain()
	assert.Equal(t, []string{"provider1", "identity1", "identity2"}, chain)
}

func TestManager_GetChain_ReturnsEmpty(t *testing.T) {
	m := &manager{
		chain: nil,
	}

	chain := m.GetChain()
	assert.Nil(t, chain)
}

func TestManager_GetIdentities_ReturnsIdentities(t *testing.T) {
	identities := map[string]schema.Identity{
		"identity1": {Kind: "aws/permission-set"},
		"identity2": {Kind: "aws/assume-role"},
	}

	m := &manager{
		config: &schema.AuthConfig{
			Identities: identities,
		},
	}

	result := m.GetIdentities()
	assert.Equal(t, identities, result)
}

func TestManager_GetProviders_ReturnsProviders(t *testing.T) {
	providers := map[string]schema.Provider{
		"provider1": {Kind: "aws/iam-identity-center"},
		"provider2": {Kind: "aws/iam-identity-center"},
	}

	m := &manager{
		config: &schema.AuthConfig{
			Providers: providers,
		},
	}

	result := m.GetProviders()
	assert.Equal(t, providers, result)
}

func TestManager_GetConfig_ReturnsConfig(t *testing.T) {
	stackInfo := &schema.ConfigAndStacksInfo{}

	m := &manager{
		stackInfo: stackInfo,
	}

	result := m.GetConfig()
	assert.Equal(t, stackInfo, result)
}

func TestManager_DetermineStartingIndex_NoCache(t *testing.T) {
	m := &manager{}

	// -1 should return 0 (start from provider).
	result := m.determineStartingIndex(-1)
	assert.Equal(t, 0, result)
}

func TestManager_DetermineStartingIndex_WithCache(t *testing.T) {
	m := &manager{}

	// Non-negative index should be returned as-is.
	result := m.determineStartingIndex(2)
	assert.Equal(t, 2, result)
}

func TestManager_GetProviderForIdentity_NoChain(t *testing.T) {
	config := &schema.AuthConfig{
		Providers: map[string]schema.Provider{},
		Identities: map[string]schema.Identity{
			"identity1": {Kind: "aws/permission-set"},
		},
	}

	m := &manager{
		config: config,
	}

	// No chain built yet, should return empty string.
	result := m.GetProviderForIdentity("identity1")
	assert.Empty(t, result)
}

func TestManager_GetProviderKindForIdentity_EmptyChain(t *testing.T) {
	config := &schema.AuthConfig{
		Providers: map[string]schema.Provider{},
		Identities: map[string]schema.Identity{
			"identity1": {Kind: "aws/permission-set"},
		},
	}

	m := &manager{
		config: config,
	}

	// Empty chain should return error.
	_, err := m.GetProviderKindForIdentity("non-existent")
	assert.Error(t, err)
}

func TestManager_GetFilesDisplayPath_ProviderNotFound(t *testing.T) {
	m := &manager{
		providers: map[string]types.Provider{},
	}

	// Provider not found should return default path.
	path := m.GetFilesDisplayPath("non-existent")
	assert.Contains(t, path, ".config/atmos")
}

func TestManager_EnsureIdentityHasManager_NoConfig(t *testing.T) {
	m := &manager{
		config: nil,
	}

	// No config should return nil (no error).
	err := m.ensureIdentityHasManager("identity1")
	assert.NoError(t, err)
}

func TestManager_EnsureIdentityHasManager_ChainExists(t *testing.T) {
	config := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"provider1": {Kind: "aws/iam-identity-center"},
		},
		Identities: map[string]schema.Identity{
			"identity1": {
				Kind: "aws/permission-set",
				Via:  &schema.IdentityVia{Provider: "provider1"},
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockIdentity := types.NewMockIdentity(ctrl)
	identities := map[string]types.Identity{
		"identity1": mockIdentity,
	}

	m := &manager{
		config:     config,
		identities: identities,
		chain:      []string{"provider1", "identity1"},
	}

	// Chain already exists and matches identity - should not error.
	err := m.ensureIdentityHasManager("identity1")
	assert.NoError(t, err)
}

func TestManager_SetIdentityManager_NoChain(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockIdentity := types.NewMockIdentity(ctrl)

	m := &manager{
		identities: map[string]types.Identity{
			"identity1": mockIdentity,
		},
		chain: []string{},
	}

	// No chain should return error.
	err := m.setIdentityManager("identity1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationChainNotBuilt)
}

func TestManager_SetIdentityManager_IdentityNotFound(t *testing.T) {
	m := &manager{
		identities: map[string]types.Identity{},
		chain:      []string{"provider1", "identity1"},
	}

	// Identity not found should return nil (no error).
	err := m.setIdentityManager("non-existent")
	assert.NoError(t, err)
}

func TestManager_FindFirstValidCachedCredentials_NoCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := types.NewMockCredentialStore(ctrl)

	m := &manager{
		credentialStore: mockStore,
		chain:           []string{"provider1", "identity1"},
	}

	// No credentials found should return -1.
	mockStore.EXPECT().Retrieve("identity1").Return(nil, errors.New("not found"))
	mockStore.EXPECT().Retrieve("provider1").Return(nil, errors.New("not found"))

	result := m.findFirstValidCachedCredentials()
	assert.Equal(t, -1, result)
}

func TestManager_FetchCachedCredentials_NoStore(t *testing.T) {
	m := &manager{
		credentialStore: nil,
		chain:           []string{"provider1", "identity1"},
	}

	// No credential store should return nil credentials and start from provider (0).
	creds, startIndex := m.fetchCachedCredentials(1)
	assert.Nil(t, creds)
	assert.Equal(t, 0, startIndex)
}

func TestManager_GetChainCredentials_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := types.NewMockCredentialStore(ctrl)

	m := &manager{
		credentialStore: mockStore,
		chain:           []string{"provider1", "identity1"},
	}

	// Credential retrieval fails.
	mockStore.EXPECT().Retrieve("identity1").Return(nil, errors.New("retrieval failed"))

	_, err := m.getChainCredentials(m.chain, 1)
	assert.Error(t, err)
}

func TestManager_EnvironListToMap_EmptyList(t *testing.T) {
	result := environListToMap([]string{})
	assert.Empty(t, result)
}

func TestManager_EnvironListToMap_ValidList(t *testing.T) {
	input := []string{
		"KEY1=value1",
		"KEY2=value2",
		"KEY3=value3",
	}

	result := environListToMap(input)
	assert.Equal(t, "value1", result["KEY1"])
	assert.Equal(t, "value2", result["KEY2"])
	assert.Equal(t, "value3", result["KEY3"])
}

func TestManager_EnvironListToMap_InvalidEntries(t *testing.T) {
	input := []string{
		"KEY1=value1",
		"INVALID",
		"KEY2=value2",
	}

	result := environListToMap(input)
	assert.Equal(t, "value1", result["KEY1"])
	assert.Equal(t, "value2", result["KEY2"])
	// Invalid entry should be skipped.
	assert.NotContains(t, result, "INVALID")
}

func TestManager_MapToEnvironList_EmptyMap(t *testing.T) {
	result := mapToEnvironList(map[string]string{})
	assert.Empty(t, result)
}

func TestManager_MapToEnvironList_ValidMap(t *testing.T) {
	input := map[string]string{
		"KEY1": "value1",
		"KEY2": "value2",
	}

	result := mapToEnvironList(input)
	assert.Len(t, result, 2)
	assert.Contains(t, result, "KEY1=value1")
	assert.Contains(t, result, "KEY2=value2")
}

func TestManager_IsSessionToken_NonAWSCredentials(t *testing.T) {
	// Non-AWS credentials should return false.
	creds := &struct{ types.ICredentials }{}
	result := isSessionToken(creds)
	assert.False(t, result)
}

func TestManager_IsSessionToken_AWSWithoutToken(t *testing.T) {
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "secret",
		SessionToken:    "",
	}

	result := isSessionToken(creds)
	assert.False(t, result)
}

func TestManager_IsSessionToken_AWSWithToken(t *testing.T) {
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "secret",
		SessionToken:    "session-token",
	}

	result := isSessionToken(creds)
	assert.True(t, result)
}

func TestManager_GetChainStepName_ValidIndex(t *testing.T) {
	m := &manager{
		chain: []string{"provider1", "identity1", "identity2"},
	}

	assert.Equal(t, "provider1", m.getChainStepName(0))
	assert.Equal(t, "identity1", m.getChainStepName(1))
	assert.Equal(t, "identity2", m.getChainStepName(2))
}

func TestManager_GetChainStepName_InvalidIndex(t *testing.T) {
	m := &manager{
		chain: []string{"provider1", "identity1"},
	}

	// Out of bounds index should return "unknown".
	assert.Equal(t, "unknown", m.getChainStepName(5))
}

func TestManager_GetEnvironmentVariables_IdentityNotFound(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{},
		},
		identities: map[string]types.Identity{},
	}

	_, err := m.GetEnvironmentVariables("non-existent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
}

func TestManager_PrepareShellEnvironment_IdentityNotFound(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{},
		},
		identities: map[string]types.Identity{},
	}

	_, err := m.PrepareShellEnvironment(context.Background(), "non-existent", []string{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
}

func TestManager_GetDefaultIdentity_NoDefaults(t *testing.T) {
	// Set non-interactive environment.
	config := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"identity1": {Default: false},
			"identity2": {Default: false},
		},
	}

	m := &manager{
		config: config,
	}

	// No defaults in non-interactive mode should return error.
	_, err := m.GetDefaultIdentity(false)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoDefaultIdentity)
}

func TestManager_GetDefaultIdentity_MultipleDefaults(t *testing.T) {
	// Set non-interactive environment.
	config := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"identity1": {Default: true},
			"identity2": {Default: true},
		},
	}

	m := &manager{
		config: config,
	}

	// Multiple defaults in non-interactive mode should return error.
	_, err := m.GetDefaultIdentity(false)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMultipleDefaultIdentities)
}

func TestManager_GetDefaultIdentity_SingleDefault(t *testing.T) {
	config := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"identity1": {Default: true},
			"identity2": {Default: false},
		},
	}

	m := &manager{
		config: config,
	}

	// Single default should be returned.
	identity, err := m.GetDefaultIdentity(false)
	assert.NoError(t, err)
	assert.Equal(t, "identity1", identity)
}

func TestManager_GetDefaultIdentity_ForceSelectNoTTY(t *testing.T) {
	config := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"identity1": {Default: true},
		},
	}

	m := &manager{
		config: config,
	}

	// Force select without TTY should return error.
	_, err := m.GetDefaultIdentity(true)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIdentitySelectionRequiresTTY)
}

func TestManager_PromptForIdentity_NoIdentities(t *testing.T) {
	m := &manager{}

	_, err := m.promptForIdentity("Choose:", []string{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoIdentitiesAvailable)
}

func TestManager_ListIdentities_Extended_WithCaseMap(t *testing.T) {
	config := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"identity1": {},
			"identity2": {},
		},
		IdentityCaseMap: map[string]string{
			"identity1": "Identity1",
			"identity2": "Identity2",
		},
	}

	m := &manager{
		config: config,
	}

	identities := m.ListIdentities()
	assert.Len(t, identities, 2)
	assert.Contains(t, identities, "Identity1")
	assert.Contains(t, identities, "Identity2")
}

func TestManager_ListIdentities_Extended_WithoutCaseMap(t *testing.T) {
	config := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"identity1": {},
			"identity2": {},
		},
		IdentityCaseMap: nil,
	}

	m := &manager{
		config: config,
	}

	identities := m.ListIdentities()
	assert.Len(t, identities, 2)
	// Should use lowercase names when case map is not available.
	assert.Contains(t, identities, "identity1")
	assert.Contains(t, identities, "identity2")
}

func TestManager_ListProviders_Extended(t *testing.T) {
	config := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"provider1": {},
			"provider2": {},
		},
	}

	m := &manager{
		config: config,
	}

	providers := m.ListProviders()
	assert.Len(t, providers, 2)
	assert.Contains(t, providers, "provider1")
	assert.Contains(t, providers, "provider2")
}

func TestManager_BuildAuthenticationChain_CircularDependency(t *testing.T) {
	config := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"identity1": {
				Via: &schema.IdentityVia{Identity: "identity2"},
			},
			"identity2": {
				Via: &schema.IdentityVia{Identity: "identity1"},
			},
		},
	}

	m := &manager{
		config: config,
	}

	_, err := m.buildAuthenticationChain("identity1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCircularDependency)
}

func TestManager_BuildAuthenticationChain_IdentityNotFound(t *testing.T) {
	config := &schema.AuthConfig{
		Identities: map[string]schema.Identity{},
	}

	m := &manager{
		config: config,
	}

	_, err := m.buildAuthenticationChain("non-existent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}

func TestManager_BuildAuthenticationChain_NoViaConfiguration(t *testing.T) {
	config := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"identity1": {
				Kind: "aws/permission-set",
				Via:  nil,
			},
		},
	}

	m := &manager{
		config: config,
	}

	_, err := m.buildAuthenticationChain("identity1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}

func TestManager_BuildAuthenticationChain_InvalidViaConfiguration(t *testing.T) {
	config := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"identity1": {
				Kind: "aws/permission-set",
				Via:  &schema.IdentityVia{}, // Empty via.
			},
		},
	}

	m := &manager{
		config: config,
	}

	_, err := m.buildAuthenticationChain("identity1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}

func TestNewAuthManager_NilConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := types.NewMockCredentialStore(ctrl)
	mockValidator := types.NewMockValidator(ctrl)

	_, err := NewAuthManager(nil, mockStore, mockValidator, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

func TestNewAuthManager_NilCredentialStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	config := &schema.AuthConfig{}
	mockValidator := types.NewMockValidator(ctrl)

	_, err := NewAuthManager(config, nil, mockValidator, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

func TestNewAuthManager_NilValidator(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	config := &schema.AuthConfig{}
	mockStore := types.NewMockCredentialStore(ctrl)

	_, err := NewAuthManager(config, mockStore, nil, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

func TestManager_GetCachedCredentials_ExpiredCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := types.NewMockCredentialStore(ctrl)
	mockIdentity := types.NewMockIdentity(ctrl)

	config := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"identity1": {
				Kind: "aws/permission-set",
			},
		},
	}

	m := &manager{
		config: config,
		identities: map[string]types.Identity{
			"identity1": mockIdentity,
		},
		credentialStore: mockStore,
	}

	// Return expired credentials.
	expiredCreds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "secret",
		Expiration:      "2020-01-01T00:00:00Z",
	}

	mockStore.EXPECT().Retrieve("identity1").Return(expiredCreds, nil)

	_, err := m.GetCachedCredentials(context.Background(), "identity1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrExpiredCredentials)
}
