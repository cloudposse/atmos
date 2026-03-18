package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestECRPublicConstants(t *testing.T) {
	assert.Equal(t, "public.ecr.aws", ECRPublicRegistryURL)
	assert.Equal(t, "us-east-1", ECRPublicAuthRegion)
}

func TestValidateECRPublicRegion(t *testing.T) {
	tests := []struct {
		name        string
		region      string
		expectError bool
	}{
		{
			name:        "us-east-1 is valid",
			region:      "us-east-1",
			expectError: false,
		},
		{
			name:        "us-west-2 is valid",
			region:      "us-west-2",
			expectError: false,
		},
		{
			name:        "eu-west-1 is invalid",
			region:      "eu-west-1",
			expectError: true,
		},
		{
			name:        "ap-southeast-1 is invalid",
			region:      "ap-southeast-1",
			expectError: true,
		},
		{
			name:        "cn-north-1 is invalid",
			region:      "cn-north-1",
			expectError: true,
		},
		{
			name:        "us-gov-west-1 is invalid",
			region:      "us-gov-west-1",
			expectError: true,
		},
		{
			name:        "empty string is invalid",
			region:      "",
			expectError: true,
		},
		{
			name:        "us-east-2 is invalid",
			region:      "us-east-2",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateECRPublicRegion(tt.region)
			if tt.expectError {
				assert.ErrorIs(t, err, errUtils.ErrECRPublicInvalidRegion)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsECRPublicRegistry(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "exact match",
			url:      "public.ecr.aws",
			expected: true,
		},
		{
			name:     "with https prefix",
			url:      "https://public.ecr.aws",
			expected: true,
		},
		{
			name:     "with path",
			url:      "public.ecr.aws/cloudposse/atmos",
			expected: true,
		},
		{
			name:     "with https and path",
			url:      "https://public.ecr.aws/cloudposse/atmos",
			expected: true,
		},
		{
			name:     "private ECR registry",
			url:      "123456789012.dkr.ecr.us-east-1.amazonaws.com",
			expected: false,
		},
		{
			name:     "Docker Hub",
			url:      "docker.io/library/nginx",
			expected: false,
		},
		{
			name:     "GitHub Container Registry",
			url:      "ghcr.io/owner/repo",
			expected: false,
		},
		{
			name:     "empty string",
			url:      "",
			expected: false,
		},
		{
			name:     "partial match - wrong prefix",
			url:      "notpublic.ecr.aws",
			expected: false,
		},
		{
			name:     "partial match - different domain",
			url:      "public.ecr.aws.evil.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsECRPublicRegistry(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}
