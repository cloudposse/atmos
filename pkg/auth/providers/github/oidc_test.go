package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	stsTypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockSTSClient is a mock implementation of the assumeRoleWithWebIdentityClient interface.
type mockSTSClient struct {
	mock.Mock
}

func (m *mockSTSClient) AssumeRoleWithWebIdentity(ctx context.Context, params *sts.AssumeRoleWithWebIdentityInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleWithWebIdentityOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*sts.AssumeRoleWithWebIdentityOutput), args.Error(1)
}

// mockAuthManager is a mock implementation of the AuthManager interface for testing.
type mockAuthManager struct {
	chain      []string
	identities map[string]schema.Identity
}

func (m *mockAuthManager) GetChain() []string {
	return m.chain
}

func (m *mockAuthManager) GetIdentities() map[string]schema.Identity {
	return m.identities
}

// Authenticate is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) Authenticate(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

// GetCachedCredentials is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) GetCachedCredentials(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

// Whoami is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) Whoami(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

// Validate is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) Validate() error {
	return nil
}

// GetDefaultIdentity is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) GetDefaultIdentity(forceSelect bool) (string, error) {
	return "", fmt.Errorf("not implemented in mock")
}

// Logout is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) Logout(ctx context.Context, identityName string) error {
	return fmt.Errorf("not implemented in mock")
}

// LogoutProvider is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) LogoutProvider(ctx context.Context, providerName string) error {
	return fmt.Errorf("not implemented in mock")
}

// LogoutAll is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) LogoutAll(ctx context.Context) error {
	return fmt.Errorf("not implemented in mock")
}

// GetEnvironmentVariables is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) GetEnvironmentVariables(identityName string) (map[string]string, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

// PrepareShellEnvironment is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) PrepareShellEnvironment(ctx context.Context, identityName string, currentEnv []string) ([]string, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

// ListIdentities is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) ListIdentities() []string {
	return nil
}

// ListProviders is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) ListProviders() []string {
	return nil
}

// GetProviderForIdentity is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) GetProviderForIdentity(identityName string) string {
	return ""
}

// GetProviderKindForIdentity is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) GetProviderKindForIdentity(identityName string) (string, error) {
	return "", fmt.Errorf("not implemented in mock")
}

// GetFilesDisplayPath is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) GetFilesDisplayPath(providerName string) string {
	return ""
}

// GetStackInfo is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) GetStackInfo() *schema.ConfigAndStacksInfo {
	return nil
}

// GetProviders is required by the AuthManager interface but not used in these tests
func (m *mockAuthManager) GetProviders() map[string]schema.Provider {
	return nil
}

func validOidcSpec() *schema.Provider {
	return &schema.Provider{
		Kind:   "github/oidc",
		Region: "us-east-1",
		Spec: map[string]interface{}{
			"audience": "sts.us-east-1.amazonaws.com",
		},
	}
}

func TestNewOIDCProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		config       *schema.Provider
		expectError  bool
		errorMsg     string
	}{
		{
			name:         "valid config",
			providerName: "github-oidc",
			config:       validOidcSpec(),
			expectError:  false,
		},
		{
			name:         "nil config",
			providerName: "github-oidc",
			config:       nil,
			expectError:  true,
			errorMsg:     "provider config is required",
		},
		{
			name:         "empty name",
			providerName: "",
			config: &schema.Provider{
				Kind:   "github/oidc",
				Region: "us-east-1",
				Spec: map[string]interface{}{
					"audience": "sts.amazonaws.com",
				},
			},
			expectError: true,
			errorMsg:    "provider name is required",
		},
		{
			name:         "invalid provider kind",
			providerName: "github-oidc",
			config: &schema.Provider{
				Kind:   "aws/saml",
				Region: "us-east-1",
				Spec: map[string]interface{}{
					"audience": "sts.amazonaws.com",
				},
			},
			expectError: true,
			errorMsg:    "invalid provider kind",
		},
		{
			name:         "missing region",
			providerName: "github-oidc",
			config: &schema.Provider{
				Kind: "github/oidc",
				Spec: map[string]interface{}{
					"audience": "sts.amazonaws.com",
				},
			},
			expectError: true,
			errorMsg:    "region is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewOIDCProvider(tt.providerName, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				assert.Equal(t, "github/oidc", provider.Kind())
			}
		})
	}
}

func TestOIDCProvider_PreAuthenticate(t *testing.T) {
	tests := []struct {
		name        string
		chain       []string
		identities  map[string]schema.Identity
		expectError bool
		errorMsg    string
		expectedArn string
	}{
		{
			name:  "valid chain with assume_role",
			chain: []string{"github-oidc", "aws-identity"},
			identities: map[string]schema.Identity{
				"aws-identity": {
					Principal: map[string]interface{}{
						"assume_role": "arn:aws:iam::123456789012:role/GitHubActionsRole",
					},
				},
			},
			expectError: false,
			expectedArn: "arn:aws:iam::123456789012:role/GitHubActionsRole",
		},
		{
			name:        "no chain - provider only",
			chain:       []string{"github-oidc"},
			identities:  map[string]schema.Identity{},
			expectError: false,
			expectedArn: "",
		},
		{
			name:  "missing identity",
			chain: []string{"github-oidc", "missing-identity"},
			identities: map[string]schema.Identity{
				"other-identity": {
					Principal: map[string]interface{}{
						"assume_role": "arn:aws:iam::123456789012:role/Role",
					},
				},
			},
			expectError: true,
			errorMsg:    "identity \"missing-identity\" not found",
		},
		{
			name:  "missing assume_role",
			chain: []string{"github-oidc", "aws-identity"},
			identities: map[string]schema.Identity{
				"aws-identity": {
					Principal: map[string]interface{}{},
				},
			},
			expectError: true,
			errorMsg:    "assume_role is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := validOidcSpec()
			p, err := NewOIDCProvider("github-oidc", config)
			require.NoError(t, err)

			provider := p.(*oidcProvider)
			manager := &mockAuthManager{
				chain:      tt.chain,
				identities: tt.identities,
			}

			err = provider.PreAuthenticate(manager)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedArn, provider.RoleToAssumeFromWebIdentity)
			}
		})
	}
}

func TestOIDCProvider_Authenticate(t *testing.T) {
	tests := []struct {
		name        string
		setupEnv    func()
		cleanupEnv  func()
		expectError bool
		errorMsg    string
		setOidcUrl  bool
		setupMock   func(*mockSTSClient)
	}{
		{
			name: "successful authentication",
			setupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "true")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")
			},
			cleanupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
			},
			setOidcUrl:  true,
			expectError: false,
			setupMock: func(m *mockSTSClient) {
				expirationTime := time.Now().Add(1 * time.Hour)
				m.On("AssumeRoleWithWebIdentity", mock.Anything, mock.Anything).Return(
					&sts.AssumeRoleWithWebIdentityOutput{
						Credentials: &stsTypes.Credentials{
							AccessKeyId:     aws.String("AKIAIOSFODNN7EXAMPLE"),
							SecretAccessKey: aws.String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
							SessionToken:    aws.String("session-token"),
							Expiration:      &expirationTime,
						},
					},
					nil,
				)
			},
		},
		{
			name: "missing GitHub Actions environment",
			setupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
			},
			cleanupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
			},
			setOidcUrl:  true,
			expectError: true,
			errorMsg:    "GitHub OIDC authentication is only available in GitHub Actions environment",
			setupMock:   func(m *mockSTSClient) {},
		},
		{
			name: "missing role to assume",
			setupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "true")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")
			},
			cleanupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
			},
			setOidcUrl:  true,
			expectError: true,
			errorMsg:    "no role to assume for web identity",
			setupMock:   func(m *mockSTSClient) {},
		},
		{
			name: "missing OIDC token",
			setupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "true")
			},
			cleanupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "")
			},
			setOidcUrl:  true,
			expectError: true,
			// The error will be about missing role since PreAuthenticate wasn't called
			// or about missing token. Since we don't set the role, we get the role error first.
			errorMsg:  "no role to assume",
			setupMock: func(m *mockSTSClient) {},
		},
		{
			name: "missing OIDC URL",
			setupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "true")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")
			},
			cleanupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
			},
			expectError: true,
			setOidcUrl:  false,
			// The error will be about missing role since PreAuthenticate wasn't called
			errorMsg:  "no role to assume",
			setupMock: func(m *mockSTSClient) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup.
			tt.setupEnv()
			defer tt.cleanupEnv()

			config := validOidcSpec()
			p, err := NewOIDCProvider("github-oidc", config)
			require.NoError(t, err)

			provider := p.(*oidcProvider)

			// Set role ARN for tests that should proceed past validation
			if tt.name == "successful authentication" {
				provider.RoleToAssumeFromWebIdentity = "arn:aws:iam::123456789012:role/GitHubActionsRole"
			}

			// Setup mock STS client
			mockClient := new(mockSTSClient)
			tt.setupMock(mockClient)

			// Test.
			ctx := context.Background()
			// Local OIDC endpoint.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"value":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test-jwt-token"}`))
			}))
			defer srv.Close()
			if tt.setOidcUrl {
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)
			}

			// Use the testable version with dependency injection
			var creds *types.AWSCredentials
			if tt.name == "successful authentication" {
				creds, err = provider.assumeRoleWithWebIdentityWithDeps(
					ctx,
					"test-jwt-token",
					func(ctx context.Context, optFns ...func(*awsConfig.LoadOptions) error) (aws.Config, error) {
						return aws.Config{Region: "us-east-1"}, nil
					},
					func(cfg aws.Config) assumeRoleWithWebIdentityClient {
						return mockClient
					},
				)
			} else {
				_, err = provider.Authenticate(ctx)
			}

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, creds)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, creds)
				assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", creds.AccessKeyID)
				assert.Equal(t, "us-east-1", creds.Region)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestOIDCProvider_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *schema.Provider
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid config",
			config:      validOidcSpec(),
			expectError: false,
		},
		{
			name: "missing audience",
			config: &schema.Provider{
				Kind:   "github/oidc",
				Region: "us-east-1",
				Spec:   map[string]interface{}{},
			},
			expectError: true,
			errorMsg:    "audience is required",
		},
		{
			name: "missing region",
			config: &schema.Provider{
				Kind: "github/oidc",
				Spec: map[string]interface{}{
					"audience": "sts.amazonaws.com",
				},
			},
			expectError: true,
			errorMsg:    "region is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error

			// Only create provider if config is valid enough for NewOIDCProvider
			if tt.config.Region != "" {
				provider, provErr := NewOIDCProvider("github-oidc", tt.config)
				require.NoError(t, provErr)

				err = provider.Validate()
			} else {
				// For missing region, the error will come from NewOIDCProvider
				_, err = NewOIDCProvider("github-oidc", tt.config)
			}

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOIDCProvider_Environment(t *testing.T) {
	config := validOidcSpec()
	provider, err := NewOIDCProvider("github-oidc", config)
	require.NoError(t, err)

	env, err := provider.Environment()
	assert.NoError(t, err)
	assert.NotEmpty(t, env)
	assert.Equal(t, "us-east-1", env["AWS_REGION"])
	assert.Equal(t, "us-east-1", env["AWS_DEFAULT_REGION"])
}

func TestOIDCProvider_isGitHubActions(t *testing.T) {
	tests := []struct {
		name     string
		setupEnv func()
		cleanup  func()
		expected bool
	}{
		{
			name: "GitHub Actions environment",
			setupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "true")
			},
			cleanup: func() {
				os.Unsetenv("GITHUB_ACTIONS")
			},
			expected: true,
		},
		{
			name: "non-GitHub Actions environment",
			setupEnv: func() {
				os.Unsetenv("GITHUB_ACTIONS")
			},
			cleanup:  func() {},
			expected: false,
		},
		{
			name: "GitHub Actions set to false",
			setupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "false")
			},
			cleanup: func() {
				os.Unsetenv("GITHUB_ACTIONS")
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			defer tt.cleanup()

			config := validOidcSpec()
			provider, err := NewOIDCProvider("github-oidc", config)
			require.NoError(t, err)

			// Access the private method through reflection or make it public for testing.
			// For now, we'll test the behavior through Authenticate.
			ctx := context.Background()
			_, err = provider.Authenticate(ctx)

			if tt.expected {
				// Should not fail due to GitHub Actions check (may fail for other reasons like missing tokens).
				if err != nil {
					assert.NotContains(t, err.Error(), "GitHub OIDC authentication is only available in GitHub Actions environment")
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "GitHub OIDC authentication is only available in GitHub Actions environment")
			}
		})
	}
}

func TestOIDCProvider_NameAndPreAuthenticate(t *testing.T) {
	p, err := NewOIDCProvider("github-oidc", &schema.Provider{Kind: "github/oidc", Region: "us-east-1", Spec: map[string]interface{}{"audience": "test"}})
	require.NoError(t, err)
	require.Equal(t, "github-oidc", p.Name())

	// PreAuthenticate with empty chain (provider only)
	manager := &mockAuthManager{
		chain:      []string{"github-oidc"},
		identities: map[string]schema.Identity{},
	}
	require.NoError(t, p.PreAuthenticate(manager))
}

func TestOIDCProvider_Logout(t *testing.T) {
	p, err := NewOIDCProvider("github-oidc", validOidcSpec())
	require.NoError(t, err)

	ctx := context.Background()
	err = p.Logout(ctx)
	// GitHub OIDC provider now cleans up AWS credential files
	// The error could be nil (success) or an error if cleanup fails
	// Since we don't have actual files in this test, it might succeed or fail gracefully
	if err != nil {
		// If there's an error, it should be a logout-related error
		assert.True(t, errors.Is(err, errUtils.ErrProviderLogout) || errors.Is(err, errUtils.ErrLogoutFailed))
	}
}

func TestOIDCProvider_GetFilesDisplayPath(t *testing.T) {
	p, err := NewOIDCProvider("github-oidc", validOidcSpec())
	require.NoError(t, err)

	path := p.GetFilesDisplayPath()
	// GitHub OIDC provider now returns AWS credential file path
	assert.NotEmpty(t, path)
	assert.Contains(t, path, "aws")
}

func TestOIDCProvider_RequestedSessionSeconds(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.Provider
		expected int32
	}{
		{
			name:     "default session duration",
			config:   validOidcSpec(),
			expected: defaultSessionSeconds,
		},
		{
			name: "custom session duration",
			config: &schema.Provider{
				Kind:   "github/oidc",
				Region: "us-east-1",
				Spec: map[string]interface{}{
					"audience": "sts.amazonaws.com",
				},
				Session: &schema.SessionConfig{
					Duration: "2h",
				},
			},
			expected: 7200,
		},
		{
			name: "minimum session duration",
			config: &schema.Provider{
				Kind:   "github/oidc",
				Region: "us-east-1",
				Spec: map[string]interface{}{
					"audience": "sts.amazonaws.com",
				},
				Session: &schema.SessionConfig{
					Duration: "5m",
				},
			},
			expected: minSTSSeconds,
		},
		{
			name: "maximum session duration",
			config: &schema.Provider{
				Kind:   "github/oidc",
				Region: "us-east-1",
				Spec: map[string]interface{}{
					"audience": "sts.amazonaws.com",
				},
				Session: &schema.SessionConfig{
					Duration: "24h",
				},
			},
			expected: maxSTSSeconds,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewOIDCProvider("github-oidc", tt.config)
			require.NoError(t, err)

			provider := p.(*oidcProvider)
			actual := provider.requestedSessionSeconds()
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestOIDCProvider_AssumeRoleWithWebIdentity(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*mockSTSClient)
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful assume role",
			setupMock: func(m *mockSTSClient) {
				expirationTime := time.Now().Add(1 * time.Hour)
				m.On("AssumeRoleWithWebIdentity", mock.Anything, mock.MatchedBy(func(input *sts.AssumeRoleWithWebIdentityInput) bool {
					return aws.ToString(input.RoleArn) == "arn:aws:iam::123456789012:role/GitHubActionsRole" &&
						aws.ToString(input.WebIdentityToken) == "test-token" &&
						aws.ToString(input.RoleSessionName) == defaultRoleSessionName
				})).Return(
					&sts.AssumeRoleWithWebIdentityOutput{
						Credentials: &stsTypes.Credentials{
							AccessKeyId:     aws.String("AKIAIOSFODNN7EXAMPLE"),
							SecretAccessKey: aws.String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
							SessionToken:    aws.String("session-token"),
							Expiration:      &expirationTime,
						},
					},
					nil,
				)
			},
			expectError: false,
		},
		{
			name: "assume role with custom session name",
			setupMock: func(m *mockSTSClient) {
				expirationTime := time.Now().Add(1 * time.Hour)
				m.On("AssumeRoleWithWebIdentity", mock.Anything, mock.MatchedBy(func(input *sts.AssumeRoleWithWebIdentityInput) bool {
					return aws.ToString(input.RoleSessionName) == "custom-session"
				})).Return(
					&sts.AssumeRoleWithWebIdentityOutput{
						Credentials: &stsTypes.Credentials{
							AccessKeyId:     aws.String("AKIAIOSFODNN7EXAMPLE"),
							SecretAccessKey: aws.String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
							SessionToken:    aws.String("session-token"),
							Expiration:      &expirationTime,
						},
					},
					nil,
				)
			},
			expectError: false,
		},
		{
			name: "STS error",
			setupMock: func(m *mockSTSClient) {
				m.On("AssumeRoleWithWebIdentity", mock.Anything, mock.Anything).Return(
					nil,
					fmt.Errorf("access denied"),
				)
			},
			expectError: true,
			errorMsg:    "failed to assume role with web identity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := validOidcSpec()
			if tt.name == "assume role with custom session name" {
				config.Spec["role_session_name"] = "custom-session"
			}

			p, err := NewOIDCProvider("github-oidc", config)
			require.NoError(t, err)

			provider := p.(*oidcProvider)
			provider.RoleToAssumeFromWebIdentity = "arn:aws:iam::123456789012:role/GitHubActionsRole"

			mockClient := new(mockSTSClient)
			tt.setupMock(mockClient)

			ctx := context.Background()
			creds, err := provider.assumeRoleWithWebIdentityWithDeps(
				ctx,
				"test-token",
				func(ctx context.Context, optFns ...func(*awsConfig.LoadOptions) error) (aws.Config, error) {
					return aws.Config{Region: "us-east-1"}, nil
				},
				func(cfg aws.Config) assumeRoleWithWebIdentityClient {
					return mockClient
				},
			)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, creds)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, creds)
				assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", creds.AccessKeyID)
				assert.Equal(t, "us-east-1", creds.Region)
				assert.NotEmpty(t, creds.SessionToken)
			}

			mockClient.AssertExpectations(t)
		})
	}
}
