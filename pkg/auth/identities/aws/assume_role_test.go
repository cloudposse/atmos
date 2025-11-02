package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

func TestNewAssumeRoleIdentity(t *testing.T) {
	// Wrong kind should error.
	_, err := NewAssumeRoleIdentity("role", &schema.Identity{Kind: "aws/permission-set"})
	assert.Error(t, err)

	// Correct kind succeeds.
	id, err := NewAssumeRoleIdentity("role", &schema.Identity{Kind: "aws/assume-role"})
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.Equal(t, "aws/assume-role", id.Kind())
}

func TestAssumeRoleIdentity_ValidateAndProviderName(t *testing.T) {
	// Missing principal -> error.
	i := &assumeRoleIdentity{name: "role", config: &schema.Identity{Kind: "aws/assume-role"}}
	assert.Error(t, i.Validate())

	// Missing assume_role -> error.
	i = &assumeRoleIdentity{name: "role", config: &schema.Identity{Kind: "aws/assume-role", Principal: map[string]any{}}}
	assert.Error(t, i.Validate())

	// Valid minimal config with provider via.
	i = &assumeRoleIdentity{name: "role", config: &schema.Identity{
		Kind: "aws/assume-role",
		Via:  &schema.IdentityVia{Provider: "aws-sso"},
		Principal: map[string]any{
			"assume_role": "arn:aws:iam::123456789012:role/Dev",
			"region":      "us-west-2",
		},
	}}
	require.NoError(t, i.Validate())
	// Provider name resolves from Via.Provider.
	prov, err := i.GetProviderName()
	assert.NoError(t, err)
	assert.Equal(t, "aws-sso", prov)

	// Via.Identity fallback.
	i.config.Via = &schema.IdentityVia{Identity: "base"}
	prov, err = i.GetProviderName()
	assert.NoError(t, err)
	assert.Equal(t, "base", prov)

	// Neither set -> error.
	i.config.Via = &schema.IdentityVia{}
	_, err = i.GetProviderName()
	assert.Error(t, err)
}

func TestAssumeRoleIdentity_Environment(t *testing.T) {
	i := &assumeRoleIdentity{name: "role", config: &schema.Identity{
		Kind:      "aws/assume-role",
		Principal: map[string]any{"assume_role": "arn:aws:iam::123:role/x"},
		Env:       []schema.EnvironmentVariable{{Key: "FOO", Value: "BAR"}},
		Via:       &schema.IdentityVia{Provider: "test-provider"},
	}}
	env, err := i.Environment()
	assert.NoError(t, err)
	// Should include custom env vars from config.
	assert.Equal(t, "BAR", env["FOO"])
	// Should include AWS file environment variables.
	assert.NotEmpty(t, env["AWS_SHARED_CREDENTIALS_FILE"])
	assert.NotEmpty(t, env["AWS_CONFIG_FILE"])
	assert.NotEmpty(t, env["AWS_PROFILE"])
	assert.Equal(t, "role", env["AWS_PROFILE"])
}

func TestAssumeRoleIdentity_BuildAssumeRoleInput(t *testing.T) {
	// External ID and duration should be set when provided.
	i := &assumeRoleIdentity{name: "role", config: &schema.Identity{
		Kind: "aws/assume-role",
		Principal: map[string]any{
			"assume_role": "arn:aws:iam::123456789012:role/Dev",
			"external_id": "abc-123",
			"duration":    "15m",
		},
	}}
	// Validate populates role arn.
	require.NoError(t, i.Validate())
	in := i.buildAssumeRoleInput()
	require.NotNil(t, in)
	assert.NotNil(t, in.ExternalId)
	assert.Equal(t, int32(900), *in.DurationSeconds)

	// Invalid duration -> no DurationSeconds set.
	i = &assumeRoleIdentity{name: "role", config: &schema.Identity{
		Kind: "aws/assume-role",
		Principal: map[string]any{
			"assume_role": "arn:aws:iam::123456789012:role/Dev",
			"duration":    "bogus",
		},
	}}
	require.NoError(t, i.Validate())
	in = i.buildAssumeRoleInput()
	assert.Nil(t, in.DurationSeconds)
}

func TestAssumeRoleIdentity_toAWSCredentials(t *testing.T) {
	i := &assumeRoleIdentity{name: "role", region: "us-east-2"}

	// Nil result -> error.
	_, err := i.toAWSCredentials(nil)
	assert.Error(t, err)

	// Valid conversion.
	exp := time.Now().Add(time.Hour)
	out := &sts.AssumeRoleOutput{Credentials: &ststypes.Credentials{
		AccessKeyId:     aws.String("AKIA123"),
		SecretAccessKey: aws.String("secret"),
		SessionToken:    aws.String("token"),
		Expiration:      &exp,
	}}
	creds, err := i.toAWSCredentials(out)
	require.NoError(t, err)
	assert.Equal(t, "us-east-2", creds.(*types.AWSCredentials).Region)
}

func Test_sanitizeRoleSessionName(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "valid",
			args: args{s: "atmos-dev-1699566677"},
			want: "atmos-dev-1699566677",
		},
		{
			name: "invalid characters replaced",
			args: args{s: "atmos-dev-1699566677!"},
			want: "atmos-dev-1699566677",
		},
		{
			name: "multiple invalid characters",
			args: args{s: "atmos-dev-1699566677!@#$%^&*()"},
			want: "atmos-dev-1699566677-@",
		},
		{
			name: "mixed valid and invalid",
			args: args{s: "atmos-dev-1699566677!@#$%^&*()_-"},
			want: "atmos-dev-1699566677-@",
		},
		{
			name: "equals sign replaced",
			args: args{s: "atmos-dev-1699566677!@#$%^&*()_-="},
			want: "atmos-dev-1699566677-@----------=",
		},
		{
			name: "control character replaced",
			args: args{s: "atmos-dev-1699566677!@#$%^&*()_-=" + string([]rune{0x7f})},
			want: "atmos-dev-1699566677-@----------=",
		},
		{
			name: "truncated to max length",
			args: args{s: "very-long-role-session-name-that-exceeds-the-maximum-allowed-length-of-64-characters"},
			want: "very-long-role-session-name-that-exceeds-the-maximum-allowed-len",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, sanitizeRoleSessionName(tt.args.s), "sanitizeRoleSessionName(%v)", tt.args.s)
		})
	}
}

func TestNewAssumeRoleIdentity_InvalidInputs(t *testing.T) {
	// Empty name.
	_, err := NewAssumeRoleIdentity("", &schema.Identity{Kind: "aws/assume-role"})
	assert.Error(t, err)

	// Nil config.
	_, err = NewAssumeRoleIdentity("role", nil)
	assert.Error(t, err)
}

func TestAssumeRoleIdentity_Validate_SetsRegion(t *testing.T) {
	i := &assumeRoleIdentity{name: "role", config: &schema.Identity{
		Kind: "aws/assume-role",
		Principal: map[string]any{
			"assume_role": "arn:aws:iam::123456789012:role/Dev",
			"region":      "us-west-2",
		},
	}}
	require.NoError(t, i.Validate())
	assert.Equal(t, "us-west-2", i.region)
}

func TestAssumeRoleIdentity_newSTSClient_RegionFallbackAndPersist(t *testing.T) {
	// This test requires AWS credentials to create an STS client
	tests.RequireAWSProfile(t, "cplive-core-gbl-identity")

	// If identity.region and base.Region are empty, default to us-east-1 and persist.
	i := &assumeRoleIdentity{name: "role", config: &schema.Identity{Kind: "aws/assume-role", Principal: map[string]any{"assume_role": "arn:aws:iam::123:role/x"}}}
	base := &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "SECRET"}
	_, err := i.newSTSClient(context.Background(), base)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", i.region)
}

func TestAssumeRoleIdentity_sanitizeRoleSessionName_EdgeCases(t *testing.T) {
	// Trailing dashes are trimmed.
	assert.Equal(t, "abc", sanitizeRoleSessionName("abc---"))
	// All invalid -> becomes atmos-session.
	assert.Equal(t, "atmos-session", sanitizeRoleSessionName("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"))
}

func TestAssumeRoleIdentity_PostAuthenticate_SetsEnvAndFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	i := &assumeRoleIdentity{name: "role", config: &schema.Identity{Kind: "aws/assume-role", Principal: map[string]any{
		"assume_role": "arn:aws:iam::123:role/x",
	}}}
	authContext := &schema.AuthContext{}
	stack := &schema.ConfigAndStacksInfo{}
	creds := &types.AWSCredentials{AccessKeyID: "AK", SecretAccessKey: "SE", SessionToken: "TK", Region: "us-east-1"}
	err := i.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext:  authContext,
		StackInfo:    stack,
		ProviderName: "aws-sso",
		IdentityName: "role",
		Credentials:  creds,
	})
	require.NoError(t, err)

	// Auth context populated.
	require.NotNil(t, authContext.AWS)
	assert.Equal(t, "role", authContext.AWS.Profile)

	// AWS files/env are set under the provider namespace (derived from auth context).
	require.Contains(t, stack.ComponentEnvSection["AWS_SHARED_CREDENTIALS_FILE"], "aws-sso")
}

func TestAssumeRoleIdentity_toAWSCredentials_DefaultRegion(t *testing.T) {
	// When i.region is empty, AWSCreds should serialize with default region.
	i := &assumeRoleIdentity{name: "role", region: ""}
	out := &sts.AssumeRoleOutput{Credentials: &ststypes.Credentials{
		AccessKeyId:     aws.String("AKIA"),
		SecretAccessKey: aws.String("SECRET"),
		SessionToken:    aws.String("TOKEN"),
	}}
	c, err := i.toAWSCredentials(out)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", c.(*types.AWSCredentials).Region)
}

func TestAssumeRoleIdentity_Authenticate_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		identity    *assumeRoleIdentity
		inputCreds  types.ICredentials
		expectError bool
		errorMsg    string
	}{
		{
			name: "nil input credentials",
			identity: &assumeRoleIdentity{
				name:    "test-role",
				config:  &schema.Identity{Kind: "aws/assume-role", Principal: map[string]any{"assume_role": "arn:aws:iam::123456789012:role/TestRole"}},
				roleArn: "arn:aws:iam::123456789012:role/TestRole",
			},
			inputCreds:  nil,
			expectError: true,
			errorMsg:    "base AWS credentials or OIDC credentials are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.identity.Authenticate(context.Background(), tt.inputCreds)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAssumeRoleIdentity_Authenticate_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		identity    *assumeRoleIdentity
		expectError bool
		errorMsg    string
	}{
		{
			name: "missing role ARN in principal",
			identity: &assumeRoleIdentity{
				name:   "test-role",
				config: &schema.Identity{Kind: "aws/assume-role", Principal: map[string]any{}},
			},
			expectError: true,
			errorMsg:    "assume_role is required in principal",
		},
		{
			name: "nil principal",
			identity: &assumeRoleIdentity{
				name:   "test-role",
				config: &schema.Identity{Kind: "aws/assume-role", Principal: nil},
			},
			expectError: true,
			errorMsg:    "principal is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputCreds := &types.AWSCredentials{
				AccessKeyID:     "AKIAEXAMPLE",
				SecretAccessKey: "basesecret",
				SessionToken:    "basetoken",
				Region:          "us-east-1",
			}

			_, err := tt.identity.Authenticate(context.Background(), inputCreds)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAssumeRoleIdentity_WithCustomResolver(t *testing.T) {
	// Test assume role identity with custom resolver configuration.
	config := &schema.Identity{
		Kind: "aws/assume-role",
		Via:  &schema.IdentityVia{Provider: "test-provider"},
		Principal: map[string]interface{}{
			"assume_role": "arn:aws:iam::123456789012:role/TestRole",
			"region":      "us-east-1",
		},
		Credentials: map[string]interface{}{
			"aws": map[string]interface{}{
				"resolver": map[string]interface{}{
					"url": "http://localhost:4566",
				},
			},
		},
	}

	identity, err := NewAssumeRoleIdentity("test-role", config)
	require.NoError(t, err)
	assert.NotNil(t, identity)

	// Cast to concrete type to verify internal state.
	ari, ok := identity.(*assumeRoleIdentity)
	require.True(t, ok)
	assert.Equal(t, "test-role", ari.name)
	assert.NotNil(t, ari.config)
	assert.NotNil(t, ari.config.Credentials)

	// Verify resolver config exists.
	awsCreds, ok := ari.config.Credentials["aws"]
	assert.True(t, ok)
	assert.NotNil(t, awsCreds)
}

func TestAssumeRoleIdentity_WithoutCustomResolver(t *testing.T) {
	// Test assume role identity without custom resolver configuration.
	config := &schema.Identity{
		Kind: "aws/assume-role",
		Via:  &schema.IdentityVia{Provider: "test-provider"},
		Principal: map[string]interface{}{
			"assume_role": "arn:aws:iam::123456789012:role/TestRole",
			"region":      "us-east-1",
		},
	}

	identity, err := NewAssumeRoleIdentity("test-role", config)
	require.NoError(t, err)
	assert.NotNil(t, identity)

	// Verify it works without resolver config.
	assert.NoError(t, identity.Validate())
}

func TestAssumeRoleIdentity_newSTSClient_WithResolver(t *testing.T) {
	// This test requires AWS credentials to create an STS client.
	tests.RequireAWSProfile(t, "cplive-core-gbl-identity")

	// Test newSTSClient with custom resolver.
	config := &schema.Identity{
		Kind: "aws/assume-role",
		Via:  &schema.IdentityVia{Provider: "test-provider"},
		Principal: map[string]interface{}{
			"assume_role": "arn:aws:iam::123456789012:role/TestRole",
			"region":      "us-east-1",
		},
		Credentials: map[string]interface{}{
			"aws": map[string]interface{}{
				"resolver": map[string]interface{}{
					"url": "http://localhost:4566",
				},
			},
		},
	}

	identity, err := NewAssumeRoleIdentity("test-role", config)
	require.NoError(t, err)

	ari, ok := identity.(*assumeRoleIdentity)
	require.True(t, ok)

	// Create base credentials.
	baseCreds := &types.AWSCredentials{
		AccessKeyID:     "AKIAEXAMPLE",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Region:          "us-east-1",
	}

	// Call newSTSClient - this should not error even with custom resolver.
	client, err := ari.newSTSClient(context.Background(), baseCreds)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestAssumeRoleIdentity_newSTSClient_WithoutResolver(t *testing.T) {
	// This test requires AWS credentials to create an STS client.
	tests.RequireAWSProfile(t, "cplive-core-gbl-identity")

	// Test newSTSClient without custom resolver.
	config := &schema.Identity{
		Kind: "aws/assume-role",
		Via:  &schema.IdentityVia{Provider: "test-provider"},
		Principal: map[string]interface{}{
			"assume_role": "arn:aws:iam::123456789012:role/TestRole",
			"region":      "us-east-1",
		},
	}

	identity, err := NewAssumeRoleIdentity("test-role", config)
	require.NoError(t, err)

	ari, ok := identity.(*assumeRoleIdentity)
	require.True(t, ok)

	// Create base credentials.
	baseCreds := &types.AWSCredentials{
		AccessKeyID:     "AKIAEXAMPLE",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Region:          "us-east-1",
	}

	// Call newSTSClient - should work without resolver.
	client, err := ari.newSTSClient(context.Background(), baseCreds)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestAssumeRoleIdentity_newSTSClient_RegionResolution(t *testing.T) {
	// This test requires AWS credentials to create an STS client.
	tests.RequireAWSProfile(t, "cplive-core-gbl-identity")

	testCases := []struct {
		name           string
		identityRegion string
		baseRegion     string
		expectedRegion string
	}{
		{
			name:           "uses identity region when set",
			identityRegion: "eu-west-1",
			baseRegion:     "us-east-1",
			expectedRegion: "eu-west-1",
		},
		{
			name:           "falls back to base region",
			identityRegion: "",
			baseRegion:     "ap-south-1",
			expectedRegion: "ap-south-1",
		},
		{
			name:           "defaults to us-east-1",
			identityRegion: "",
			baseRegion:     "",
			expectedRegion: "us-east-1",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			config := &schema.Identity{
				Kind: "aws/assume-role",
				Via:  &schema.IdentityVia{Provider: "test-provider"},
				Principal: map[string]interface{}{
					"assume_role": "arn:aws:iam::123456789012:role/TestRole",
				},
			}

			if tt.identityRegion != "" {
				config.Principal["region"] = tt.identityRegion
			}

			identity, err := NewAssumeRoleIdentity("test-role", config)
			require.NoError(t, err)

			ari, ok := identity.(*assumeRoleIdentity)
			require.True(t, ok)

			// Validate to extract region from principal.
			err = ari.Validate()
			require.NoError(t, err)

			baseCreds := &types.AWSCredentials{
				AccessKeyID:     "AKIAEXAMPLE",
				SecretAccessKey: "secret",
				SessionToken:    "token",
				Region:          tt.baseRegion,
			}

			client, err := ari.newSTSClient(context.Background(), baseCreds)
			assert.NoError(t, err)
			assert.NotNil(t, client)
			// Verify region was persisted.
			assert.Equal(t, tt.expectedRegion, ari.region)
		})
	}
}

func TestAssumeRoleIdentity_Logout(t *testing.T) {
	// Test that assume-role identity Logout returns nil (no identity-specific cleanup).
	identity, err := NewAssumeRoleIdentity("test-role", &schema.Identity{
		Kind: "aws/assume-role",
		Principal: map[string]interface{}{
			"assume_role": "arn:aws:iam::123456789012:role/MyRole",
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = identity.Logout(ctx)

	// Should always succeed with no cleanup.
	assert.NoError(t, err)
}

func TestAssumeRoleIdentity_CredentialsExist(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name           string
		setupFiles     bool
		expectedExists bool
	}{
		{
			name:           "credentials file exists",
			setupFiles:     true,
			expectedExists: true,
		},
		{
			name:           "credentials file does not exist",
			setupFiles:     false,
			expectedExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity, err := NewAssumeRoleIdentity("test-role", &schema.Identity{
				Kind: "aws/assume-role",
				Via:  &schema.IdentityVia{Provider: "aws-sso"},
				Principal: map[string]interface{}{
					"assume_role": "arn:aws:iam::123456789012:role/MyRole",
				},
			})
			require.NoError(t, err)

			if tt.setupFiles {
				t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)
				// assume_role uses the Via.Provider name, which is "aws-sso" in our test config
				credPath := filepath.Join(tmpDir, "atmos", "aws", "aws-sso", "credentials")
				require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
				require.NoError(t, os.WriteFile(credPath, []byte("[test-role]\naws_access_key_id=test\n"), 0o600))
			} else {
				t.Setenv("ATMOS_XDG_CONFIG_HOME", filepath.Join(tmpDir, "nonexistent"))
			}

			exists, err := identity.CredentialsExist()
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedExists, exists)
		})
	}
}

func TestAssumeRoleIdentity_LoadCredentials(t *testing.T) {
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
			// Unset AWS environment variables that might interfere with test isolation.
			t.Setenv("AWS_REGION", "")
			t.Setenv("AWS_DEFAULT_REGION", "")

			identity, err := NewAssumeRoleIdentity("test-role", &schema.Identity{
				Kind: "aws/assume-role",
				Via:  &schema.IdentityVia{Provider: "aws-sso"},
				Principal: map[string]interface{}{
					"assume_role": "arn:aws:iam::123456789012:role/MyRole",
				},
			})
			require.NoError(t, err)

			if tt.setupFiles {
				t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)

				// Create credentials file - assume_role uses Via.Provider name.
				credPath := filepath.Join(tmpDir, "atmos", "aws", "aws-sso", "credentials")
				require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
				credContent := `[test-role]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
aws_session_token = FwoGZXIvYXdzEBExample
`
				require.NoError(t, os.WriteFile(credPath, []byte(credContent), 0o600))

				// Create config file.
				configPath := filepath.Join(tmpDir, "atmos", "aws", "aws-sso", "config")
				configContent := `[profile test-role]
region = ap-south-1
output = json
`
				require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))
			} else {
				t.Setenv("ATMOS_XDG_CONFIG_HOME", filepath.Join(tmpDir, "nonexistent"))
			}

			ctx := context.Background()
			creds, err := identity.LoadCredentials(ctx)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, creds)
			} else {
				require.NoError(t, err)
				require.NotNil(t, creds)

				awsCreds, ok := creds.(*types.AWSCredentials)
				require.True(t, ok, "credentials should be AWSCredentials type")
				assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", awsCreds.AccessKeyID)
				assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", awsCreds.SecretAccessKey)
				assert.Equal(t, "FwoGZXIvYXdzEBExample", awsCreds.SessionToken)
				assert.Equal(t, "ap-south-1", awsCreds.Region)
			}
		})
	}
}

// OIDC Web Identity Tests

func TestAssumeRoleIdentity_Authenticate_WithOIDCCredentials(t *testing.T) {
	// Test that Authenticate detects OIDC credentials and uses AssumeRoleWithWebIdentity.
	identity := &assumeRoleIdentity{
		name: "test-role",
		config: &schema.Identity{
			Kind: "aws/assume-role",
			Via:  &schema.IdentityVia{Provider: "github-oidc"},
			Principal: map[string]interface{}{
				"assume_role": "arn:aws:iam::123456789012:role/GitHubActionsRole",
				"region":      "us-east-1",
			},
		},
	}

	// Validate to populate roleArn and region.
	require.NoError(t, identity.Validate())

	oidcCreds := &types.OIDCCredentials{
		Token:    "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJyZXBvOm93bmVyL3JlcG86cmVmOnJlZnMvaGVhZHMvbWFpbiIsImF1ZCI6InN0czphbWF6b25hd3MuY29tIiwiZXhwIjo5OTk5OTk5OTk5fQ.test",
		Provider: "github",
		Audience: "sts:amazonaws.com",
	}

	// We can't actually call AWS without real credentials, but we can verify the flow is triggered.
	// We'll test the helper methods separately.
	_, err := identity.Authenticate(context.Background(), oidcCreds)
	// Expect an error since we don't have real AWS connectivity, but it should be an AWS error, not a type error.
	assert.Error(t, err)
	// The error should not be about credentials type.
	assert.NotContains(t, err.Error(), "base AWS credentials or OIDC credentials are required")
}

func TestAssumeRoleIdentity_buildAssumeRoleWithWebIdentityInput(t *testing.T) {
	tests := []struct {
		name           string
		identityName   string
		principal      map[string]interface{}
		oidcToken      string
		expectDuration bool
		durationSecs   int32
	}{
		{
			name:         "basic input without duration",
			identityName: "github-role",
			principal: map[string]interface{}{
				"assume_role": "arn:aws:iam::123456789012:role/GitHubActionsRole",
			},
			oidcToken:      "test-token",
			expectDuration: false,
		},
		{
			name:         "input with duration",
			identityName: "github-role",
			principal: map[string]interface{}{
				"assume_role": "arn:aws:iam::123456789012:role/GitHubActionsRole",
				"duration":    "1h",
			},
			oidcToken:      "test-token",
			expectDuration: true,
			durationSecs:   3600,
		},
		{
			name:         "input with invalid duration (ignored)",
			identityName: "github-role",
			principal: map[string]interface{}{
				"assume_role": "arn:aws:iam::123456789012:role/GitHubActionsRole",
				"duration":    "invalid",
			},
			oidcToken:      "test-token",
			expectDuration: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity := &assumeRoleIdentity{
				name: tt.identityName,
				config: &schema.Identity{
					Kind:      "aws/assume-role",
					Principal: tt.principal,
				},
			}

			// Validate to populate roleArn.
			require.NoError(t, identity.Validate())

			oidcCreds := &types.OIDCCredentials{
				Token: tt.oidcToken,
			}

			input := identity.buildAssumeRoleWithWebIdentityInput(oidcCreds)

			assert.NotNil(t, input)
			assert.Equal(t, "arn:aws:iam::123456789012:role/GitHubActionsRole", *input.RoleArn)
			assert.Equal(t, tt.oidcToken, *input.WebIdentityToken)
			assert.NotEmpty(t, *input.RoleSessionName)
			assert.Contains(t, *input.RoleSessionName, "atmos-"+tt.identityName)

			if tt.expectDuration {
				require.NotNil(t, input.DurationSeconds)
				assert.Equal(t, tt.durationSecs, *input.DurationSeconds)
			} else {
				assert.Nil(t, input.DurationSeconds)
			}
		})
	}
}

func TestAssumeRoleIdentity_toAWSCredentialsFromWebIdentity(t *testing.T) {
	identity := &assumeRoleIdentity{
		name:   "github-role",
		region: "us-west-2",
	}

	tests := []struct {
		name        string
		input       *sts.AssumeRoleWithWebIdentityOutput
		expectError bool
		expectCreds bool
	}{
		{
			name:        "nil result",
			input:       nil,
			expectError: true,
			expectCreds: false,
		},
		{
			name: "nil credentials",
			input: &sts.AssumeRoleWithWebIdentityOutput{
				Credentials: nil,
			},
			expectError: true,
			expectCreds: false,
		},
		{
			name: "valid credentials",
			input: &sts.AssumeRoleWithWebIdentityOutput{
				Credentials: &ststypes.Credentials{
					AccessKeyId:     aws.String("AKIAIOSFODNN7EXAMPLE"),
					SecretAccessKey: aws.String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
					SessionToken:    aws.String("FwoGZXIvYXdzEBExample"),
					Expiration:      aws.Time(time.Now().Add(1 * time.Hour)),
				},
			},
			expectError: false,
			expectCreds: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, err := identity.toAWSCredentialsFromWebIdentity(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, creds)
			} else {
				require.NoError(t, err)
				require.NotNil(t, creds)

				if tt.expectCreds {
					awsCreds, ok := creds.(*types.AWSCredentials)
					require.True(t, ok)
					assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", awsCreds.AccessKeyID)
					assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", awsCreds.SecretAccessKey)
					assert.Equal(t, "FwoGZXIvYXdzEBExample", awsCreds.SessionToken)
					assert.Equal(t, "us-west-2", awsCreds.Region)
					assert.NotEmpty(t, awsCreds.Expiration)
				}
			}
		})
	}
}

func TestAssumeRoleIdentity_toAWSCredentialsFromWebIdentity_DefaultRegion(t *testing.T) {
	// When region is empty, should default to us-east-1.
	identity := &assumeRoleIdentity{
		name:   "github-role",
		region: "",
	}

	output := &sts.AssumeRoleWithWebIdentityOutput{
		Credentials: &ststypes.Credentials{
			AccessKeyId:     aws.String("AKIAIOSFODNN7EXAMPLE"),
			SecretAccessKey: aws.String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
			SessionToken:    aws.String("FwoGZXIvYXdzEBExample"),
			Expiration:      aws.Time(time.Now().Add(1 * time.Hour)),
		},
	}

	creds, err := identity.toAWSCredentialsFromWebIdentity(output)
	require.NoError(t, err)

	awsCreds, ok := creds.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "us-east-1", awsCreds.Region)
}

func TestAssumeRoleIdentity_Authenticate_WithOIDCAndAWSCredentials(t *testing.T) {
	// Test that both OIDC and AWS credentials work with the same identity.
	identity := &assumeRoleIdentity{
		name: "test-role",
		config: &schema.Identity{
			Kind: "aws/assume-role",
			Via:  &schema.IdentityVia{Provider: "test-provider"},
			Principal: map[string]interface{}{
				"assume_role": "arn:aws:iam::123456789012:role/TestRole",
				"region":      "us-east-1",
			},
		},
	}

	require.NoError(t, identity.Validate())

	// Test with OIDC credentials - should trigger web identity flow.
	oidcCreds := &types.OIDCCredentials{
		Token:    "test-oidc-token",
		Provider: "github",
		Audience: "sts:amazonaws.com",
	}

	_, err := identity.Authenticate(context.Background(), oidcCreds)
	// Expect AWS error (no real connectivity), not type error.
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "base AWS credentials or OIDC credentials are required")

	// Test with AWS credentials - should trigger standard assume role flow.
	awsCreds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "FwoGZXIvYXdzEBExample",
		Region:          "us-east-1",
	}

	_, err = identity.Authenticate(context.Background(), awsCreds)
	// Expect AWS error (no real connectivity), not type error.
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "base AWS credentials or OIDC credentials are required")
}

// mockInvalidCredentials is a mock credentials type that implements ICredentials
// but is neither AWSCredentials nor OIDCCredentials, for testing error handling.
type mockInvalidCredentials struct{}

func (m *mockInvalidCredentials) IsExpired() bool                        { return false }
func (m *mockInvalidCredentials) GetExpiration() (*time.Time, error)     { return nil, nil }
func (m *mockInvalidCredentials) BuildWhoamiInfo(info *types.WhoamiInfo) {}
func (m *mockInvalidCredentials) Validate(ctx context.Context) (*types.ValidationInfo, error) {
	return nil, nil
}

var _ types.ICredentials = (*mockInvalidCredentials)(nil)

func TestAssumeRoleIdentity_Authenticate_WithInvalidCredentialsType(t *testing.T) {
	// Test that invalid credentials type returns appropriate error.
	identity := &assumeRoleIdentity{
		name: "test-role",
		config: &schema.Identity{
			Kind: "aws/assume-role",
			Via:  &schema.IdentityVia{Provider: "test-provider"},
			Principal: map[string]interface{}{
				"assume_role": "arn:aws:iam::123456789012:role/TestRole",
				"region":      "us-east-1",
			},
		},
	}

	require.NoError(t, identity.Validate())

	// Create a mock credentials type that's neither AWS nor OIDC.
	invalidCreds := &mockInvalidCredentials{}

	_, err := identity.Authenticate(context.Background(), invalidCreds)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "base AWS credentials or OIDC credentials are required")
}
