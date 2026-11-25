package ecr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

const helperTestRegistry = "123456789012.dkr.ecr.us-east-2.amazonaws.com"

// writeDockerConfig writes config.json under a fresh DOCKER_CONFIG and returns its path.
func writeDockerConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// stubECRTokens stubs the AWS seams so logins mint tokens without AWS.
func stubECRTokens(t *testing.T) {
	t.Helper()
	stubAWSSeams(
		t,
		func(context.Context) (*authTypes.AWSCredentials, error) {
			return &authTypes.AWSCredentials{Region: "us-east-2"}, nil
		},
		func(context.Context, authTypes.ICredentials, ...awsCloud.ECRPublicAuthOption) (*awsCloud.ECRPublicAuthResult, error) {
			return validPublicAuthResult(), nil
		},
		func(context.Context, authTypes.ICredentials, string, string) (*awsCloud.ECRAuthResult, error) {
			return &awsCloud.ECRAuthResult{Username: "AWS", Password: "token", ExpiresAt: time.Now().Add(12 * time.Hour)}, nil
		},
	)
}

// TestExecuteExplicitRegistries_EcrLoginHelperManagesRegistry reproduces
// `atmos aws ecr login <registry>` with config.json routing the registry to
// docker-credential-ecr-login, which fetches ECR credentials itself and cannot
// store them. The login succeeds and config.json is left as it was.
func TestExecuteExplicitRegistries_EcrLoginHelperManagesRegistry(t *testing.T) {
	const content = `{"credHelpers":{"` + helperTestRegistry + `":"ecr-login","public.ecr.aws":"ecr-login"}}`
	path := writeDockerConfig(t, content)
	installFakeECRCredentialHelper(t, "ecr-login", fakeECRHelperLogin)
	stubECRTokens(t)

	require.NoError(t, executeExplicitRegistries(context.Background(), []string{helperTestRegistry}))
	require.NoError(t, publicLogin(context.Background(), &authTypes.AWSCredentials{Region: "us-east-1"}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, content, string(data), "config.json keeps credHelpers and gains no inline auth")
}

// TestExecuteExplicitRegistries_FailingHelperStillFails is the negative path:
// a helper whose store genuinely fails still fails the login.
func TestExecuteExplicitRegistries_FailingHelperStillFails(t *testing.T) {
	const content = `{"credHelpers":{"` + helperTestRegistry + `":"broken","public.ecr.aws":"broken"}}`
	path := writeDockerConfig(t, content)
	installFakeECRCredentialHelper(t, "broken", fakeECRHelperFail)
	stubECRTokens(t)

	err := executeExplicitRegistries(context.Background(), []string{helperTestRegistry})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	assert.Contains(t, err.Error(), "keychain is locked")

	err = publicLogin(context.Background(), &authTypes.AWSCredentials{Region: "us-east-1"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.JSONEq(t, content, string(data))
}
