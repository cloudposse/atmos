package auth

import (
	"os"
	"path/filepath"
	"testing"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// changeWorkingDir mirrors the helper used in pkg/config/config_test.go
func changeWorkingDir(t *testing.T, dir string) {
	cwd, err := os.Getwd()
	require.NoError(t, err, "Failed to get current directory")

	t.Cleanup(func() {
		err := os.Chdir(cwd)
		require.NoError(t, err, "Failed to restore working directory")
	})

	err = os.Chdir(dir)
	require.NoError(t, err, "Failed to change working directory")
}

// TestScenario_Auth_OIDC_GetIdentity loads the atmos-auth scenario fixtures and verifies
// the auth package can resolve the configured OIDC identity and its provider defaults
// without performing any network operations.
func TestScenario_Auth_OIDC_GetIdentity(t *testing.T) {
	// Use committed scenario fixtures
	scenarioDir := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "atmos-auth")
	changeWorkingDir(t, scenarioDir)

	// Initialize Atmos config from the scenario
	atmosCfg, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Sanity-check the scenario loaded as expected
	assert.Equal(t, "./", atmosCfg.BasePath)
	// Providers and identities should be present per tests/fixtures/scenarios/atmos-auth/atmos.yaml
	require.NotNil(t, atmosCfg.Auth.Providers)
	require.NotNil(t, atmosCfg.Auth.Identities)

	// The identity key is "oidc" and its provider is "oidcprov" of type aws/oidc
	idpName, err := GetIdp("oidc", atmosCfg.Auth)
	require.NoError(t, err)
	assert.Equal(t, "oidcprov", idpName)

	typeVal, err := GetType(idpName, atmosCfg.Auth)
	require.NoError(t, err)
	assert.Equal(t, "aws/oidc", typeVal)

	// Ensure we can construct the concrete identity instance and it has defaults from provider config
	lm, err := GetIdentityInstance("oidc", atmosCfg.Auth, nil)
	require.NoError(t, err)
	awsOidcInst, ok := lm.(*awsOidc)
	if !ok {
		t.Fatalf("expected *awsOidc, got %T", lm)
	}

	// Validate required fields populated from scenario
	require.NoError(t, awsOidcInst.Validate())
	assert.Equal(t, "arn:aws:iam::000000000000:role/Dummy", awsOidcInst.RoleArn)
	assert.Equal(t, "us-east-1", awsOidcInst.Common.Region)
	assert.Equal(t, "oidc-prof", awsOidcInst.Common.Profile)
}
