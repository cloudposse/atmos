package azure

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stubACRAuth stubs the ACR token seam and uses the real Docker config manager.
func stubACRAuth(t *testing.T) {
	t.Helper()
	origGetAuth, origFactory := acrGetAuthToken, acrDockerConfigFactory
	t.Cleanup(func() { acrGetAuthToken, acrDockerConfigFactory = origGetAuth, origFactory })
	acrGetAuthToken = func(context.Context, types.ICredentials, string) (*azureCloud.ACRAuthResult, error) {
		return &azureCloud.ACRAuthResult{
			Username: "00000000-0000-0000-0000-000000000000", Password: "refresh-token",
			Registry: "myregistry.azurecr.io", ExpiresAt: time.Now().Add(3 * time.Hour),
		}, nil
	}
	acrDockerConfigFactory = docker.NewConfigManager
}

// TestACRIntegration_HelperManagesRegistry verifies that a registry routed to
// a credential helper that cannot store (it gets ACR credentials itself) is
// left to that helper: the login succeeds and config.json is unchanged.
func TestACRIntegration_HelperManagesRegistry(t *testing.T) {
	const content = `{"credHelpers":{"myregistry.azurecr.io":"acr-env"}}`
	path := installFakeCredentialHelper(t, "acr-env", "not-implemented", content)
	stubACRAuth(t)

	integration := &ACRIntegration{name: "test-acr", registry: &schema.Registry{Name: "myregistry"}}
	require.NoError(t, integration.Execute(context.Background(), nil))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, content, string(data))
}

// TestACRIntegration_FailingHelperStillFails is the negative path.
func TestACRIntegration_FailingHelperStillFails(t *testing.T) {
	installFakeCredentialHelper(t, "broken", "fail", `{"credsStore":"broken"}`)
	stubACRAuth(t)

	integration := &ACRIntegration{name: "test-acr", registry: &schema.Registry{Name: "myregistry"}}
	err := integration.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
}
