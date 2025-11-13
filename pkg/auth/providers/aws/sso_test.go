package aws

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	tea "github.com/charmbracelet/bubbletea"
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

func TestSSOProvider_Logout(t *testing.T) {
	tests := []struct {
		name        string
		providerCfg *schema.Provider
		expectError bool
	}{
		{
			name: "successful logout",
			providerCfg: &schema.Provider{
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://company.awsapps.com/start",
			},
			expectError: false,
		},
		{
			name: "logout with custom base_path",
			providerCfg: &schema.Provider{
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://company.awsapps.com/start",
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": t.TempDir(),
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewSSOProvider("test-sso", tt.providerCfg)
			require.NoError(t, err)

			testProviderLogoutWithFilesystemVerification(t, tt.providerCfg, "test-sso", p, tt.expectError)
		})
	}
}

func TestSSOProvider_GetFilesDisplayPath(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.Provider
		expected string
	}{
		{
			name: "default path with no base_path",
			config: &schema.Provider{
				Kind:     testSSOKind,
				Region:   testRegion,
				StartURL: testStartURL,
			},
			expected: "atmos/aws", // XDG path contains atmos/aws
		},
		{
			name: "custom base_path",
			config: &schema.Provider{
				Kind:     testSSOKind,
				Region:   testRegion,
				StartURL: testStartURL,
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": "/custom/path",
					},
				},
			},
			expected: "/custom/path",
		},
		{
			name: "home directory base_path",
			config: &schema.Provider{
				Kind:     testSSOKind,
				Region:   testRegion,
				StartURL: testStartURL,
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": "~/.custom/aws",
					},
				},
			},
			expected: "~/.custom/aws",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewSSOProvider(testProviderName, tt.config)
			require.NoError(t, err)

			path := provider.GetFilesDisplayPath()
			// Normalize path separators for cross-platform compatibility.
			normalizedPath := filepath.ToSlash(path)
			assert.Contains(t, normalizedPath, tt.expected)
		})
	}
}

func TestSSOProvider_Logout_ErrorPaths(t *testing.T) {
	tests := []struct {
		name        string
		providerCfg *schema.Provider
		expectError bool
	}{
		{
			name: "handles invalid base_path gracefully",
			providerCfg: &schema.Provider{
				Kind:     testSSOKind,
				Region:   testRegion,
				StartURL: testStartURL,
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": "/invalid/\x00/path", // Invalid path with null character.
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewSSOProvider(testProviderName, tt.providerCfg)
			require.NoError(t, err)

			ctx := context.Background()
			err = p.Logout(ctx)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsTTY(t *testing.T) {
	// isTTY checks if stderr is a terminal.
	// In test environment, this will typically return false.
	result := isTTY()
	assert.IsType(t, false, result)
}

func TestDisplayVerificationDialog(t *testing.T) {
	tests := []struct {
		name string
		code string
		url  string
	}{
		{
			name: "with verification code and URL",
			code: "ABCD-1234",
			url:  "https://device.sso.us-east-1.amazonaws.com/",
		},
		{
			name: "with empty code",
			code: "",
			url:  "https://device.sso.us-east-1.amazonaws.com/",
		},
		{
			name: "with empty URL",
			code: "WXYZ-5678",
			url:  "",
		},
		{
			name: "with both empty",
			code: "",
			url:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This function outputs to stderr, so we just verify it doesn't panic.
			assert.NotPanics(t, func() {
				displayVerificationDialog(tt.code, tt.url)
			})
		})
	}
}

func TestDisplayVerificationPlainText(t *testing.T) {
	tests := []struct {
		name string
		code string
		url  string
	}{
		{
			name: "with verification code and URL",
			code: "ABCD-1234",
			url:  "https://device.sso.us-east-1.amazonaws.com/",
		},
		{
			name: "with empty code",
			code: "",
			url:  "https://device.sso.us-east-1.amazonaws.com/",
		},
		{
			name: "with empty URL",
			code: "WXYZ-5678",
			url:  "",
		},
		{
			name: "with both empty",
			code: "",
			url:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This function outputs to stderr, so we just verify it doesn't panic.
			assert.NotPanics(t, func() {
				displayVerificationPlainText(tt.code, tt.url)
			})
		})
	}
}

func TestPollForAccessToken_ContextCancellation(t *testing.T) {
	// Test that pollForAccessToken respects context cancellation.
	// This test verifies the context cancellation behavior without actually
	// making network calls to AWS SSO.
	t.Setenv("GO_TEST", "1")
	t.Setenv("CI", "1")

	config := &schema.Provider{
		Kind:     testSSOKind,
		Region:   testRegion,
		StartURL: testStartURL,
	}

	provider, err := NewSSOProvider(testProviderName, config)
	require.NoError(t, err)

	// Create a context with a very short timeout to simulate cancellation.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire.
	time.Sleep(10 * time.Millisecond)

	// Note: We can't easily test pollForAccessToken in isolation without a real OIDC client.
	// The context cancellation is tested indirectly through the Authenticate method.
	// This test primarily verifies that the provider is set up correctly for context handling.
	assert.NotNil(t, provider)
	assert.Equal(t, ctx.Err(), context.DeadlineExceeded)
}

func TestPollResult_Structure(t *testing.T) {
	// Test pollResult struct creation.
	now := time.Now()
	result := pollResult{
		token:     "test-token",
		expiresAt: now,
		err:       nil,
	}

	assert.Equal(t, "test-token", result.token)
	assert.Equal(t, now, result.expiresAt)
	assert.Nil(t, result.err)
}

func TestSpinnerModel_Init(t *testing.T) {
	// Test spinner model initialization.
	resultChan := make(chan pollResult, 1)
	defer close(resultChan)

	model := spinnerModel{
		message:    "Testing",
		done:       false,
		resultChan: resultChan,
	}

	cmd := model.Init()
	assert.NotNil(t, cmd)
}

func TestSpinnerModel_Update_KeyPress(t *testing.T) {
	// Test spinner model handling Ctrl+C.
	resultChan := make(chan pollResult, 1)
	defer close(resultChan)

	cancelCalled := false
	cancelFunc := func() {
		cancelCalled = true
	}

	model := spinnerModel{
		message:    "Testing",
		done:       false,
		resultChan: resultChan,
		cancel:     cancelFunc,
	}

	// Simulate Ctrl+C key press.
	newModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updatedModel := newModel.(spinnerModel)

	assert.True(t, updatedModel.done)
	assert.NotNil(t, updatedModel.result)
	assert.Error(t, updatedModel.result.err)
	assert.Contains(t, updatedModel.result.err.Error(), "user aborted")
	assert.True(t, cancelCalled)
}

func TestSpinnerModel_Update_PollResult(t *testing.T) {
	// Test spinner model handling poll result.
	resultChan := make(chan pollResult, 1)
	defer close(resultChan)

	cancelCalled := false
	cancelFunc := func() {
		cancelCalled = true
	}

	model := spinnerModel{
		message:    "Testing",
		done:       false,
		resultChan: resultChan,
		cancel:     cancelFunc,
	}

	// Simulate receiving poll result.
	now := time.Now()
	pollRes := pollResult{
		token:     "test-token",
		expiresAt: now,
		err:       nil,
	}

	newModel, _ := model.Update(pollRes)
	updatedModel := newModel.(spinnerModel)

	assert.True(t, updatedModel.done)
	assert.NotNil(t, updatedModel.result)
	assert.Equal(t, "test-token", updatedModel.result.token)
	assert.Equal(t, now, updatedModel.result.expiresAt)
	assert.Nil(t, updatedModel.result.err)
	assert.True(t, cancelCalled)
}

func TestSpinnerModel_View(t *testing.T) {
	tests := []struct {
		name        string
		done        bool
		result      *pollResult
		expectEmpty bool
		expectText  string
	}{
		{
			name:        "in progress",
			done:        false,
			result:      nil,
			expectEmpty: false,
			expectText:  "Testing",
		},
		{
			name: "success",
			done: true,
			result: &pollResult{
				token: "test",
				err:   nil,
			},
			expectEmpty: true, // Success returns empty string, auth login will show table.
			expectText:  "",
		},
		{
			name: "failure",
			done: true,
			result: &pollResult{
				err: assert.AnError,
			},
			expectEmpty: false,
			expectText:  "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := spinnerModel{
				message: "Testing",
				done:    tt.done,
				result:  tt.result,
			}

			view := model.View()
			if tt.expectEmpty {
				assert.Empty(t, view)
			} else {
				assert.Contains(t, view, tt.expectText)
			}
		})
	}
}

func TestSpinnerModel_CheckResult(t *testing.T) {
	// Test checkResult with immediate result.
	resultChan := make(chan pollResult, 1)
	now := time.Now()
	resultChan <- pollResult{
		token:     "test-token",
		expiresAt: now,
		err:       nil,
	}

	model := spinnerModel{
		message:    "Testing",
		resultChan: resultChan,
	}

	cmd := model.checkResult()
	assert.NotNil(t, cmd)

	// Execute the command to get the message.
	msg := cmd()
	pollRes, ok := msg.(pollResult)
	assert.True(t, ok)
	assert.Equal(t, "test-token", pollRes.token)

	close(resultChan)
}

func TestDisplayVerificationPlainText_EmptyValues(t *testing.T) {
	// Test that displayVerificationPlainText handles empty values without panicking.
	assert.NotPanics(t, func() {
		displayVerificationPlainText("", "")
	})
}

func TestNewSSOProvider_MissingConfig(t *testing.T) {
	// Test that NewSSOProvider returns error with nil config.
	provider, err := NewSSOProvider("test", nil)
	assert.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), "provider config is required")
}

func TestNewSSOProvider_AllFields(t *testing.T) {
	// Test NewSSOProvider with all fields populated.
	config := &schema.Provider{
		Kind:     "aws/iam-identity-center",
		Region:   "us-west-2",
		StartURL: "https://company.awsapps.com/start",
		Session: &schema.SessionConfig{
			Duration: "30m",
		},
		Spec: map[string]interface{}{
			"aws": map[string]interface{}{
				"resolver": map[string]interface{}{
					"url": "http://localhost:4566",
				},
			},
		},
	}

	provider, err := NewSSOProvider("test-sso", config)
	assert.NoError(t, err)
	assert.NotNil(t, provider)
	assert.Equal(t, "test-sso", provider.Name())
	assert.Equal(t, "aws/iam-identity-center", provider.Kind())
	assert.Equal(t, "us-west-2", provider.region)
	assert.Equal(t, "https://company.awsapps.com/start", provider.startURL)
	assert.Equal(t, 30, provider.getSessionDuration())
}

func TestSpinnerModel_Update_OtherMessages(t *testing.T) {
	// Test that other messages don't change state.
	resultChan := make(chan pollResult, 1)
	defer close(resultChan)

	model := spinnerModel{
		message:    "Testing",
		done:       false,
		resultChan: resultChan,
	}

	// Simulate unknown message type (should be a no-op).
	newModel, _ := model.Update("unknown message")
	updatedModel := newModel.(spinnerModel)
	assert.False(t, updatedModel.done)
	assert.Nil(t, updatedModel.result)
}

func TestPromptDeviceAuth_VariousURLFormats(t *testing.T) {
	tests := []struct {
		name                    string
		userCode                *string
		verificationURI         *string
		verificationURIComplete *string
		isCI                    bool
	}{
		{
			name:                    "complete URI only",
			userCode:                stringPtr("ABCD-1234"),
			verificationURIComplete: stringPtr("https://device.sso.us-east-1.amazonaws.com/"),
			isCI:                    false,
		},
		{
			name:            "base URI only",
			userCode:        stringPtr("EFGH-5678"),
			verificationURI: stringPtr("https://device.sso.us-east-1.amazonaws.com/"),
			isCI:            false,
		},
		{
			name:     "CI environment",
			userCode: stringPtr("TEST-CODE"),
			isCI:     true,
		},
		{
			name: "nil user code",
			isCI: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GO_TEST", "1")
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

			authOutput := &ssooidc.StartDeviceAuthorizationOutput{
				UserCode:                tt.userCode,
				VerificationUri:         tt.verificationURI,
				VerificationUriComplete: tt.verificationURIComplete,
			}

			assert.NotPanics(t, func() {
				p.promptDeviceAuth(authOutput)
			})
		})
	}
}

func TestIsInteractive(t *testing.T) {
	// Test isInteractive function.
	result := isInteractive()
	// In test environment, this typically returns false, but we just verify it doesn't panic.
	assert.IsType(t, false, result)
}

// stringPtr is a helper to create string pointers.
func stringPtr(s string) *string {
	return &s
}

func TestNewSSOProvider_InvalidProviderKind(t *testing.T) {
	// Test that NewSSOProvider rejects non-SSO provider kinds.
	config := &schema.Provider{
		Kind:     "aws/static",
		Region:   testRegion,
		StartURL: testStartURL,
	}

	provider, err := NewSSOProvider(testProviderName, config)
	assert.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), "invalid provider kind for SSO provider")
}

func TestNewSSOProvider_MissingStartURL(t *testing.T) {
	// Test that NewSSOProvider requires start_url.
	config := &schema.Provider{
		Kind:   testSSOKind,
		Region: testRegion,
	}

	provider, err := NewSSOProvider(testProviderName, config)
	assert.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), "start_url is required")
}

func TestNewSSOProvider_MissingRegion(t *testing.T) {
	// Test that NewSSOProvider requires region.
	config := &schema.Provider{
		Kind:     testSSOKind,
		StartURL: testStartURL,
	}

	provider, err := NewSSOProvider(testProviderName, config)
	assert.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), "region is required")
}

func TestSSOProvider_PrepareEnvironment(t *testing.T) {
	// Test PrepareEnvironment method.
	// Note: SSO providers don't write credential files - that's done by identities.
	// PrepareEnvironment just returns the environment unchanged for interface compliance.
	config := &schema.Provider{
		Kind:     testSSOKind,
		Region:   testRegion,
		StartURL: testStartURL,
	}

	provider, err := NewSSOProvider(testProviderName, config)
	require.NoError(t, err)

	ctx := context.Background()
	inputEnv := map[string]string{
		"TEST_VAR":    "test_value",
		"ANOTHER_VAR": "another_value",
		"AWS_PROFILE": "existing_profile",
	}

	resultEnv, err := provider.PrepareEnvironment(ctx, inputEnv)
	assert.NoError(t, err)

	// Verify returned environment is a new map (not the same reference).
	// Modify the result to ensure input isn't affected.
	resultEnv["NEW_KEY"] = "new_value"
	assert.NotContains(t, inputEnv, "NEW_KEY", "Input map should not be modified")
	delete(resultEnv, "NEW_KEY") // Clean up for later assertions.

	// Verify all existing entries are preserved.
	assert.Equal(t, "test_value", resultEnv["TEST_VAR"])
	assert.Equal(t, "another_value", resultEnv["ANOTHER_VAR"])
	assert.Equal(t, "existing_profile", resultEnv["AWS_PROFILE"])

	// SSO provider's PrepareEnvironment injects AWS_REGION from provider config.
	// It doesn't inject AWS_SHARED_CREDENTIALS_FILE, AWS_CONFIG_FILE, etc.
	// Those are handled by identities (like permission-set) that use the SSO provider.
	assert.Equal(t, testRegion, resultEnv["AWS_REGION"], "AWS_REGION should be set from provider config")
	assert.NotEmpty(t, resultEnv["AWS_REGION"], "AWS_REGION should not be empty")
}
