package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewStoreRegistry_SSMWithIdentity(t *testing.T) {
	config := &StoresConfig{
		"prod-ssm": StoreConfig{
			Type:     "aws-ssm-parameter-store",
			Identity: "prod-admin",
			Options:  map[string]interface{}{"region": "us-east-1"},
		},
	}

	registry, err := NewStoreRegistry(config)
	assert.NoError(t, err)
	assert.Len(t, registry, 1)

	// Verify SSM store was created with identity and deferred client init.
	ssmStore, ok := registry["prod-ssm"].(*SSMStore)
	assert.True(t, ok)
	assert.Equal(t, "prod-admin", ssmStore.identityName)
	assert.Nil(t, ssmStore.client) // Lazy init — no client created yet.
}

func TestNewStoreRegistry_AzureWithIdentity(t *testing.T) {
	config := &StoresConfig{
		"prod-azure": StoreConfig{
			Type:     "azure-key-vault",
			Identity: "azure-prod",
			Options:  map[string]interface{}{"vault_url": "https://prod.vault.azure.net"},
		},
	}

	registry, err := NewStoreRegistry(config)
	assert.NoError(t, err)
	assert.Len(t, registry, 1)

	// Verify Azure store was created with identity and deferred client init.
	azStore, ok := registry["prod-azure"].(*AzureKeyVaultStore)
	assert.True(t, ok)
	assert.Equal(t, "azure-prod", azStore.identityName)
	assert.Nil(t, azStore.client) // Lazy init — no client created yet.
}

func TestNewStoreRegistry_GSMWithIdentity(t *testing.T) {
	config := &StoresConfig{
		"prod-gsm": StoreConfig{
			Type:     "google-secret-manager",
			Identity: "gcp-prod",
			Options:  map[string]interface{}{"project_id": "my-project"},
		},
	}

	registry, err := NewStoreRegistry(config)
	assert.NoError(t, err)
	assert.Len(t, registry, 1)

	// Verify GSM store was created with identity and deferred client init.
	gsmStore, ok := registry["prod-gsm"].(*GSMStore)
	assert.True(t, ok)
	assert.Equal(t, "gcp-prod", gsmStore.identityName)
	assert.Nil(t, gsmStore.client) // Lazy init — no client created yet.
}

func TestNewStoreRegistry_RedisWithIdentityWarning(t *testing.T) {
	config := &StoresConfig{
		"cache": StoreConfig{
			Type:     "redis",
			Identity: "prod-admin",
			Options:  map[string]interface{}{"url": "redis://localhost:6379"},
		},
	}

	// Redis stores log a warning for identity (unsupported) but still create successfully.
	registry, err := NewStoreRegistry(config)
	assert.NoError(t, err)
	assert.Len(t, registry, 1)

	// Verify the store is created (identity is ignored by Redis).
	_, ok := registry["cache"].(*RedisStore)
	assert.True(t, ok)
}

func TestNewStoreRegistry_ArtifactoryWithIdentityWarning(t *testing.T) {
	config := &StoresConfig{
		"artifacts": StoreConfig{
			Type:     "artifactory",
			Identity: "prod-admin",
			Options: map[string]interface{}{
				"access_token": "anonymous",
				"url":          "https://example.jfrog.io/artifactory",
				"repo_name":    "test-repo",
			},
		},
	}

	// Artifactory stores log a warning for identity (unsupported) but still create successfully.
	registry, err := NewStoreRegistry(config)
	assert.NoError(t, err)
	assert.Len(t, registry, 1)

	_, ok := registry["artifacts"].(*ArtifactoryStore)
	assert.True(t, ok)
}

func TestNewStoreRegistry_GSMAliasWithIdentity(t *testing.T) {
	config := &StoresConfig{
		"prod-gsm": StoreConfig{
			Type:     "gsm", // Test the alias.
			Identity: "gcp-prod",
			Options:  map[string]interface{}{"project_id": "my-project"},
		},
	}

	registry, err := NewStoreRegistry(config)
	assert.NoError(t, err)
	assert.Len(t, registry, 1)

	gsmStore, ok := registry["prod-gsm"].(*GSMStore)
	assert.True(t, ok)
	assert.Equal(t, "gcp-prod", gsmStore.identityName)
	assert.Nil(t, gsmStore.client)
}

func TestNewStoreRegistry_MixedIdentityStores(t *testing.T) {
	config := &StoresConfig{
		"identity-ssm": StoreConfig{
			Type:     "aws-ssm-parameter-store",
			Identity: "prod-admin",
			Options:  map[string]interface{}{"region": "us-east-1"},
		},
		"identity-azure": StoreConfig{
			Type:     "azure-key-vault",
			Identity: "azure-prod",
			Options:  map[string]interface{}{"vault_url": "https://vault.azure.net"},
		},
		"identity-gsm": StoreConfig{
			Type:     "google-secret-manager",
			Identity: "gcp-prod",
			Options:  map[string]interface{}{"project_id": "my-project"},
		},
	}

	registry, err := NewStoreRegistry(config)
	assert.NoError(t, err)
	assert.Len(t, registry, 3)

	// All stores should implement IdentityAwareStore and have nil clients.
	for name, s := range registry {
		ias, ok := s.(IdentityAwareStore)
		assert.True(t, ok, "store %q should implement IdentityAwareStore", name)
		_ = ias
	}
}
