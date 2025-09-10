package aws

import (
    "context"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/cloudposse/atmos/internal/auth/types"
    "github.com/cloudposse/atmos/pkg/schema"
)

func TestNewUserIdentity_And_GetProviderName(t *testing.T) {
    // Wrong kind should error
    _, err := NewUserIdentity("me", &schema.Identity{Kind: "aws/assume-role"})
    assert.Error(t, err)

    // Correct kind
    id, err := NewUserIdentity("me", &schema.Identity{Kind: "aws/user"})
    assert.NoError(t, err)
    assert.NotNil(t, id)
    assert.Equal(t, "aws/user", id.Kind())

    // Provider name is constant
    name, err := id.GetProviderName()
    assert.NoError(t, err)
    assert.Equal(t, "aws-user", name)
}

func TestUserIdentity_Environment(t *testing.T) {
    // Environment should include AWS files and pass through additional env from config
    id, err := NewUserIdentity("dev", &schema.Identity{Kind: "aws/user", Env: []schema.EnvironmentVariable{{Key: "FOO", Value: "BAR"}}})
    require.NoError(t, err)
    env, err := id.Environment()
    require.NoError(t, err)

    // Contains the three AWS_* vars and our custom one
    assert.NotEmpty(t, env["AWS_SHARED_CREDENTIALS_FILE"])
    assert.NotEmpty(t, env["AWS_CONFIG_FILE"])
    // points under ~/.aws/atmos/aws-user
    assert.Contains(t, env["AWS_SHARED_CREDENTIALS_FILE"], filepath.Join(".aws", "atmos", "aws-user"))
    assert.Contains(t, env["AWS_CONFIG_FILE"], filepath.Join(".aws", "atmos", "aws-user"))
    assert.Equal(t, "BAR", env["FOO"])
}

func TestIsStandaloneAWSUserChain(t *testing.T) {
    // Not standalone when multiple elements
    assert.False(t, IsStandaloneAWSUserChain([]string{"p", "dev"}, map[string]schema.Identity{"dev": {Kind: "aws/user"}}))

    // Single element but wrong kind -> false
    assert.False(t, IsStandaloneAWSUserChain([]string{"dev"}, map[string]schema.Identity{"dev": {Kind: "aws/permission-set"}}))

    // Single element and aws/user -> true
    assert.True(t, IsStandaloneAWSUserChain([]string{"dev"}, map[string]schema.Identity{"dev": {Kind: "aws/user"}}))
}

// stubUser satisfies types.Identity for testing AuthenticateStandaloneAWSUser
type stubUser struct{ creds types.ICredentials }

func (s stubUser) Kind() string { return "aws/user" }
func (s stubUser) GetProviderName() (string, error) { return "aws-user", nil }
func (s stubUser) Authenticate(_ context.Context, _ types.ICredentials) (types.ICredentials, error) {
    return s.creds, nil
}
func (s stubUser) Validate() error { return nil }
func (s stubUser) Environment() (map[string]string, error) { return map[string]string{}, nil }
func (s stubUser) PostAuthenticate(_ context.Context, _ *schema.ConfigAndStacksInfo, _ string, _ string, _ types.ICredentials) error {
    return nil
}

func TestAuthenticateStandaloneAWSUser(t *testing.T) {
    // Not found -> error
    _, err := AuthenticateStandaloneAWSUser(context.Background(), "missing", map[string]types.Identity{})
    assert.Error(t, err)

    // Found -> returns credentials from identity implementation
    out, err := AuthenticateStandaloneAWSUser(context.Background(), "dev", map[string]types.Identity{
        "dev": stubUser{creds: &types.AWSCredentials{AccessKeyID: "AKIA", Region: "us-east-1"}},
    })
    require.NoError(t, err)
    assert.Equal(t, "AKIA", out.(*types.AWSCredentials).AccessKeyID)
}

