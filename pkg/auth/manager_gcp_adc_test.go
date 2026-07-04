package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
)

// --- Manager-Level GCP ADC Chain Tests ---
//
// These tests verify the complete auth manager setup and chain building
// for GCP ADC flows. They test that:
//   1. Manager initializes GCP ADC provider + gcp/project or gcp/service-account identities.
//   2. Chain building works correctly (provider → identity).
//   3. Realm propagation works for both empty and explicit realms.
//   4. Identity/provider resolution returns correct kinds and names.

// TestManager_GCP_ADC_ProjectIdentity_Init verifies that NewAuthManager
// correctly initializes a gcp/adc provider and gcp/project identity.
func TestManager_GCP_ADC_ProjectIdentity_Init(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"my-gcp-adc": {
				Kind: "gcp/adc",
				Spec: map[string]any{
					"project_id": "test-project",
				},
			},
		},
		Identities: map[string]schema.Identity{
			"my-gcp-project": {
				Kind: "gcp/project",
				Via:  &schema.IdentityVia{Provider: "my-gcp-adc"},
				Principal: map[string]any{
					"project_id": "test-project",
					"region":     "us-central1",
				},
			},
		},
	}, credStore, validator, nil, "")
	require.NoError(t, err, "Manager should initialize with GCP ADC + project identity")
	require.NotNil(t, m)

	// Verify provider and identity are registered.
	mgr := m.(*manager)
	assert.Contains(t, mgr.providers, "my-gcp-adc")
	assert.Contains(t, mgr.identities, "my-gcp-project")

	// Verify provider kind.
	assert.Equal(t, "gcp/adc", mgr.providers["my-gcp-adc"].Kind())

	// Verify identity kind.
	assert.Equal(t, "gcp/project", mgr.identities["my-gcp-project"].Kind())
}

// TestManager_GCP_ADC_ServiceAccountIdentity_Init verifies that NewAuthManager
// correctly initializes a gcp/adc provider and gcp/service-account identity.
func TestManager_GCP_ADC_ServiceAccountIdentity_Init(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"my-gcp-adc": {
				Kind: "gcp/adc",
				Spec: map[string]any{
					"project_id": "test-project",
				},
			},
		},
		Identities: map[string]schema.Identity{
			"my-gcp-sa": {
				Kind: "gcp/service-account",
				Via:  &schema.IdentityVia{Provider: "my-gcp-adc"},
				Principal: map[string]any{
					"service_account_email": "sa@test-project.iam.gserviceaccount.com",
				},
			},
		},
	}, credStore, validator, nil, "")
	require.NoError(t, err, "Manager should initialize with GCP ADC + service-account identity")
	require.NotNil(t, m)

	mgr := m.(*manager)
	assert.Contains(t, mgr.providers, "my-gcp-adc")
	assert.Contains(t, mgr.identities, "my-gcp-sa")
	assert.Equal(t, "gcp/service-account", mgr.identities["my-gcp-sa"].Kind())
}

// TestManager_GCP_ADC_EmptyRealm_Allowed verifies that the manager accepts
// empty realm (no auth.realm configured) which is the default for new users.
func TestManager_GCP_ADC_EmptyRealm_Allowed(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		// Realm intentionally empty.
		Providers: map[string]schema.Provider{
			"my-gcp-adc": {
				Kind: "gcp/adc",
				Spec: map[string]any{
					"project_id": "test-project",
				},
			},
		},
		Identities: map[string]schema.Identity{
			"my-gcp-project": {
				Kind: "gcp/project",
				Via:  &schema.IdentityVia{Provider: "my-gcp-adc"},
				Principal: map[string]any{
					"project_id": "test-project",
				},
			},
		},
	}, credStore, validator, nil, "")
	require.NoError(t, err, "Empty realm should be accepted by manager")
	require.NotNil(t, m)

	// Verify realm is auto-sourced and empty.
	mgr := m.(*manager)
	assert.Equal(t, realm.SourceAuto, mgr.realm.Source)
	assert.Empty(t, mgr.realm.Value, "Auto realm should be empty")
}

// TestManager_GCP_ADC_ExplicitRealm_Propagated verifies that an explicit realm
// is propagated to all providers and identities.
func TestManager_GCP_ADC_ExplicitRealm_Propagated(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Realm: "customer-acme",
		Providers: map[string]schema.Provider{
			"my-gcp-adc": {
				Kind: "gcp/adc",
				Spec: map[string]any{
					"project_id": "test-project",
				},
			},
		},
		Identities: map[string]schema.Identity{
			"my-gcp-project": {
				Kind: "gcp/project",
				Via:  &schema.IdentityVia{Provider: "my-gcp-adc"},
				Principal: map[string]any{
					"project_id": "test-project",
				},
			},
		},
	}, credStore, validator, nil, "")
	require.NoError(t, err)
	require.NotNil(t, m)

	mgr := m.(*manager)
	assert.Equal(t, realm.SourceConfig, mgr.realm.Source)
	assert.Equal(t, "customer-acme", mgr.realm.Value)
}

// TestManager_GCP_ADC_ChainBuilding verifies that buildAuthenticationChain
// correctly builds the chain: [gcp-adc-provider, gcp-project-identity].
func TestManager_GCP_ADC_ChainBuilding(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"my-gcp-adc": {
				Kind: "gcp/adc",
				Spec: map[string]any{
					"project_id": "test-project",
				},
			},
		},
		Identities: map[string]schema.Identity{
			"my-gcp-project": {
				Kind: "gcp/project",
				Via:  &schema.IdentityVia{Provider: "my-gcp-adc"},
				Principal: map[string]any{
					"project_id": "test-project",
				},
			},
		},
	}, credStore, validator, nil, "")
	require.NoError(t, err)

	mgr := m.(*manager)
	chain, err := mgr.buildAuthenticationChain("my-gcp-project")
	require.NoError(t, err)

	// Chain should be: [provider, identity].
	require.Len(t, chain, 2)
	assert.Equal(t, "my-gcp-adc", chain[0], "First element should be the ADC provider")
	assert.Equal(t, "my-gcp-project", chain[1], "Second element should be the project identity")
}

// TestManager_GCP_ADC_ServiceAccount_ChainBuilding verifies chain building
// for ADC → service-account impersonation.
func TestManager_GCP_ADC_ServiceAccount_ChainBuilding(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"my-gcp-adc": {
				Kind: "gcp/adc",
				Spec: map[string]any{
					"project_id": "test-project",
				},
			},
		},
		Identities: map[string]schema.Identity{
			"deployer": {
				Kind: "gcp/service-account",
				Via:  &schema.IdentityVia{Provider: "my-gcp-adc"},
				Principal: map[string]any{
					"service_account_email": "deployer@prod.iam.gserviceaccount.com",
				},
			},
		},
	}, credStore, validator, nil, "")
	require.NoError(t, err)

	mgr := m.(*manager)
	chain, err := mgr.buildAuthenticationChain("deployer")
	require.NoError(t, err)

	require.Len(t, chain, 2)
	assert.Equal(t, "my-gcp-adc", chain[0])
	assert.Equal(t, "deployer", chain[1])
}

// TestManager_GCP_ADC_MultipleIdentities verifies that manager correctly
// initializes multiple GCP identities sharing the same ADC provider.
func TestManager_GCP_ADC_MultipleIdentities(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"shared-adc": {
				Kind: "gcp/adc",
				Spec: map[string]any{
					"project_id": "default-project",
				},
			},
		},
		Identities: map[string]schema.Identity{
			"project-ctx": {
				Kind: "gcp/project",
				Via:  &schema.IdentityVia{Provider: "shared-adc"},
				Principal: map[string]any{
					"project_id": "project-alpha",
					"region":     "us-east1",
				},
			},
			"sa-deployer": {
				Kind: "gcp/service-account",
				Via:  &schema.IdentityVia{Provider: "shared-adc"},
				Principal: map[string]any{
					"service_account_email": "deployer@project-beta.iam.gserviceaccount.com",
				},
			},
		},
	}, credStore, validator, nil, "")
	require.NoError(t, err)

	mgr := m.(*manager)

	// Verify both identities exist.
	assert.Contains(t, mgr.identities, "project-ctx")
	assert.Contains(t, mgr.identities, "sa-deployer")

	// Verify both chain to the same provider.
	chain1, err := mgr.buildAuthenticationChain("project-ctx")
	require.NoError(t, err)
	assert.Equal(t, "shared-adc", chain1[0])

	chain2, err := mgr.buildAuthenticationChain("sa-deployer")
	require.NoError(t, err)
	assert.Equal(t, "shared-adc", chain2[0])

	// Verify identities list contains both.
	identityNames := m.ListIdentities()
	assert.Contains(t, identityNames, "project-ctx")
	assert.Contains(t, identityNames, "sa-deployer")
}

// TestManager_GCP_ADC_GetProviderKindForIdentity verifies that
// GetProviderKindForIdentity returns "gcp/adc" for GCP identities.
func TestManager_GCP_ADC_GetProviderKindForIdentity(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"my-adc": {
				Kind: "gcp/adc",
				Spec: map[string]any{
					"project_id": "test-project",
				},
			},
		},
		Identities: map[string]schema.Identity{
			"my-project": {
				Kind: "gcp/project",
				Via:  &schema.IdentityVia{Provider: "my-adc"},
				Principal: map[string]any{
					"project_id": "test-project",
				},
			},
			"my-sa": {
				Kind: "gcp/service-account",
				Via:  &schema.IdentityVia{Provider: "my-adc"},
				Principal: map[string]any{
					"service_account_email": "sa@test.iam.gserviceaccount.com",
				},
			},
		},
	}, credStore, validator, nil, "")
	require.NoError(t, err)

	// Both identities should resolve to gcp/adc provider kind.
	kind1, err := m.GetProviderKindForIdentity("my-project")
	require.NoError(t, err)
	assert.Equal(t, "gcp/adc", kind1)

	kind2, err := m.GetProviderKindForIdentity("my-sa")
	require.NoError(t, err)
	assert.Equal(t, "gcp/adc", kind2)
}

// TestManager_GCP_ADC_GetEnvironmentVariables verifies that
// GetEnvironmentVariables returns GCP project env vars for both identity types.
func TestManager_GCP_ADC_GetEnvironmentVariables(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"my-adc": {
				Kind: "gcp/adc",
				Spec: map[string]any{
					"project_id": "provider-project",
				},
			},
		},
		Identities: map[string]schema.Identity{
			"my-project": {
				Kind: "gcp/project",
				Via:  &schema.IdentityVia{Provider: "my-adc"},
				Principal: map[string]any{
					"project_id": "identity-project",
					"region":     "europe-west1",
				},
			},
		},
	}, credStore, validator, nil, "")
	require.NoError(t, err)

	env, err := m.GetEnvironmentVariables("my-project")
	require.NoError(t, err)
	require.NotNil(t, env)

	// Should contain project env vars from the identity.
	assert.Equal(t, "identity-project", env["GOOGLE_CLOUD_PROJECT"])
	assert.Equal(t, "identity-project", env["CLOUDSDK_CORE_PROJECT"])
	assert.Equal(t, "europe-west1", env["GOOGLE_CLOUD_REGION"])
}

// TestManager_GCP_ADC_GetRealmInfo verifies that GetRealm returns correct
// realm information for different configurations.
func TestManager_GCP_ADC_GetRealmInfo(t *testing.T) {
	tests := []struct {
		name           string
		configRealm    string
		expectedSource string
		expectedValue  string
	}{
		{
			name:           "empty realm returns auto source",
			configRealm:    "",
			expectedSource: realm.SourceAuto,
			expectedValue:  "",
		},
		{
			name:           "explicit realm returns config source",
			configRealm:    "customer-acme",
			expectedSource: realm.SourceConfig,
			expectedValue:  "customer-acme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credStore := credentials.NewCredentialStore()
			validator := validation.NewValidator()

			m, err := NewAuthManager(&schema.AuthConfig{
				Realm: tt.configRealm,
				Providers: map[string]schema.Provider{
					"adc": {
						Kind: "gcp/adc",
						Spec: map[string]any{
							"project_id": "test",
						},
					},
				},
				Identities: map[string]schema.Identity{
					"proj": {
						Kind: "gcp/project",
						Via:  &schema.IdentityVia{Provider: "adc"},
						Principal: map[string]any{
							"project_id": "test",
						},
					},
				},
			}, credStore, validator, nil, "")
			require.NoError(t, err)

			realmInfo := m.GetRealm()
			assert.Equal(t, tt.expectedSource, realmInfo.Source)
			assert.Equal(t, tt.expectedValue, realmInfo.Value)
		})
	}
}

// TestManager_GCP_ADC_ServiceAccountChainedIdentity verifies that a service account
// identity chained through another identity works correctly.
func TestManager_GCP_ADC_ServiceAccountChainedIdentity(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"gcp-adc": {
				Kind: "gcp/adc",
				Spec: map[string]any{
					"project_id": "base-project",
				},
			},
		},
		Identities: map[string]schema.Identity{
			"base-sa": {
				Kind: "gcp/service-account",
				Via:  &schema.IdentityVia{Provider: "gcp-adc"},
				Principal: map[string]any{
					"service_account_email": "base@base-project.iam.gserviceaccount.com",
				},
			},
			"target-sa": {
				Kind: "gcp/service-account",
				Via:  &schema.IdentityVia{Identity: "base-sa"},
				Principal: map[string]any{
					"service_account_email": "target@target-project.iam.gserviceaccount.com",
				},
			},
		},
	}, credStore, validator, nil, "")
	require.NoError(t, err)

	mgr := m.(*manager)

	// Build chain for target-sa: should be [gcp-adc, base-sa, target-sa].
	chain, err := mgr.buildAuthenticationChain("target-sa")
	require.NoError(t, err)
	require.Len(t, chain, 3)
	assert.Equal(t, "gcp-adc", chain[0])
	assert.Equal(t, "base-sa", chain[1])
	assert.Equal(t, "target-sa", chain[2])
}

// TestManager_GCP_ADC_GetFilesDisplayPath verifies that the ADC provider
// returns empty display path (no credential files managed).
func TestManager_GCP_ADC_GetFilesDisplayPath(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"my-adc": {
				Kind: "gcp/adc",
				Spec: map[string]any{
					"project_id": "test",
				},
			},
		},
		Identities: map[string]schema.Identity{},
	}, credStore, validator, nil, "")
	require.NoError(t, err)

	// ADC provider has no credential files to display.
	displayPath := m.GetFilesDisplayPath("my-adc")
	assert.Empty(t, displayPath, "ADC provider should have empty display path")
}

// TestManager_GCP_ADC_InvalidSpec verifies that NewAuthManager returns errors
// for invalid GCP ADC configurations.
func TestManager_GCP_ADC_InvalidSpec(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	// ADC provider with nil spec should fail.
	_, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"bad-adc": {
				Kind: "gcp/adc",
				// No Spec provided.
			},
		},
	}, credStore, validator, nil, "")
	require.Error(t, err, "ADC provider without spec should fail initialization")
}

// TestManager_GCP_ADC_WhoamiIdentityResolution verifies that identity
// name resolution works correctly for GCP identities (case-insensitive).
func TestManager_GCP_ADC_WhoamiIdentityResolution(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"adc": {
				Kind: "gcp/adc",
				Spec: map[string]any{
					"project_id": "test",
				},
			},
		},
		Identities: map[string]schema.Identity{
			"my-project": {
				Kind: "gcp/project",
				Via:  &schema.IdentityVia{Provider: "adc"},
				Principal: map[string]any{
					"project_id": "test",
				},
			},
		},
	}, credStore, validator, nil, "")
	require.NoError(t, err)

	// Whoami with non-existent identity should fail.
	ctx := types.WithSuppressAuthErrors(context.Background(), true)
	_, err = m.Whoami(ctx, "non-existent")
	require.Error(t, err)
}

// TestManager_GCP_ADC_ListProviders verifies that ListProviders includes
// the GCP ADC provider.
func TestManager_GCP_ADC_ListProviders(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"my-adc": {
				Kind: "gcp/adc",
				Spec: map[string]any{
					"project_id": "test",
				},
			},
		},
		Identities: map[string]schema.Identity{},
	}, credStore, validator, nil, "")
	require.NoError(t, err)

	providers := m.ListProviders()
	assert.Contains(t, providers, "my-adc")
}
