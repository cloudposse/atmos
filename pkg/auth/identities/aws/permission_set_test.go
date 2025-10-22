package aws

import (
	"context"
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
	stack := &schema.ConfigAndStacksInfo{}
	creds := &types.AWSCredentials{AccessKeyID: "AK", SecretAccessKey: "SE", SessionToken: "TK", Region: "us-east-1"}
	err := i.PostAuthenticate(context.Background(), stack, "aws-sso", "dev", creds)
	require.NoError(t, err)
	// Env set on stack.
	assert.Contains(t, stack.ComponentEnvSection["AWS_SHARED_CREDENTIALS_FILE"], filepath.Join(".aws", "atmos", "aws-sso", "credentials"))
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
