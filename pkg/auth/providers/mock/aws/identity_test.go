package aws

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewIdentityAndKind(t *testing.T) {
	t.Parallel()

	cfg := &schema.Identity{Kind: "aws"}
	identity := NewIdentity("test/identity", cfg)
	require.NotNil(t, identity)
	assert.Equal(t, "test/identity", identity.name)
	assert.Equal(t, cfg, identity.config)
	assert.Equal(t, "aws", identity.Kind())
}

func TestIdentity_GetCredentialsFilePath_SanitizesSlash(t *testing.T) {
	t.Parallel()

	identity := NewIdentity("team/dev", &schema.Identity{Kind: "aws"})
	path := identity.getCredentialsFilePath()
	assert.Contains(t, path, "atmos-mock-team-dev.json")
	assert.NotContains(t, path, "team/dev")
}

func TestIdentity_GetProviderName(t *testing.T) {
	t.Parallel()

	identityWithoutVia := NewIdentity("identity-default", &schema.Identity{Kind: "aws"})
	providerName, err := identityWithoutVia.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "mock", providerName)

	identityWithVia := NewIdentity("identity-via", &schema.Identity{
		Kind: "aws",
		Via: &schema.IdentityVia{
			Provider: "mock-provider",
		},
	})
	providerName, err = identityWithVia.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "mock-provider", providerName)
}

func TestIdentity_AuthenticateAndEnvironment(t *testing.T) {
	t.Parallel()

	identity := NewIdentity("dev-identity", &schema.Identity{Kind: "aws"})
	creds, err := identity.Authenticate(context.Background(), nil)
	require.NoError(t, err)

	mockCreds, ok := creds.(*Credentials)
	require.True(t, ok)
	assert.Equal(t, "MOCK_KEY_dev-identity", mockCreds.AccessKeyID)
	assert.Equal(t, "MOCK_SECRET_dev-identity", mockCreds.SecretAccessKey)
	assert.Equal(t, "MOCK_TOKEN_dev-identity", mockCreds.SessionToken)
	assert.Equal(t, MockRegion, mockCreds.Region)
	assert.Equal(t, MockExpirationYear, mockCreds.Expiration.Year())

	env, err := identity.Environment()
	require.NoError(t, err)
	assert.Equal(t, "dev-identity", env["MOCK_IDENTITY"])
	assert.Equal(t, "dev-identity", env["AWS_PROFILE"])
	assert.Equal(t, MockRegion, env["AWS_REGION"])
	assert.Equal(t, MockRegion, env["AWS_DEFAULT_REGION"])
}

func TestIdentity_PrepareEnvironment(t *testing.T) {
	t.Parallel()

	identity := NewIdentity("prepared", &schema.Identity{Kind: "aws"})
	input := map[string]string{"EXISTING": "value"}
	output, err := identity.PrepareEnvironment(context.Background(), input)
	require.NoError(t, err)

	assert.Equal(t, "value", output["EXISTING"])
	assert.Equal(t, "prepared", output["ATMOS_IDENTITY"])
	assert.Equal(t, "prepared", output["AWS_PROFILE"])
	assert.Equal(t, "/tmp/mock-credentials", output["AWS_SHARED_CREDENTIALS_FILE"])
	assert.Equal(t, "/tmp/mock-config", output["AWS_CONFIG_FILE"])
	assert.Equal(t, MockRegion, output["AWS_REGION"])
}

func TestIdentity_LoadCredentialsAndLifecycle(t *testing.T) {
	t.Parallel()

	identity := NewIdentity("load-creds", &schema.Identity{Kind: "aws"})
	credPath := identity.getCredentialsFilePath()
	t.Cleanup(func() {
		_ = os.Remove(credPath)
	})

	exists, err := identity.CredentialsExist()
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = identity.LoadCredentials(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoStoredCredentials))
	assert.Contains(t, err.Error(), "use 'atmos auth login' to authenticate")

	validJSON := `{"AccessKeyID":"A","SecretAccessKey":"B","SessionToken":"C","Region":"us-west-2","Expiration":"2099-12-31T23:59:59Z"}`
	require.NoError(t, os.WriteFile(credPath, []byte(validJSON), MockFilePermissions))

	exists, err = identity.CredentialsExist()
	require.NoError(t, err)
	assert.True(t, exists)

	loaded, err := identity.LoadCredentials(context.Background())
	require.NoError(t, err)
	loadedCreds, ok := loaded.(*Credentials)
	require.True(t, ok)
	assert.Equal(t, "A", loadedCreds.AccessKeyID)
	assert.Equal(t, "B", loadedCreds.SecretAccessKey)
	assert.Equal(t, "C", loadedCreds.SessionToken)
	assert.Equal(t, "us-west-2", loadedCreds.Region)

	require.NoError(t, identity.Logout(context.Background()))
	exists, err = identity.CredentialsExist()
	require.NoError(t, err)
	assert.False(t, exists)

	assert.NoError(t, identity.Logout(context.Background()))
}

func TestIdentity_LoadCredentials_InvalidJSON(t *testing.T) {
	t.Parallel()

	identity := NewIdentity("invalid-json", &schema.Identity{Kind: "aws"})
	credPath := identity.getCredentialsFilePath()
	t.Cleanup(func() {
		_ = os.Remove(credPath)
	})

	require.NoError(t, os.WriteFile(credPath, []byte("{not-json"), MockFilePermissions))
	_, err := identity.LoadCredentials(context.Background())
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "failed to unmarshal mock credentials"))
}

func TestIdentity_SetRealmValidateAndPaths(t *testing.T) {
	t.Parallel()

	identity := NewIdentity("realm-test", &schema.Identity{Kind: "aws"})
	identity.SetRealm("test-realm")
	assert.Equal(t, "test-realm", identity.realm)
	assert.NoError(t, identity.Validate())

	paths, err := identity.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths)
}
