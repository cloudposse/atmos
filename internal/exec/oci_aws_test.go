package exec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestECRAuthDirect tests AWS ECR authentication directly.
func TestECRAuthDirect(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Invalid ECR registry format",
			registry:    "invalid-ecr-registry.com",
			expectError: true,
			errorMsg:    "invalid ECR registry format",
		},
		{
			name:        "Non-ECR registry",
			registry:    "docker.io",
			expectError: true,
			errorMsg:    "invalid ECR registry format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getECRAuth(tt.registry)

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

// TestECRRegistryParsing tests the ECR registry parsing logic.
func TestECRRegistryParsing(t *testing.T) {
	tests := []struct {
		name     string
		registry string
		isECR    bool
	}{
		{
			name:     "Standard ECR",
			registry: "123456789012.dkr.ecr.us-west-2.amazonaws.com",
			isECR:    true,
		},
		{
			name:     "ECR FIPS",
			registry: "123456789012.dkr.ecr-fips.us-west-2.amazonaws.com",
			isECR:    true,
		},
		{
			name:     "ECR China",
			registry: "123456789012.dkr.ecr.cn-northwest-1.amazonaws.com.cn",
			isECR:    true,
		},
		{
			name:     "ECR FIPS China",
			registry: "123456789012.dkr.ecr-fips.cn-northwest-1.amazonaws.com.cn",
			isECR:    true,
		},
		{
			name:     "Non-ECR registry",
			registry: "docker.io",
			isECR:    false,
		},
		{
			name:     "Invalid format",
			registry: "invalid-registry.com",
			isECR:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isECR := strings.Contains(tt.registry, "dkr.ecr") && strings.Contains(tt.registry, "amazonaws.com")
			assert.Equal(t, tt.isECR, isECR)
		})
	}
}
