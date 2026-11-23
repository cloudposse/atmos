package acr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

// TestExecuteExplicitRegistries_UnusableCredentialHelperFailsLoudly verifies that an
// explicit `atmos azure acr login --registry` fails, instead of reporting success, when
// config.json routes the registry to a Docker credential helper that cannot store the
// token. Docker would ignore an inline auth entry for such a host, so a silent success
// would leave the user without working credentials. The user's config.json must be
// left exactly as it was.
func TestExecuteExplicitRegistries_UnusableCredentialHelperFailsLoudly(t *testing.T) {
	dockerDir := t.TempDir()
	original := []byte(`{"credsStore":"atmos-test-missing-helper","currentContext":"desktop-linux"}`)
	configPath := filepath.Join(dockerDir, "config.json")
	require.NoError(t, os.WriteFile(configPath, original, 0o600))
	t.Setenv("DOCKER_CONFIG", dockerDir)
	// An empty PATH guarantees docker-credential-atmos-test-missing-helper cannot be found.
	t.Setenv("PATH", t.TempDir())

	stubAzureSeams(
		t, okACRCreds,
		func(_ context.Context, _ authTypes.ICredentials, loginServer string) (*azureCloud.ACRAuthResult, error) {
			return &azureCloud.ACRAuthResult{Username: "u", Password: "p", Registry: loginServer}, nil
		},
	)

	err := executeExplicitRegistries(context.Background(), []string{"myregistry.azurecr.io"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrDockerConfigWrite), "expected ErrDockerConfigWrite, got %v", err)
	assert.True(t, errors.Is(err, errUtils.ErrDockerCredentialHelper), "expected ErrDockerCredentialHelper, got %v", err)

	after, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	assert.JSONEq(t, string(original), string(after), "a failed login must not modify config.json")
}
