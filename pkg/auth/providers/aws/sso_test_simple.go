package aws

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	testSSOKind                = "aws/iam-identity-center"
	testRegion                 = "us-east-1"
	testStartURL               = "https://company.awsapps.com/start"
	testProviderName           = "aws-sso"
	testErrorMsgRequiredConfig = "provider config is required"
	testErrorMsgRequiredName   = "provider name is required"
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
			providerName: testProviderName,
			config: &schema.Provider{
				Kind:     testSSOKind,
				Region:   testRegion,
				StartURL: testStartURL,
			},
			expectError: false,
		},
		{
			name:         "nil config",
			providerName: testProviderName,
			config:       nil,
			expectError:  true,
			errorMsg:     testErrorMsgRequiredConfig,
		},
		{
			name:         "empty name",
			providerName: "",
			config: &schema.Provider{
				Kind:     testSSOKind,
				Region:   testRegion,
				StartURL: testStartURL,
			},
			expectError: true,
			errorMsg:    testErrorMsgRequiredName,
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
				assert.Equal(t, testSSOKind, provider.Kind())
			}
		})
	}
}

func TestSSOProvider_Validate_Simple(t *testing.T) {
	config := &schema.Provider{
		Kind:     testSSOKind,
		Region:   testRegion,
		StartURL: testStartURL,
	}

	provider, err := NewSSOProvider(testProviderName, config)
	require.NoError(t, err)

	err = provider.Validate()
	assert.NoError(t, err)
}

func TestSSOProvider_Environment_Simple(t *testing.T) {
	config := &schema.Provider{
		Kind:     testSSOKind,
		Region:   testRegion,
		StartURL: testStartURL,
	}

	provider, err := NewSSOProvider(testProviderName, config)
	require.NoError(t, err)

	env, err := provider.Environment()
	assert.NoError(t, err)
	assert.NotNil(t, env)
	assert.Equal(t, testRegion, env["AWS_REGION"])
}

func TestSSOProvider_Authenticate_Simple(t *testing.T) {
	config := &schema.Provider{
		Kind:     testSSOKind,
		Region:   testRegion,
		StartURL: testStartURL,
	}

	provider, err := NewSSOProvider(testProviderName, config)
	require.NoError(t, err)

	// Note: This would fail without proper AWS SSO setup
	// In a real test, we'd mock the AWS SDK clients
	ctx := context.Background()
	_, err = provider.Authenticate(ctx)

	// We expect this to fail in test environment without proper SSO setup
	assert.Error(t, err)
}
