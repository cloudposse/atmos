package aws

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sso"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewPermissionSetIdentity(t *testing.T) {
	// Wrong kind should error
	_, err := NewPermissionSetIdentity("dev", &schema.Identity{Kind: "aws/assume-role"})
	assert.Error(t, err)

	// Correct kind succeeds
	id, err := NewPermissionSetIdentity("dev", &schema.Identity{Kind: "aws/permission-set"})
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.Equal(t, "aws/permission-set", id.Kind())
}

func TestPermissionSetIdentity_Validate_Principal(t *testing.T) {
	i := &permissionSetIdentity{name: "dev", config: &schema.Identity{Kind: "aws/permission-set"}}
	assert.Error(t, i.Validate())

	// Missing account -> error
	i = &permissionSetIdentity{name: "dev", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{"name": "DevAccess"}}}
	assert.Error(t, i.Validate())

	// Missing name and id -> error
	i = &permissionSetIdentity{name: "dev", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{
		"account": map[string]any{},
	}}}
	assert.Error(t, i.Validate())

	// Valid principal
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

	// Provider name
	p, err := i.GetProviderName()
	assert.NoError(t, err)
	assert.Equal(t, "aws-sso", p)

	// Env passthrough
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

	// Missing account
	j := &permissionSetIdentity{name: "bad", config: &schema.Identity{Kind: "aws/permission-set", Principal: map[string]any{"name": "X"}}}
	_, _, err = j.getAccountDetails()
	assert.Error(t, err)
}

func TestPermissionSetIdentity_buildCredsFromRole(t *testing.T) {
	i := &permissionSetIdentity{name: "dev"}

	// Nil role creds -> error
	_, err := i.buildCredsFromRole(&sso.GetRoleCredentialsOutput{}, "us-east-1")
	assert.Error(t, err)

	// Valid conversion
	expMs := time.Now().Add(2 * time.Hour).UnixMilli()
	out := &sso.GetRoleCredentialsOutput{RoleCredentials: &ssotypes.RoleCredentials{
		AccessKeyId:     strPtr("AKIAxyz"),
		SecretAccessKey: strPtr("secret"),
		SessionToken:    strPtr("token"),
		Expiration:      &expMs,
	}}
	creds, err := i.buildCredsFromRole(out, "eu-west-1")
	require.NoError(t, err)
	assert.Equal(t, "eu-west-1", creds.Region)
}
