package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestECRAuthDirect tests AWS ECR authentication directly.
func TestECRAuthDirect(t *testing.T) {
	t.Parallel()
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
	tests := []struct {
		name        string
		registry    string
		wantAcct    string
		wantRegion  string
		expectError bool
	}{
		{"Standard ECR", "123456789012.dkr.ecr.us-west-2.amazonaws.com", "123456789012", "us-west-2", false},
		{"ECR FIPS", "123456789012.dkr.ecr-fips.us-west-2.amazonaws.com", "123456789012", "us-west-2", false},
		{"ECR China", "123456789012.dkr.ecr.cn-northwest-1.amazonaws.com.cn", "123456789012", "cn-northwest-1", false},
		{"ECR FIPS China", "123456789012.dkr.ecr-fips.cn-northwest-1.amazonaws.com.cn", "123456789012", "cn-northwest-1", false},
		{"Non-ECR registry", "docker.io", "", "", true},
		{"Invalid format", "invalid-registry.com", "", "", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			acct, region, err := parseECRRegistry(tt.registry)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantAcct, acct)
			assert.Equal(t, tt.wantRegion, region)
		})
	}
}
