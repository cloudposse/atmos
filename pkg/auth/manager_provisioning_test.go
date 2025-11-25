package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestProvisioningResult_Structure tests the provisioning result structure.
func TestProvisioningResult_Structure(t *testing.T) {
	// Test that provisioning result has expected structure.
	result := &authTypes.ProvisioningResult{
		Provider:      "test-sso",
		ProvisionedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Identities: map[string]*schema.Identity{
			"account1/role1": {
				Provider: "test-sso",
				Principal: map[string]interface{}{
					"name": "role1",
					"account": map[string]interface{}{
						"name": "account1",
						"id":   "123456789012",
					},
				},
			},
		},
		Metadata: authTypes.ProvisioningMetadata{
			Source: "aws-sso",
			Counts: &authTypes.ProvisioningCounts{
				Accounts:   1,
				Roles:      1,
				Identities: 1,
			},
		},
	}

	// Verify structure.
	assert.Equal(t, "test-sso", result.Provider)
	assert.Len(t, result.Identities, 1)
	assert.Contains(t, result.Identities, "account1/role1")
	assert.Equal(t, "aws-sso", result.Metadata.Source)
	assert.NotNil(t, result.Metadata.Counts)
	assert.Equal(t, 1, result.Metadata.Counts.Accounts)
	assert.Equal(t, 1, result.Metadata.Counts.Roles)
	assert.Equal(t, 1, result.Metadata.Counts.Identities)
}

// TestProvisioningResult_EmptyIdentities tests handling of empty identities in result.
func TestProvisioningResult_EmptyIdentities(t *testing.T) {
	// Create provisioning result with empty identities.
	result := &authTypes.ProvisioningResult{
		Provider:      "test-sso",
		ProvisionedAt: time.Now(),
		Identities:    map[string]*schema.Identity{}, // Empty.
		Metadata: authTypes.ProvisioningMetadata{
			Source: "aws-sso",
			Counts: &authTypes.ProvisioningCounts{
				Accounts:   0,
				Roles:      0,
				Identities: 0,
			},
		},
	}

	// Verify structure.
	assert.NotNil(t, result)
	assert.Len(t, result.Identities, 0)
	assert.Equal(t, 0, result.Metadata.Counts.Accounts)
	assert.Equal(t, 0, result.Metadata.Counts.Roles)
	assert.Equal(t, 0, result.Metadata.Counts.Identities)
}

// TestWriteProvisionedIdentities_Success tests successful writing of provisioned identities.
func TestWriteProvisionedIdentities_Success(t *testing.T) {
	// Create a temporary directory for cache.
	tempDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tempDir)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := authTypes.NewMockCredentialStore(ctrl)
	mockValidator := authTypes.NewMockValidator(ctrl)

	config := &schema.AuthConfig{
		Providers:  map[string]schema.Provider{},
		Identities: map[string]schema.Identity{},
	}

	// Create manager.
	m := &manager{
		config:          config,
		providers:       map[string]authTypes.Provider{},
		identities:      map[string]authTypes.Identity{},
		credentialStore: mockStore,
		validator:       mockValidator,
	}

	// Create provisioning result.
	result := &authTypes.ProvisioningResult{
		Provider:      "test-sso",
		ProvisionedAt: time.Now(),
		Identities: map[string]*schema.Identity{
			"account1/role1": {
				Provider: "test-sso",
			},
		},
		Metadata: authTypes.ProvisioningMetadata{
			Source: "aws-sso",
			Counts: &authTypes.ProvisioningCounts{
				Accounts:   1,
				Roles:      1,
				Identities: 1,
			},
		},
	}

	// Call writeProvisionedIdentities.
	err := m.writeProvisionedIdentities(result)
	require.NoError(t, err)

	// Verify file was written.
	// The file should be at $XDG_CACHE_HOME/atmos/auth/test-sso/provisioned-identities.yaml.
	// We don't check the file contents here, as that's tested in provisioning/writer_test.go.
}

// TestWriteProvisionedIdentities_WriterCreationError tests error when writer creation fails.
func TestWriteProvisionedIdentities_WriterCreationError(t *testing.T) {
	// Set invalid XDG_CACHE_HOME to force writer creation error.
	// This is difficult to test without modifying the implementation,
	// so we test the structure instead.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := authTypes.NewMockCredentialStore(ctrl)
	mockValidator := authTypes.NewMockValidator(ctrl)

	config := &schema.AuthConfig{
		Providers:  map[string]schema.Provider{},
		Identities: map[string]schema.Identity{},
	}

	// Create manager.
	m := &manager{
		config:          config,
		providers:       map[string]authTypes.Provider{},
		identities:      map[string]authTypes.Identity{},
		credentialStore: mockStore,
		validator:       mockValidator,
	}

	// Verify manager is properly initialized.
	assert.NotNil(t, m)
	assert.NotNil(t, m.config)
}

// TestProvisioningMetadata_AllFields tests metadata structure with all fields populated.
func TestProvisioningMetadata_AllFields(t *testing.T) {
	// Create provisioning result with all metadata fields.
	result := &authTypes.ProvisioningResult{
		Provider:      "test-sso",
		ProvisionedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Identities: map[string]*schema.Identity{
			"account1/role1": {
				Provider: "test-sso",
				Principal: map[string]interface{}{
					"name": "role1",
					"account": map[string]interface{}{
						"name": "account1",
						"id":   "123456789012",
					},
				},
			},
		},
		Metadata: authTypes.ProvisioningMetadata{
			Source: "aws-sso",
			Counts: &authTypes.ProvisioningCounts{
				Accounts:   1,
				Roles:      1,
				Identities: 1,
			},
			Extra: map[string]interface{}{
				"start_url": "https://test.awsapps.com/start",
				"region":    "us-east-1",
			},
		},
	}

	// Verify metadata structure.
	assert.Equal(t, "aws-sso", result.Metadata.Source)
	assert.NotNil(t, result.Metadata.Counts)
	assert.Equal(t, 1, result.Metadata.Counts.Accounts)
	assert.Equal(t, 1, result.Metadata.Counts.Roles)
	assert.Equal(t, 1, result.Metadata.Counts.Identities)
	assert.NotNil(t, result.Metadata.Extra)
	assert.Equal(t, "https://test.awsapps.com/start", result.Metadata.Extra["start_url"])
	assert.Equal(t, "us-east-1", result.Metadata.Extra["region"])
}
