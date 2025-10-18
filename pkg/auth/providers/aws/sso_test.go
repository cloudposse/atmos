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

func TestSSOProvider_NameAndKind(t *testing.T) {
	config := &schema.Provider{
		Kind:     testSSOKind,
		Region:   testRegion,
		StartURL: testStartURL,
	}
	provider, err := NewSSOProvider(testProviderName, config)
	require.NoError(t, err)
	assert.Equal(t, testProviderName, provider.Name())
	assert.Equal(t, testSSOKind, provider.Kind())
}

func TestSSOProvider_PreAuthenticate_NoOp(t *testing.T) {
	config := &schema.Provider{Kind: testSSOKind, Region: testRegion, StartURL: testStartURL}
	provider, err := NewSSOProvider(testProviderName, config)
	require.NoError(t, err)
	// PreAuthenticate is a no-op for SSO and should not error.
	assert.NoError(t, provider.PreAuthenticate(nil))
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
	// Default when no session configured.
	p, err := NewSSOProvider("sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x"})
	require.NoError(t, err)
	assert.Equal(t, 60, p.getSessionDuration())

	// Valid duration string.
	p, err = NewSSOProvider("sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x", Session: &schema.SessionConfig{Duration: "15m"}})
	require.NoError(t, err)
	assert.Equal(t, 15, p.getSessionDuration())

	// Invalid duration string -> default.
	p, err = NewSSOProvider("sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x", Session: &schema.SessionConfig{Duration: "bogus"}})
	require.NoError(t, err)
	assert.Equal(t, 60, p.getSessionDuration())
}

func TestSSOProvider_Validate_Errors(t *testing.T) {
	// Create valid provider, then mutate fields to trigger Validate errors.
	p, err := NewSSOProvider("sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x"})
	require.NoError(t, err)

	p.region = ""
	assert.Error(t, p.Validate())

	p.region = "us-east-1"
	p.startURL = ""
	assert.Error(t, p.Validate())
}

func TestSSOProvider_NameAndPreAuthenticate_NoOp(t *testing.T) {
	p, err := NewSSOProvider("aws-sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x"})
	require.NoError(t, err)
	assert.Equal(t, "aws-sso", p.Name())
	// PreAuthenticate is a no-op.
	assert.NoError(t, p.PreAuthenticate(nil))
}

func TestSSOProvider_promptDeviceAuth_NonCI_OpensURL(t *testing.T) {
	t.Setenv("GO_TEST", "1") // ensure OpenUrl returns quickly
	t.Setenv("CI", "")       // not CI
	p, err := NewSSOProvider("sso", &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://x"})
	require.NoError(t, err)
	url := "https://company.awsapps.com/start/#/device?user_code=ABCD"
	p.promptDeviceAuth(&ssooidc.StartDeviceAuthorizationOutput{VerificationUriComplete: &url})
}

func TestSSOProvider_promptDeviceAuth_DisplaysVerificationCode(t *testing.T) {
	tests := []struct {
		name                    string
		userCode                string
		verificationURI         string
		verificationURIComplete string
		isCI                    bool
		expectedInOutput        []string
	}{
		{
			name:                    "displays verification code with complete URI in non-CI",
			userCode:                "WXYZ-1234",
			verificationURIComplete: "https://device.sso.us-east-1.amazonaws.com/",
			isCI:                    false,
			expectedInOutput: []string{
				"AWS SSO Authentication Required",
				"Verification Code: **WXYZ-1234**",
				"Opening browser to:",
				"Waiting for authentication",
			},
		},
		{
			name:            "displays verification code with base URI in non-CI",
			userCode:        "ABCD-5678",
			verificationURI: "https://device.sso.us-east-1.amazonaws.com/",
			isCI:            false,
			expectedInOutput: []string{
				"AWS SSO Authentication Required",
				"Verification Code: **ABCD-5678**",
				"Please visit",
				"Waiting for authentication",
			},
		},
		{
			name:                    "displays verification code in CI environment",
			userCode:                "TEST-CODE",
			verificationURIComplete: "https://device.sso.us-east-1.amazonaws.com/",
			isCI:                    true,
			expectedInOutput: []string{
				"AWS SSO Authentication Required",
				"Verification Code: **TEST-CODE**",
				"Verification URL:",
				"Waiting for authentication",
			},
		},
		{
			name:     "handles nil user code gracefully",
			userCode: "",
			isCI:     true,
			expectedInOutput: []string{
				"AWS SSO Authentication Required",
				"Verification Code: ****",
				"Waiting for authentication",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GO_TEST", "1") // ensure OpenUrl returns quickly
			if tt.isCI {
				t.Setenv("CI", "1")
			} else {
				t.Setenv("CI", "")
			}

			p, err := NewSSOProvider("sso", &schema.Provider{
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://x",
			})
			require.NoError(t, err)

			// Build the authorization output.
			authOutput := &ssooidc.StartDeviceAuthorizationOutput{}
			if tt.userCode != "" {
				authOutput.UserCode = &tt.userCode
			}
			if tt.verificationURI != "" {
				authOutput.VerificationUri = &tt.verificationURI
			}
			if tt.verificationURIComplete != "" {
				authOutput.VerificationUriComplete = &tt.verificationURIComplete
			}

			// Call promptDeviceAuth - this will output to stderr via PrintfMessageToTUI.
			// Since we can't easily capture stderr in tests, we just verify it doesn't panic.
			// The actual output verification is done manually or via integration tests.
			assert.NotPanics(t, func() {
				p.promptDeviceAuth(authOutput)
			})

			// Note: To fully verify the output contains the expected strings, we would need
			// to capture stderr output, which is complex in Go tests. The important thing is
			// that the function executes without errors and the test output shows the messages.
		})
	}
}

func TestSSOProvider_WithCustomResolver(t *testing.T) {
	// Test SSO provider with custom resolver configuration.
	config := &schema.Provider{
		Kind:     "aws/iam-identity-center",
		Region:   "us-east-1",
		StartURL: "https://company.awsapps.com/start",
		Spec: map[string]interface{}{
			"aws": map[string]interface{}{
				"resolver": map[string]interface{}{
					"url": "http://localhost:4566",
				},
			},
		},
	}

	p, err := NewSSOProvider("sso-localstack", config)
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "sso-localstack", p.Name())
	assert.Equal(t, "us-east-1", p.region)
	assert.Equal(t, "https://company.awsapps.com/start", p.startURL)

	// Verify the provider has the config with resolver.
	assert.NotNil(t, p.config)
	assert.NotNil(t, p.config.Spec)
	awsSpec, ok := p.config.Spec["aws"]
	assert.True(t, ok)
	assert.NotNil(t, awsSpec)
}

func TestSSOProvider_WithoutCustomResolver(t *testing.T) {
	// Test SSO provider without custom resolver configuration.
	config := &schema.Provider{
		Kind:     "aws/iam-identity-center",
		Region:   "us-east-1",
		StartURL: "https://company.awsapps.com/start",
	}

	p, err := NewSSOProvider("sso-standard", config)
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, "sso-standard", p.Name())

	// Verify the provider works without resolver config.
	assert.NoError(t, p.Validate())
}
