package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestBuildRegistryURL(t *testing.T) {
	tests := []struct {
		name      string
		accountID string
		region    string
		expected  string
	}{
		{
			name:      "basic ECR URL",
			accountID: "123456789012",
			region:    "us-east-1",
			expected:  "123456789012.dkr.ecr.us-east-1.amazonaws.com",
		},
		{
			name:      "different region",
			accountID: "999999999999",
			region:    "eu-west-1",
			expected:  "999999999999.dkr.ecr.eu-west-1.amazonaws.com",
		},
		{
			name:      "gov-cloud region",
			accountID: "123456789012",
			region:    "us-gov-west-1",
			expected:  "123456789012.dkr.ecr.us-gov-west-1.amazonaws.com",
		},
		{
			name:      "ap region",
			accountID: "123456789012",
			region:    "ap-southeast-1",
			expected:  "123456789012.dkr.ecr.ap-southeast-1.amazonaws.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildRegistryURL(tt.accountID, tt.region)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseRegistryURL(t *testing.T) {
	tests := []struct {
		name              string
		registryURL       string
		expectedAccountID string
		expectedRegion    string
		expectError       bool
	}{
		{
			name:              "standard ECR URL",
			registryURL:       "123456789012.dkr.ecr.us-east-1.amazonaws.com",
			expectedAccountID: "123456789012",
			expectedRegion:    "us-east-1",
			expectError:       false,
		},
		{
			name:              "ECR URL with https prefix",
			registryURL:       "https://123456789012.dkr.ecr.us-west-2.amazonaws.com",
			expectedAccountID: "123456789012",
			expectedRegion:    "us-west-2",
			expectError:       false,
		},
		{
			name:              "gov-cloud region",
			registryURL:       "123456789012.dkr.ecr.us-gov-west-1.amazonaws.com",
			expectedAccountID: "123456789012",
			expectedRegion:    "us-gov-west-1",
			expectError:       false,
		},
		{
			name:              "invalid URL - not ECR format",
			registryURL:       "docker.io/library/nginx",
			expectedAccountID: "",
			expectedRegion:    "",
			expectError:       true,
		},
		{
			name:              "invalid URL - missing account ID",
			registryURL:       "dkr.ecr.us-east-1.amazonaws.com",
			expectedAccountID: "",
			expectedRegion:    "",
			expectError:       true,
		},
		{
			name:              "invalid URL - empty string",
			registryURL:       "",
			expectedAccountID: "",
			expectedRegion:    "",
			expectError:       true,
		},
		{
			name:              "invalid URL - trailing slash not supported",
			registryURL:       "123456789012.dkr.ecr.us-east-1.amazonaws.com/",
			expectedAccountID: "",
			expectedRegion:    "",
			expectError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accountID, region, err := ParseRegistryURL(tt.registryURL)

			if tt.expectError {
				assert.ErrorIs(t, err, errUtils.ErrECRInvalidRegistry)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAccountID, accountID)
				assert.Equal(t, tt.expectedRegion, region)
			}
		})
	}
}

