package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAWSMaterializerLifecycle(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	materializer := AWSMaterializer("broker", &schema.Identity{Provider: "legacy", Via: &schema.IdentityVia{Provider: "corporate"}})
	materializer.SetRealm("fixture")
	credentials := &types.AWSCredentials{AccessKeyID: "access-key", SecretAccessKey: "secret-key", SessionToken: "session-token", Expiration: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), Region: "us-west-2"}
	authContext := &schema.AuthContext{}
	stack := &schema.ConfigAndStacksInfo{}
	require.NoError(t, materializer.Apply(t.Context(), &types.PostAuthenticateParams{ProviderName: "corporate", Realm: "fixture", Credentials: credentials, AuthContext: authContext, StackInfo: stack}))
	require.NotNil(t, authContext.AWS)
	assert.Equal(t, "broker", authContext.AWS.Profile)
	assert.Equal(t, "us-west-2", stack.ComponentEnvSection["AWS_REGION"])
	paths, err := materializer.Paths()
	require.NoError(t, err)
	assert.Equal(t, []types.Path{{Location: authContext.AWS.CredentialsFile, Type: types.PathTypeFile, Purpose: "AWS credentials"}, {Location: authContext.AWS.ConfigFile, Type: types.PathTypeFile, Purpose: "AWS configuration"}}, paths)
	loaded, err := materializer.Load(t.Context())
	require.NoError(t, err)
	aws, ok := loaded.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, credentials.AccessKeyID, aws.AccessKeyID)
	assert.Equal(t, credentials.SecretAccessKey, aws.SecretAccessKey)
	assert.Equal(t, credentials.SessionToken, aws.SessionToken)
	input := map[string]string{"AWS_ACCESS_KEY_ID": "ambient", "KEEP": "retained"}
	prepared, err := materializer.PrepareEnvironment(input)
	require.NoError(t, err)
	assert.NotContains(t, prepared, "AWS_ACCESS_KEY_ID")
	assert.Equal(t, "broker", prepared["AWS_PROFILE"])
	assert.Equal(t, "retained", prepared["KEEP"])
	assert.Equal(t, "ambient", input["AWS_ACCESS_KEY_ID"])
	require.NoError(t, materializer.Logout(t.Context()))
	for _, path := range paths {
		_, err := os.Stat(path.Location)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
}

func TestAWSMaterializerRejectsInvalidInputAndStorage(t *testing.T) {
	materializer := AWSMaterializer("broker", &schema.Identity{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, materializer.Apply(ctx, nil), context.Canceled)
	require.ErrorIs(t, materializer.Apply(t.Context(), nil), errUtils.ErrInvalidAuthConfig)
	require.ErrorIs(t, materializer.Apply(t.Context(), &types.PostAuthenticateParams{}), errUtils.ErrInvalidAuthConfig)
	require.NoError(t, materializer.Logout(t.Context()), "unbound provider must not remove another provider's files")
	blocked := filepath.Join(t.TempDir(), "not-directory")
	require.NoError(t, os.WriteFile(blocked, []byte("blocker"), 0o600))
	t.Setenv("XDG_CONFIG_HOME", blocked)
	materializer.SetRootProvider("corporate")
	_, err := materializer.Environment()
	require.Error(t, err)
	_, err = materializer.Paths()
	require.Error(t, err)
	_, err = materializer.Load(t.Context())
	require.Error(t, err)
	_, err = materializer.PrepareEnvironment(nil)
	require.Error(t, err)
	require.Error(t, materializer.Logout(t.Context()))
	require.Error(t, materializer.Apply(t.Context(), &types.PostAuthenticateParams{Credentials: &types.AWSCredentials{}, ProviderName: "corporate"}))
}
