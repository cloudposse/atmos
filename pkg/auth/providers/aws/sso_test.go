package aws

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
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
		// Note: provider name is not validated by NewSSOProvider, so empty name is allowed.
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
	// Prevent browser launch during device auth flow and shorten network timeouts.
	t.Setenv("GO_TEST", "1") // utils.OpenUrl early-exits when set.
	t.Setenv("CI", "1")      // promptDeviceAuth avoids opening in CI.

	config := &schema.Provider{
		Kind:     testSSOKind,
		Region:   testRegion,
		StartURL: testStartURL,
	}

	provider, err := NewSSOProvider(testProviderName, config)
	require.NoError(t, err)

	// Use short timeout so SDK calls fail fast in tests.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err = provider.Authenticate(ctx)

	// We expect this to fail in test environment without proper SSO setup.
	assert.Error(t, err)
}

func TestSSOProvider_promptDeviceAuth_SafeInCI(t *testing.T) {
	t.Setenv("GO_TEST", "1")
	t.Setenv("CI", "1")
	p, err := NewSSOProvider("sso", &schema.Provider{Kind: testSSOKind, Region: testRegion, StartURL: testStartURL})
	require.NoError(t, err)
	// With a full verification URL, OpenUrl is skipped under GO_TEST and CI.
	url := "https://company.awsapps.com/start/#/device?user_code=WDDD-HRQV"
	p.promptDeviceAuth(&ssooidc.StartDeviceAuthorizationOutput{VerificationUriComplete: &url})
}

func TestSSOProvider_promptDeviceAuth_NilURL(t *testing.T) {
	t.Setenv("GO_TEST", "1")
	t.Setenv("CI", "1")
	p, err := NewSSOProvider("sso", &schema.Provider{Kind: testSSOKind, Region: testRegion, StartURL: testStartURL})
	require.NoError(t, err)
	// Nil URL should be safe and no-op.
	p.promptDeviceAuth(&ssooidc.StartDeviceAuthorizationOutput{})
}

func TestSSOProvider_getSessionDuration(t *testing.T) {
	// Default when no session configured
	p, err := NewSSOProvider("sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x"})
	require.NoError(t, err)
	assert.Equal(t, 60, p.getSessionDuration())

	// Valid duration string
	p, err = NewSSOProvider("sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x", Session: &schema.SessionConfig{Duration: "15m"}})
	require.NoError(t, err)
	assert.Equal(t, 15, p.getSessionDuration())

	// Invalid duration string -> default
	p, err = NewSSOProvider("sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x", Session: &schema.SessionConfig{Duration: "bogus"}})
	require.NoError(t, err)
	assert.Equal(t, 60, p.getSessionDuration())
}

func TestSSOProvider_Validate_Errors(t *testing.T) {
	// Create valid provider, then mutate fields to trigger Validate errors
	p, err := NewSSOProvider("sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x"})
	require.NoError(t, err)

	p.region = ""
	assert.Error(t, p.Validate())

	p.region = "us-east-1"
	p.startURL = ""
	assert.Error(t, p.Validate())
}
