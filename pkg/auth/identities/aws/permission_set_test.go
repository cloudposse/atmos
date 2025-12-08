package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

func TestNewPermissionSetIdentity(t *testing.T) {
	// Wrong kind should error.
	_, err := NewPermissionSetIdentity("dev", &schema.Identity{Kind: "aws/assume-role"})
	assert.Error(t, err)

	// Correct kind succeeds.
	id, err := NewPermissionSetIdentity("dev", &schema.Identity{Kind: "aws/permission-set"})
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.Equal(t, "aws/permission-set", id.Kind())
}

func TestPermissionSetIdentity_Validate_Principal(t *testing.T) {
	i := &permissionSetIdentity{name: "dev", config: &schema.Identity{Kind: "aws/permission-set"}}
	assert.Error(t, i.Validate())

	// Missing account -> error.
	i = &permissionSetIdentity{name: "dev", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{"name": "DevAccess"}}}
	assert.Error(t, i.Validate())

	// Missing name and id -> error.
	i = &permissionSetIdentity{name: "dev", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{
		"account": map[string]any{},
	}}}
	assert.Error(t, i.Validate())

	// Valid principal.
	i = &permissionSetIdentity{name: "dev", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{
		"name":    "DevAccess",
		"account": map[string]any{"name": "dev"},
	}}}
	assert.NoError(t, i.Validate())
}

func TestPermissionSetIdentity_Getters(t *testing.T) {
	i := &permissionSetIdentity{name: "dev", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{
		"name":    "DevAccess",
		"account": map[string]any{"id": "123456789012"},
	}, Via: &schema.IdentityVia{Provider: "aws-sso"}}}

	// Provider name.
	p, err := i.GetProviderName()
	assert.NoError(t, err)
	assert.Equal(t, "aws-sso", p)

	// Env passthrough.
	i.config.Env = []schema.EnvironmentVariable{{Key: "X", Value: "Y"}}
	env, err := i.Environment()
	assert.NoError(t, err)
	assert.Equal(t, "Y", env["X"])
}

func TestPermissionSetIdentity_Extractors(t *testing.T) {
	i := &permissionSetIdentity{name: "dev", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{
		"name":    "DevAccess",
		"account": map[string]any{"name": "dev"},
	}}}

	name, err := i.getPermissionSetName()
	assert.NoError(t, err)
	assert.Equal(t, "DevAccess", name)

	accName, accID, err := i.getAccountDetails()
	assert.NoError(t, err)
	assert.Equal(t, "dev", accName)
	assert.Equal(t, "", accID) // not set

	// Missing account.
	j := &permissionSetIdentity{name: "bad", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{"name": "X"}}}
	_, _, err = j.getAccountDetails()
	assert.Error(t, err)

	// Empty account map -> error from extractor.
	k := &permissionSetIdentity{name: "bad", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{
		"name":    "X",
		"account": map[string]any{},
	}}}
	_, _, err = k.getAccountDetails()
	assert.Error(t, err)
}

func TestPermissionSetIdentity_buildCredsFromRole(t *testing.T) {
	i := &permissionSetIdentity{name: "dev"}

	// Nil role creds -> error.
	_, err := i.buildCredsFromRole(&sso.GetRoleCredentialsOutput{}, "us-east-1")
	assert.Error(t, err)

	// Valid conversion.
	expMs := time.Now().Add(2 * time.Hour).UnixMilli()
	out := &sso.GetRoleCredentialsOutput{RoleCredentials: &ssotypes.RoleCredentials{
		AccessKeyId:     aws.String("AKIAxyz"),
		SecretAccessKey: aws.String("secret"),
		SessionToken:    aws.String("token"),
		Expiration:      expMs,
	}}
	creds, err := i.buildCredsFromRole(out, "eu-west-1")
	require.NoError(t, err)
	assert.Equal(t, "eu-west-1", creds.Region)
}

func TestPermissionSetIdentity_GetProviderName_Error(t *testing.T) {
	i := &permissionSetIdentity{name: "dev", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{
		"name":    "DevAccess",
		"account": map[string]any{"id": "123"},
	}}}
	_, err := i.GetProviderName()
	assert.Error(t, err)
}

func TestPermissionSetIdentity_getPermissionSetName_Error(t *testing.T) {
	i := &permissionSetIdentity{name: "dev", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{
		"account": map[string]any{"id": "123"},
	}}}
	_, err := i.getPermissionSetName()
	assert.Error(t, err)
}

func TestPermissionSetIdentity_newSSOClient_Success(t *testing.T) {
	// This test requires AWS credentials to create an SSO client.
	tests.RequireAWSProfile(t, "cplive-core-gbl-identity")

	i := &permissionSetIdentity{name: "dev"}
	// AccessKeyID is used as an access token by newSSOClient; no network happens here.
	base := &types.AWSCredentials{AccessKeyID: "access-token", Region: "us-east-1"}
	cli, err := i.newSSOClient(t.Context(), base)
	require.NoError(t, err)
	require.NotNil(t, cli)
}

// Note: resolveAccountID and PostAuthenticate are covered in permission_set_more_test.go.

func TestPermissionSetIdentity_GetProviderName_ErrorWhenMissing(t *testing.T) {
	i := &permissionSetIdentity{name: "dev", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{
		"name":    "DevAccess",
		"account": map[string]any{"id": "123456789012"},
	}}}
	_, err := i.GetProviderName()
	assert.Error(t, err)
}

func TestPermissionSetIdentity_buildCredsFromRole_EmptyExpiration(t *testing.T) {
	i := &permissionSetIdentity{name: "dev"}
	out := &sso.GetRoleCredentialsOutput{RoleCredentials: &ssotypes.RoleCredentials{
		AccessKeyId:     aws.String("AKIAxyz"),
		SecretAccessKey: aws.String("secret"),
		SessionToken:    aws.String("token"),
		Expiration:      0,
	}}
	creds, err := i.buildCredsFromRole(out, "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "", creds.Expiration)
}

func TestPermissionSetIdentity_PostAuthenticate_WritesFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	i := &permissionSetIdentity{name: "dev", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{
		"name":    "DevAccess",
		"account": map[string]any{"id": "123456789012"},
	}}}
	authContext := &schema.AuthContext{}
	stack := &schema.ConfigAndStacksInfo{}
	creds := &types.AWSCredentials{AccessKeyID: "AK", SecretAccessKey: "SE", SessionToken: "TK", Region: "us-east-1"}
	err := i.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext:  authContext,
		StackInfo:    stack,
		ProviderName: "aws-sso",
		IdentityName: "dev",
		Credentials:  creds,
	})
	require.NoError(t, err)

	// Auth context populated.
	require.NotNil(t, authContext.AWS)
	assert.Equal(t, "dev", authContext.AWS.Profile)
	assert.Equal(t, "us-east-1", authContext.AWS.Region)

	// Env set on stack (derived from auth context).
	// XDG path contains "atmos/aws/aws-sso/credentials"
	assert.Contains(t, stack.ComponentEnvSection["AWS_SHARED_CREDENTIALS_FILE"], filepath.Join("atmos", "aws", "aws-sso", "credentials"))
	assert.Equal(t, "dev", stack.ComponentEnvSection["AWS_PROFILE"])
}

func TestPermissionSetIdentity_resolveAccountID_ReturnsProvidedID(t *testing.T) {
	i := &permissionSetIdentity{name: "dev"}
	id, err := i.resolveAccountID(context.Background(), nil, "", "123456789012", "token")
	require.NoError(t, err)
	assert.Equal(t, "123456789012", id)
}

func TestPermissionSetIdentity_Logout(t *testing.T) {
	// Test that permission-set identity Logout returns nil (no identity-specific cleanup).
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = identity.Logout(ctx)

	// Should always succeed with no cleanup.
	assert.NoError(t, err)
}

func TestPermissionSetIdentity_CredentialsExist(t *testing.T) {
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
			identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
				Kind: "aws/permission-set",
				Via:  &schema.IdentityVia{Provider: "aws-sso"},
				Principal: map[string]interface{}{
					"name":    "DevAccess",
					"account": map[string]interface{}{"id": "123456789012"},
				},
			})
			require.NoError(t, err)

			// Set root provider name so the identity can resolve file paths.
			psIdentity := identity.(*permissionSetIdentity)
			psIdentity.SetManagerAndProvider(nil, "aws-sso")

			if tt.setupFiles {
				t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)
				credPath := filepath.Join(tmpDir, "atmos", "aws", "aws-sso", "credentials")
				require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
				require.NoError(t, os.WriteFile(credPath, []byte("[test-ps]\naws_access_key_id=test\n"), 0o600))
			} else {
				t.Setenv("ATMOS_XDG_CONFIG_HOME", filepath.Join(tmpDir, "nonexistent"))
			}

			exists, err := identity.CredentialsExist()
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedExists, exists)
		})
	}
}

func TestPermissionSetIdentity_LoadCredentials(t *testing.T) {
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
			identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
				Kind: "aws/permission-set",
				Via:  &schema.IdentityVia{Provider: "aws-sso"},
				Principal: map[string]interface{}{
					"name":    "DevAccess",
					"account": map[string]interface{}{"id": "123456789012"},
				},
			})
			require.NoError(t, err)

			// Set root provider name so the identity can resolve file paths.
			psIdentity := identity.(*permissionSetIdentity)
			psIdentity.SetManagerAndProvider(nil, "aws-sso")

			if tt.setupFiles {
				t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)

				// Create credentials file.
				credPath := filepath.Join(tmpDir, "atmos", "aws", "aws-sso", "credentials")
				require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
				credContent := `[test-ps]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
aws_session_token = FwoGZXIvYXdzEBExample
`
				require.NoError(t, os.WriteFile(credPath, []byte(credContent), 0o600))

				// Create config file.
				configPath := filepath.Join(tmpDir, "atmos", "aws", "aws-sso", "config")
				configContent := `[profile test-ps]
region = us-east-1
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
				assert.Equal(t, "us-east-1", awsCreds.Region)
			}
		})
	}
}

func TestPermissionSetIdentity_Paths(t *testing.T) {
	id, err := NewPermissionSetIdentity("dev", &schema.Identity{
		Kind: "aws/permission-set",
	})
	require.NoError(t, err)

	// Permission set identities don't add additional credential files beyond the provider.
	paths, err := id.Paths()
	assert.NoError(t, err)
	assert.Empty(t, paths, "permission set identities should not return additional paths")
}
