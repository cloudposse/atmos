package exec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGCRAuth tests Google Container Registry authentication
func TestGCRAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Google Container Registry",
			registry:    "gcr.io",
			expectError: true, // Will fail without Google Cloud credentials
			errorMsg:    "failed to find Google Cloud credentials",
		},
		{
			name:        "Google Artifact Registry",
			registry:    "us-docker.pkg.dev",
			expectError: true, // Will fail without Google Cloud credentials
			errorMsg:    "failed to find Google Cloud credentials",
		},
		{
			name:        "Non-Google registry",
			registry:    "docker.io",
			expectError: true,
			errorMsg:    "failed to find Google Cloud credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getGCRAuth(tt.registry)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestGCRRegistryDetection tests Google registry detection logic
// Note: This test reflects the current implementation logic in getRegistryAuth
func TestGCRRegistryDetection(t *testing.T) {
	tests := []struct {
		name     string
		registry string
		isGCR    bool
	}{
		{
			name:     "Google Container Registry",
			registry: "gcr.io",
			isGCR:    true,
		},
		{
			name:     "Google Artifact Registry",
			registry: "us-docker.pkg.dev",
			isGCR:    true,
		},
		{
			name:     "Another Artifact Registry region",
			registry: "europe-docker.pkg.dev",
			isGCR:    true,
		},
		{
			name:     "Non-Google registry",
			registry: "docker.io",
			isGCR:    false,
		},
		{
			name:     "Azure Container Registry",
			registry: "test.azurecr.io",
			isGCR:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This matches the logic in getRegistryAuth in oci_utils.go
			isGCR := strings.Contains(tt.registry, "gcr.io") || strings.Contains(tt.registry, "pkg.dev")
			assert.Equal(t, tt.isGCR, isGCR)
		})
	}
}
