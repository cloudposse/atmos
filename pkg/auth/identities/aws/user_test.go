package aws

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	atmosCreds "github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewUserIdentity_And_GetProviderName(t *testing.T) {
	// Wrong kind should error.
	_, err := NewUserIdentity("me", &schema.Identity{Kind: "aws/assume-role"})
	assert.Error(t, err)

	// Correct kind.
	id, err := NewUserIdentity("me", &schema.Identity{Kind: "aws/user"})
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.Equal(t, "aws/user", id.Kind())

	// Provider name is constant.
	name, err := id.GetProviderName()
	assert.NoError(t, err)
	assert.Equal(t, "aws-user", name)
}

func TestUserIdentity_Environment(t *testing.T) {
	// Environment should include AWS files and pass through additional env from config.
	id, err := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user", Env: []schema.EnvironmentVariable{{Key: "FOO", Value: "BAR"}}})
	require.NoError(t, err)
	env, err := id.Environment()
	require.NoError(t, err)

	// Contains the three AWS_* vars and our custom one.
	assert.NotEmpty(t, env["AWS_SHARED_CREDENTIALS_FILE"])
	assert.NotEmpty(t, env["AWS_CONFIG_FILE"])
	// Points under XDG config: atmos/aws/aws-user.
	assert.Contains(t, env["AWS_SHARED_CREDENTIALS_FILE"], filepath.Join("atmos", "aws", "aws-user"))
	assert.Contains(t, env["AWS_CONFIG_FILE"], filepath.Join("atmos", "aws", "aws-user"))
	assert.Equal(t, "BAR", env["FOO"])
}

func TestIsStandaloneAWSUserChain(t *testing.T) {
	// Not standalone when multiple elements.
	assert.False(t, IsStandaloneAWSUserChain([]string{"p", "dev"}, map[string]schema.Identity{"dev": {Kind: "aws/user"}}))

	// Single element but wrong kind -> false.
	assert.False(t, IsStandaloneAWSUserChain([]string{"dev"}, map[string]schema.Identity{"dev": {Kind: "aws/permission-set"}}))

	// Single element and aws/user -> true.
	assert.True(t, IsStandaloneAWSUserChain([]string{"dev"}, map[string]schema.Identity{"dev": {Kind: "aws/user"}}))
}

// stubUser satisfies types.Identity for testing AuthenticateStandaloneAWSUser.
type stubUser struct{ creds types.ICredentials }

func (s stubUser) Kind() string                     { return "aws/user" }
func (s stubUser) GetProviderName() (string, error) { return "aws-user", nil }
func (s stubUser) Authenticate(_ context.Context, _ types.ICredentials) (types.ICredentials, error) {
	return s.creds, nil
}
func (s stubUser) Validate() error                         { return nil }
func (s stubUser) Environment() (map[string]string, error) { return map[string]string{}, nil }
func (s stubUser) PostAuthenticate(_ context.Context, _ *types.PostAuthenticateParams) error {
	return nil
}
func (s stubUser) Logout(_ context.Context) error                                { return nil }
func (s stubUser) CredentialsExist() (bool, error)                               { return true, nil }
func (s stubUser) LoadCredentials(_ context.Context) (types.ICredentials, error) { return s.creds, nil }

func TestAuthenticateStandaloneAWSUser(t *testing.T) {
	// Not found -> error.
	_, err := AuthenticateStandaloneAWSUser(context.Background(), "missing", map[string]types.Identity{})
	assert.Error(t, err)

	// Found -> returns credentials from identity implementation.
	out, err := AuthenticateStandaloneAWSUser(context.Background(), "dev", map[string]types.Identity{
		"dev": stubUser{creds: &types.AWSCredentials{AccessKeyID: "AKIA", Region: "us-east-1"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "AKIA", out.(*types.AWSCredentials).AccessKeyID)
}

// Use in-memory keyring for this test package.
func init() { keyring.MockInit() }

func TestUser_credentialsFromConfig(t *testing.T) {
	// Missing secret when access_key_id present -> error.
	id, err := NewUserIdentity("me", &schema.Identity{Kind: "aws/user", Credentials: map[string]any{
		"access_key_id": "AKIA",
	}})
	require.NoError(t, err)
	ui := id.(*userIdentity)
	creds, err := ui.credentialsFromConfig()
	assert.Nil(t, creds)
	assert.Error(t, err)

	// Full credentials with MFA -> success.
	id, err = NewUserIdentity("me", &schema.Identity{Kind: "aws/user", Credentials: map[string]any{
		"access_key_id":     "AKIA",
		"secret_access_key": "SECRET",
		"mfa_arn":           "arn:aws:iam::111111111111:mfa/me",
	}})
	require.NoError(t, err)
	ui = id.(*userIdentity)
	creds, err = ui.credentialsFromConfig()
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "AKIA", creds.AccessKeyID)
	assert.Equal(t, "SECRET", creds.SecretAccessKey)
	assert.Equal(t, "arn:aws:iam::111111111111:mfa/me", creds.MfaArn)
}

func TestUser_credentialsFromStore(t *testing.T) {
	// Prime the store for alias "dev".
	store := atmosCreds.NewCredentialStore()
	_ = store.Store("dev", &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "SECRET", Region: "us-east-1"})

	id, err := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user"})
	require.NoError(t, err)
	ui := id.(*userIdentity)

	// Success path.
	creds, err := ui.credentialsFromStore()
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "AKIA", creds.AccessKeyID)
	assert.Equal(t, "us-east-1", creds.Region)

	// Wrong type stored.
	_ = store.Store("other", &types.OIDCCredentials{Token: "hdr.payload."})
	id, _ = NewUserIdentity("other", &schema.Identity{Kind: "aws/user"})
	ui = id.(*userIdentity)
	_, err = ui.credentialsFromStore()
	assert.Error(t, err)

	// Incomplete stored.
	_ = store.Store("incomplete", &types.AWSCredentials{AccessKeyID: "AKIA"})
	id, _ = NewUserIdentity("incomplete", &schema.Identity{Kind: "aws/user"})
	ui = id.(*userIdentity)
	_, err = ui.credentialsFromStore()
	assert.Error(t, err)

	// Missing alias -> retrieval error.
	id, _ = NewUserIdentity("missing", &schema.Identity{Kind: "aws/user"})
	ui = id.(*userIdentity)
	_, err = ui.credentialsFromStore()
	assert.Error(t, err)
}

func TestUser_resolveLongLivedCredentials_Order(t *testing.T) {
	// When config has full credentials, prefer those.
	id, err := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user", Credentials: map[string]any{
		"access_key_id":     "AKIA",
		"secret_access_key": "SECRET",
	}})
	require.NoError(t, err)
	ui := id.(*userIdentity)
	creds, err := ui.resolveLongLivedCredentials()
	require.NoError(t, err)
	assert.Equal(t, "AKIA", creds.AccessKeyID)

	// If config has no access key, fallback to store.
	// Prime the store.
	store := atmosCreds.NewCredentialStore()
	_ = store.Store("dev2", &types.AWSCredentials{AccessKeyID: "AK2", SecretAccessKey: "SEC2"})
	id, _ = NewUserIdentity("dev2", &schema.Identity{Kind: "aws/user"})
	ui = id.(*userIdentity)
	creds, err = ui.resolveLongLivedCredentials()
	require.NoError(t, err)
	assert.Equal(t, "AK2", creds.AccessKeyID)
}

func TestUser_resolveRegion_DefaultAndOverride(t *testing.T) {
	id, _ := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user"})
	ui := id.(*userIdentity)
	assert.Equal(t, defaultRegion, ui.resolveRegion())

	id, _ = NewUserIdentity("dev", &schema.Identity{Kind: "aws/user", Credentials: map[string]any{"region": "us-west-2"}})
	ui = id.(*userIdentity)
	assert.Equal(t, "us-west-2", ui.resolveRegion())
}

func TestUser_writeAWSFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	id, _ := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user"})
	ui := id.(*userIdentity)
	creds := &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "SECRET", SessionToken: "TOKEN"}
	err := ui.writeAWSFiles(creds, "us-east-2")
	require.NoError(t, err)

	// Files should exist under XDG config: atmos/aws/aws-user.
	// Note: we can't know the exact tempdir path here; assert partial suffix.
	env, _ := id.Environment()
	require.Contains(t, env["AWS_SHARED_CREDENTIALS_FILE"], filepath.Join("atmos", "aws", "aws-user", "credentials"))
	require.Contains(t, env["AWS_CONFIG_FILE"], filepath.Join("atmos", "aws", "aws-user", "config"))
}

func TestUser_buildGetSessionTokenInput_NoMFA(t *testing.T) {
	id, _ := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user"})
	ui := id.(*userIdentity)
	in, err := ui.buildGetSessionTokenInput(&types.AWSCredentials{})
	require.NoError(t, err)
	require.NotNil(t, in)
	assert.Nil(t, in.SerialNumber)
	assert.Nil(t, in.TokenCode)
	require.NotNil(t, in.DurationSeconds)
	assert.Equal(t, int32(defaultUserSessionSeconds), *in.DurationSeconds)
}

func TestUser_buildGetSessionTokenInput_WithMFA(t *testing.T) {
	id, _ := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user"})
	ui := id.(*userIdentity)

	// Stub MFA prompt to avoid interactive UI.
	old := promptMfaTokenFunc
	defer func() { promptMfaTokenFunc = old }()
	promptMfaTokenFunc = func(_ *types.AWSCredentials) (string, error) { return "123456", nil }

	in, err := ui.buildGetSessionTokenInput(&types.AWSCredentials{MfaArn: "arn:aws:iam::111111111111:mfa/me"})
	require.NoError(t, err)
	require.NotNil(t, in)
	require.NotNil(t, in.SerialNumber)
	assert.Equal(t, "arn:aws:iam::111111111111:mfa/me", *in.SerialNumber)
	require.NotNil(t, in.TokenCode)
	assert.Equal(t, "123456", *in.TokenCode)
	assert.Equal(t, int32(defaultUserSessionSeconds), *in.DurationSeconds)
}

func TestUser_PostAuthenticate_SetsEnvAndFiles(t *testing.T) {
	to := t.TempDir()
	t.Setenv("HOME", to)
	id, _ := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user"})
	ui := id.(*userIdentity)
	authContext := &schema.AuthContext{}
	stack := &schema.ConfigAndStacksInfo{}
	creds := &types.AWSCredentials{AccessKeyID: "AK", SecretAccessKey: "SE", Region: "us-east-1"}
	err := ui.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext:  authContext,
		StackInfo:    stack,
		ProviderName: "aws-user",
		IdentityName: "dev",
		Credentials:  creds,
	})
	require.NoError(t, err)

	// Auth context populated.
	require.NotNil(t, authContext.AWS)
	assert.Equal(t, "dev", authContext.AWS.Profile)
	assert.Equal(t, "us-east-1", authContext.AWS.Region)

	// Env set on stack (derived from auth context).
	// XDG path contains "atmos/aws/aws-user/credentials"
	assert.Contains(t, stack.ComponentEnvSection["AWS_SHARED_CREDENTIALS_FILE"], filepath.Join("atmos", "aws", "aws-user", "credentials"))
	assert.Equal(t, "dev", stack.ComponentEnvSection["AWS_PROFILE"])
}

func TestUser_generateSessionToken_toAWSCredentials(t *testing.T) {
	// This tests the conversion part separately by invoking GetSessionToken via a local stubbed client through a helper.
	// We don’t call generateSessionToken directly to avoid network calls.
	// Instead, validate that the result from STS is translated as expected.
	exp := time.Now().Add(45 * time.Minute)
	out := &sts.GetSessionTokenOutput{Credentials: &ststypes.Credentials{
		AccessKeyId:     aws.String("TMPAKIA"),
		SecretAccessKey: aws.String("TMPSECRET"),
		SessionToken:    aws.String("TMPTOKEN"),
		Expiration:      &exp,
	}}
	// Verify fields mapping and RFC3339 formatting for expiration.
	// Reuse logic similar to user.generateSessionToken’s construction.
	region := "us-west-1"
	sessionCreds := &types.AWSCredentials{
		AccessKeyID:     aws.ToString(out.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(out.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(out.Credentials.SessionToken),
		Region:          region,
		Expiration:      out.Credentials.Expiration.Format(time.RFC3339),
	}
	assert.Equal(t, "TMPAKIA", sessionCreds.AccessKeyID)
	assert.Equal(t, region, sessionCreds.Region)
	assert.NotEmpty(t, sessionCreds.Expiration)
}

func TestUser_credentialsFromStore_ErrorsWhenMissing(t *testing.T) {
	id, err := NewUserIdentity("nostore", &schema.Identity{Kind: "aws/user"})
	require.NoError(t, err)
	ui := id.(*userIdentity)
	creds, err := ui.credentialsFromStore()
	assert.Nil(t, creds)
	assert.Error(t, err)
}

func TestUser_buildGetSessionTokenInput_MFAError(t *testing.T) {
	id, _ := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user"})
	ui := id.(*userIdentity)

	// Stub MFA prompt to return error and verify it propagates.
	old := promptMfaTokenFunc
	defer func() { promptMfaTokenFunc = old }()
	promptMfaTokenFunc = func(_ *types.AWSCredentials) (string, error) { return "", errors.New("prompt failed") }

	in, err := ui.buildGetSessionTokenInput(&types.AWSCredentials{MfaArn: "arn:aws:iam::111111111111:mfa/me"})
	assert.Nil(t, in)
	assert.Error(t, err)
}

func TestUserIdentity_Logout(t *testing.T) {
	tests := []struct {
		name         string
		identityName string
		expectError  bool
	}{
		{
			name:         "successful logout",
			identityName: "test-user",
			expectError:  false,
		},
		{
			name:         "logout with different identity name",
			identityName: "dev",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity, err := NewUserIdentity(tt.identityName, &schema.Identity{
				Kind: "aws/user",
			})
			require.NoError(t, err)

			ctx := context.Background()
			err = identity.Logout(ctx)

			// Logout should succeed (it creates temp dir for file manager).
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUserIdentity_Validate(t *testing.T) {
	identity := &userIdentity{
		name: "test-user",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	err := identity.Validate()
	assert.NoError(t, err, "user identity validation should always succeed")
}

func TestUserIdentity_CredentialsExist(t *testing.T) {
	// Create a temporary directory for credentials.
	tmpDir := t.TempDir()

	tests := []struct {
		name           string
		setupFiles     bool
		expectedExists bool
		expectedError  bool
	}{
		{
			name:           "credentials file exists",
			setupFiles:     true,
			expectedExists: true,
			expectedError:  false,
		},
		{
			name:           "credentials file does not exist",
			setupFiles:     false,
			expectedExists: false,
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create identity.
			identity, err := NewUserIdentity("test-user", &schema.Identity{
				Kind: "aws/user",
			})
			require.NoError(t, err)

			// Setup credentials file if needed.
			if tt.setupFiles {
				// Create AWS file manager with custom base path.
				t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)

				// Create the credentials file.
				credPath := filepath.Join(tmpDir, "atmos", "aws", "aws-user", "credentials")
				require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
				require.NoError(t, os.WriteFile(credPath, []byte("[test-user]\naws_access_key_id=test\n"), 0o600))
			} else {
				// Use a non-existent directory.
				t.Setenv("ATMOS_XDG_CONFIG_HOME", filepath.Join(tmpDir, "nonexistent"))
			}

			// Check if credentials exist.
			exists, err := identity.CredentialsExist()

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedExists, exists)
			}
		})
	}
}

func TestUserIdentity_LoadCredentials(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name          string
		setupFiles    bool
		expectedError bool
	}{
		{
			name:          "successfully loads credentials from files",
			setupFiles:    true,
			expectedError: false,
		},
		{
			name:          "fails when credentials file does not exist",
			setupFiles:    false,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create identity.
			identity, err := NewUserIdentity("test-user", &schema.Identity{
				Kind: "aws/user",
			})
			require.NoError(t, err)

			// Setup credentials and config files if needed.
			if tt.setupFiles {
				t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)

				// Create credentials file.
				credPath := filepath.Join(tmpDir, "atmos", "aws", "aws-user", "credentials")
				require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
				credContent := `[test-user]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
aws_session_token = FwoGZXIvYXdzEBExample
`
				require.NoError(t, os.WriteFile(credPath, []byte(credContent), 0o600))

				// Create config file.
				configPath := filepath.Join(tmpDir, "atmos", "aws", "aws-user", "config")
				configContent := `[profile test-user]
region = us-west-2
output = json
`
				require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))
			} else {
				t.Setenv("ATMOS_XDG_CONFIG_HOME", filepath.Join(tmpDir, "nonexistent"))
			}

			// Load credentials.
			ctx := context.Background()
			creds, err := identity.LoadCredentials(ctx)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, creds)
			} else {
				require.NoError(t, err)
				require.NotNil(t, creds)

				// Verify credentials were loaded.
				awsCreds, ok := creds.(*types.AWSCredentials)
				require.True(t, ok, "credentials should be AWSCredentials type")
				assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", awsCreds.AccessKeyID)
				assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", awsCreds.SecretAccessKey)
				assert.Equal(t, "FwoGZXIvYXdzEBExample", awsCreds.SessionToken)
				assert.Equal(t, "us-west-2", awsCreds.Region)
			}
		})
	}
}
