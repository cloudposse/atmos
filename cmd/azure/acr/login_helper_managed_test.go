package acr

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

const helperManagedRegistry = "myregistry.azurecr.io"

// writeACRDockerConfig writes config.json under a fresh DOCKER_CONFIG and returns its path.
func writeACRDockerConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// stubACRToken stubs the Azure seams so logins mint tokens without Azure.
func stubACRToken(t *testing.T) {
	t.Helper()
	stubAzureSeams(
		t, okACRCreds,
		func(_ context.Context, _ authTypes.ICredentials, loginServer string) (*azureCloud.ACRAuthResult, error) {
			return &azureCloud.ACRAuthResult{Username: "u", Password: "p", Registry: loginServer}, nil
		},
	)
}

// TestExecuteExplicitRegistries_HelperManagedRegistrySucceeds verifies that an explicit
// `atmos azure acr login --registry` succeeds, and leaves config.json as it was, when
// config.json routes the registry to a credential helper that gets registry credentials
// itself and does not implement store.
func TestExecuteExplicitRegistries_HelperManagedRegistrySucceeds(t *testing.T) {
	const content = `{"credHelpers":{"` + helperManagedRegistry + `":"acr-env"}}`
	path := writeACRDockerConfig(t, content)
	installFakeACRCredentialHelper(t, "acr-env", fakeACRHelperUnsupported)
	stubACRToken(t)

	require.NoError(t, executeExplicitRegistries(context.Background(), []string{helperManagedRegistry}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, content, string(data), "config.json keeps credHelpers and gains no inline auth")
}

// TestExecuteExplicitRegistries_HelperFailureStillFails is the negative path: a helper
// whose store genuinely fails still fails the explicit ACR login.
func TestExecuteExplicitRegistries_HelperFailureStillFails(t *testing.T) {
	const content = `{"credHelpers":{"` + helperManagedRegistry + `":"broken"}}`
	path := writeACRDockerConfig(t, content)
	installFakeACRCredentialHelper(t, "broken", fakeACRHelperFail)
	stubACRToken(t)

	err := executeExplicitRegistries(context.Background(), []string{helperManagedRegistry})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	assert.Contains(t, err.Error(), "keychain is locked")

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.JSONEq(t, content, string(data))
}
