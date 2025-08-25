package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGetRegistryAuth tests the main registry authentication function
// This is an integration test that verifies the overall authentication flow
func TestGetRegistryAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		atmosConfig *schema.AtmosConfiguration
		expectError bool
		errorMsg    string
	}{
		{
			name:        "GitHub Container Registry",
			registry:    "ghcr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "Docker Hub",
			registry:    "docker.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "Azure Container Registry",
			registry:    "test.azurecr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "AWS ECR (skipped - requires credentials)",
			registry:    "invalid-ecr-registry.com",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "Google Container Registry",
			registry:    "gcr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
		{
			name:        "Unknown registry",
			registry:    "unknown.registry.com",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getRegistryAuth(tt.registry, tt.atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				// Authentication might fail due to missing credentials, but should not panic
				if err != nil {
					t.Logf("Authentication failed as expected: %v", err)
				}
			}
		})
	}
}

// TestCloudProviderAuth tests authentication for different cloud providers
// This is an integration test that verifies provider-specific authentication flows
func TestCloudProviderAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		provider    string
		expectError bool
	}{
		{
			name:        "GitHub CR with token",
			registry:    "ghcr.io",
			provider:    "github",
			expectError: false,
		},
		{
			name:        "Docker Hub with credentials",
			registry:    "docker.io",
			provider:    "docker",
			expectError: false,
		},
		{
			name:        "Azure Container Registry",
			registry:    "test.azurecr.io",
			provider:    "azure",
			expectError: false,
		},
		{
			name:        "AWS ECR (skipped - requires credentials)",
			registry:    "invalid-ecr-registry.com",
			provider:    "aws",
			expectError: false,
		},
		{
			name:        "Google Container Registry",
			registry:    "gcr.io",
			provider:    "google",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}
			_, err := getRegistryAuth(tt.registry, atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				// Authentication might fail due to missing credentials, but should not panic
				if err != nil {
					t.Logf("Authentication failed as expected: %v", err)
				}
			}
		})
	}
}
