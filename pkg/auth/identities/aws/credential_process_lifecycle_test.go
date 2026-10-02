package aws

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Lifecycle tests for the aws/credential-process identity: stored files, environment, logout.
// Shared helpers live in credential_process_test.go.

func TestCredentialProcessIdentity_LoadCredentials_IsSideEffectFree(t *testing.T) {
	isolateXDG(t)
	fake := &fakeRetriever{creds: &types.AWSCredentials{AccessKeyID: "A", SecretAccessKey: "S"}}
	id := newCPIdentity(t, nil, fake)

	got, err := id.LoadCredentials(context.Background())
	require.Error(t, err, "no files means no credentials")
	assert.Nil(t, got)
	assert.Equal(t, 0, fake.calls, "LoadCredentials must never run the helper")

	exists, err := id.CredentialsExist()
	require.NoError(t, err)
	assert.False(t, exists, "LoadCredentials must not create files")
}

func TestCredentialProcessIdentity_CredentialsExist(t *testing.T) {
	isolateXDG(t)
	id := newCPIdentity(t, nil, nil)

	exists, err := id.CredentialsExist()
	require.NoError(t, err)
	assert.False(t, exists)

	require.NoError(t, id.writeAWSFiles(&types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "S", Region: "us-east-1"}))
	exists, err = id.CredentialsExist()
	require.NoError(t, err)
	assert.True(t, exists)

	other := newCPIdentity(t, nil, nil)
	other.name = "someone-else"
	exists, err = other.CredentialsExist()
	require.NoError(t, err)
	assert.False(t, exists, "a different identity has no section in the shared file")
}

func TestCredentialProcessIdentity_Logout(t *testing.T) {
	isolateXDG(t)
	id := newCPIdentity(t, nil, nil)
	sibling := newCPIdentity(t, nil, nil)
	sibling.name = "sibling"

	creds := &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "S", SessionToken: "T", Region: "us-east-1", Expiration: futureExpiration()}
	require.NoError(t, id.writeAWSFiles(creds))
	require.NoError(t, sibling.writeAWSFiles(creds))

	require.NoError(t, id.Logout(context.Background()))

	exists, err := id.CredentialsExist()
	require.NoError(t, err)
	assert.False(t, exists, "logout removes this identity's credentials")

	exists, err = sibling.CredentialsExist()
	require.NoError(t, err)
	assert.True(t, exists, "logout must not remove other identities' credentials")

	// Logging out again is harmless.
	require.NoError(t, id.Logout(context.Background()))
}

func TestCredentialProcessIdentity_SetRealm(t *testing.T) {
	dir := isolateXDG(t)
	id := newCPIdentity(t, nil, nil)
	id.SetRealm("realm-abc")
	assert.Equal(t, "realm-abc", id.realm)

	require.NoError(t, id.writeAWSFiles(&types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "S", Region: "us-east-1"}))

	env, err := id.Environment()
	require.NoError(t, err)
	assert.Contains(t, env["AWS_SHARED_CREDENTIALS_FILE"], "realm-abc")
	assert.FileExists(t, env["AWS_SHARED_CREDENTIALS_FILE"])
	assert.True(t, strings.HasPrefix(env["AWS_SHARED_CREDENTIALS_FILE"], dir))
}

func TestCredentialProcessIdentity_Environment(t *testing.T) {
	isolateXDG(t)
	id, err := NewCredentialProcessIdentity("corp-base", &schema.Identity{
		Kind:        types.IdentityKindAWSCredentialProcess,
		Credentials: map[string]any{"credential_process": cpTestCommand, "region": "eu-central-1"},
		Env:         []schema.EnvironmentVariable{{Key: "CUSTOM_VAR", Value: "custom"}},
	})
	require.NoError(t, err)

	env, err := id.Environment()
	require.NoError(t, err)
	assert.Equal(t, "corp-base", env["AWS_PROFILE"])
	assert.Equal(t, "eu-central-1", env["AWS_REGION"])
	assert.Equal(t, "custom", env["CUSTOM_VAR"])

	paths, err := id.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestCredentialProcessIdentity_PrepareEnvironment(t *testing.T) {
	dir := isolateXDG(t)
	id := newCPIdentity(t, map[string]any{"credential_process": cpTestCommand, "region": "eu-west-2"}, nil)

	input := map[string]string{
		"PATH":              "/usr/bin",
		"AWS_ACCESS_KEY_ID": "AMBIENT",
	}
	out, err := id.PrepareEnvironment(context.Background(), input)
	require.NoError(t, err)

	wantDir := filepath.Join(dir, "atmos", "aws", "aws-credential-process")
	assert.Equal(t, filepath.Join(wantDir, "credentials"), out["AWS_SHARED_CREDENTIALS_FILE"])
	assert.Equal(t, filepath.Join(wantDir, "config"), out["AWS_CONFIG_FILE"])
	assert.Equal(t, "corp-base", out["AWS_PROFILE"])
	assert.Equal(t, "eu-west-2", out["AWS_REGION"])
	assert.Equal(t, "/usr/bin", out["PATH"])
	assert.NotContains(t, out, "AWS_ACCESS_KEY_ID", "ambient credential variables are cleared")
	assert.Equal(t, "AMBIENT", input["AWS_ACCESS_KEY_ID"], "the input map must not be mutated")
}

func TestCredentialProcessIdentity_PostAuthenticate(t *testing.T) {
	t.Run("nil params", func(t *testing.T) {
		err := newCPIdentity(t, nil, nil).PostAuthenticate(context.Background(), nil)
		assert.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	})

	t.Run("nil credentials", func(t *testing.T) {
		err := newCPIdentity(t, nil, nil).PostAuthenticate(context.Background(), &types.PostAuthenticateParams{})
		assert.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	})

	t.Run("writes files and populates the auth context under the fixed provider", func(t *testing.T) {
		dir := isolateXDG(t)
		id := newCPIdentity(t, nil, nil)
		authContext := &schema.AuthContext{}
		stack := &schema.ConfigAndStacksInfo{}

		// The provider name from the caller is ignored to avoid path drift.
		err := id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
			AuthContext:  authContext,
			StackInfo:    stack,
			ProviderName: "something-else",
			IdentityName: "corp-base",
			Credentials:  &types.AWSCredentials{AccessKeyID: "AK", SecretAccessKey: "SE", Region: "us-east-2"},
		})
		require.NoError(t, err)

		wantCreds := filepath.Join(dir, "atmos", "aws", "aws-credential-process", "credentials")
		assert.FileExists(t, wantCreds)
		require.NotNil(t, authContext.AWS)
		assert.Equal(t, wantCreds, authContext.AWS.CredentialsFile)
		assert.Equal(t, "corp-base", authContext.AWS.Profile)
		assert.Equal(t, "us-east-2", authContext.AWS.Region)
		assert.Equal(t, wantCreds, stack.ComponentEnvSection["AWS_SHARED_CREDENTIALS_FILE"])
	})

	t.Run("propagates spec.endpoint_url to the auth context", func(t *testing.T) {
		isolateXDG(t)
		ctrl := gomock.NewController(t)
		manager := types.NewMockAuthManager(ctrl)
		manager.EXPECT().GetIdentities().Return(map[string]schema.Identity{
			"corp-base": {
				Kind: types.IdentityKindAWSCredentialProcess,
				Spec: map[string]any{"endpoint_url": "http://localhost:4566"},
			},
		}).AnyTimes()

		id := newCPIdentity(t, nil, nil)
		authContext := &schema.AuthContext{}
		err := id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
			AuthContext: authContext,
			StackInfo:   &schema.ConfigAndStacksInfo{},
			Credentials: &types.AWSCredentials{AccessKeyID: "AK", SecretAccessKey: "SE", Region: "us-east-1"},
			Manager:     manager,
		})
		require.NoError(t, err)
		require.NotNil(t, authContext.AWS)
		assert.Equal(t, "http://localhost:4566", authContext.AWS.EndpointURL)
	})
}

// Ensure the shared file manager path used by the identity is the documented one.
func TestCredentialProcessIdentity_ProviderDirectoryConstant(t *testing.T) {
	assert.Equal(t, "aws-credential-process", awsCredentialProcessProviderName)

	dir := isolateXDG(t)
	mgr, err := awsCloud.NewAWSFileManager("", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "atmos", "aws", "aws-credential-process", "credentials"),
		mgr.GetCredentialsPath(awsCredentialProcessProviderName))
}
