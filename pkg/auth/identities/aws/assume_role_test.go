package aws

import (
	"context"
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
			errorMsg:    "base AWS credentials are required",
		},
		{
			name: "invalid credentials type",
			identity: &assumeRoleIdentity{
				name:    "test-role",
				config:  &schema.Identity{Kind: "aws/assume-role", Principal: map[string]any{"assume_role": "arn:aws:iam::123456789012:role/TestRole"}},
				roleArn: "arn:aws:iam::123456789012:role/TestRole",
			},
			inputCreds:  &types.OIDCCredentials{Token: "test"},
			expectError: true,
			errorMsg:    "base AWS credentials are required",
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
