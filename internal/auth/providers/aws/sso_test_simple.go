package aws

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewSSOProvider_Simple(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		config       *schema.Provider
		expectError  bool
		errorMsg     string
	}{
		{
			name:         "valid config",
			providerName: "aws-sso",
			config: &schema.Provider{
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://company.awsapps.com/start",
			},
			expectError: false,
		},
		{
			name:         "nil config",
			providerName: "aws-sso",
			config:       nil,
			expectError:  true,
			errorMsg:     "provider config is required",
		},
		{
			name:         "empty name",
			providerName: "",
			config: &schema.Provider{
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://company.awsapps.com/start",
			},
			expectError: true,
			errorMsg:    "provider name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewSSOProvider(tt.providerName, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				assert.Equal(t, "aws/iam-identity-center", provider.Kind())
			}
		})
	}
}

func TestSSOProvider_Validate_Simple(t *testing.T) {
	config := &schema.Provider{
		Kind:     "aws/iam-identity-center",
		Region:   "us-east-1",
		StartURL: "https://company.awsapps.com/start",
	}

	provider, err := NewSSOProvider("aws-sso", config)
	require.NoError(t, err)

	err = provider.Validate()
	assert.NoError(t, err)
}

func TestSSOProvider_Environment_Simple(t *testing.T) {
	config := &schema.Provider{
		Kind:     "aws/iam-identity-center",
		Region:   "us-east-1",
		StartURL: "https://company.awsapps.com/start",
	}

	provider, err := NewSSOProvider("aws-sso", config)
	require.NoError(t, err)

	env, err := provider.Environment()
	assert.NoError(t, err)
	assert.NotNil(t, env)
	assert.Equal(t, "us-east-1", env["AWS_REGION"])
}

func TestSSOProvider_Authenticate_Simple(t *testing.T) {
	config := &schema.Provider{
		Kind:     "aws/iam-identity-center",
		Region:   "us-east-1",
		StartURL: "https://company.awsapps.com/start",
	}

	provider, err := NewSSOProvider("aws-sso", config)
	require.NoError(t, err)

	// Note: This would fail without proper AWS SSO setup
	// In a real test, we'd mock the AWS SDK clients
	ctx := context.Background()
	_, err = provider.Authenticate(ctx)

	// We expect this to fail in test environment without proper SSO setup
	assert.Error(t, err)
}
