package ecr

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCreateAuthManager_Success(t *testing.T) {
	// Test that createAuthManager can be created with valid config.
	authConfig := &schema.AuthConfig{
		Realm:        "test-realm",
		Providers:    map[string]schema.Provider{},
		Identities:   map[string]schema.Identity{},
		Integrations: map[string]schema.Integration{},
	}

	manager, err := createAuthManager(authConfig, "")
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestCreateAuthManager_NilConfig(t *testing.T) {
	// Test that createAuthManager fails with nil config.
	_, err := createAuthManager(nil, "")
	require.Error(t, err)
}

func TestLoginCmd_Help(t *testing.T) {
	// Verify command metadata.
	assert.Equal(t, "login [integration]", loginCmd.Use)
	assert.Equal(t, "Login to AWS ECR registries", loginCmd.Short)
	assert.Contains(t, loginCmd.Long, "Login to AWS ECR registries")
}

func TestLoginCmd_HasRegistryFlag(t *testing.T) {
	// Verify --registry flag exists.
	registryFlag := loginCmd.Flags().Lookup("registry")
	require.NotNil(t, registryFlag)
	assert.Equal(t, "r", registryFlag.Shorthand)
}

func TestLoginCmd_HasIdentityFlag(t *testing.T) {
	// Verify --identity flag exists.
	identityFlag := loginCmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag)
	assert.Equal(t, "i", identityFlag.Shorthand)
}

func TestLoginCmd_TooManyArgs(t *testing.T) {
	// Test that command returns error when too many arguments provided.
	err := loginCmd.Args(loginCmd, []string{"arg1", "arg2"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at most 1 arg")
}

func TestLoginCmd_MaxOneArg(t *testing.T) {
	// Test that command accepts at most 1 argument.
	assert.Nil(t, loginCmd.Args(loginCmd, []string{}))
	assert.Nil(t, loginCmd.Args(loginCmd, []string{"arg1"}))
	assert.NotNil(t, loginCmd.Args(loginCmd, []string{"arg1", "arg2"}))
}

func TestLoginCmd_NoArgsError(t *testing.T) {
	// Verify the error sentinel exists. Behavioral testing requires a full
	// config + auth manager setup which is covered by integration tests.
	assert.NotNil(t, errUtils.ErrECRLoginNoArgs)
}

func TestCreateAuthManager_EmptyConfig(t *testing.T) {
	// Test with nil maps (not empty maps) to verify zero-value behavior.
	authConfig := &schema.AuthConfig{
		Realm: "test-realm",
	}

	manager, err := createAuthManager(authConfig, "")
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestCreateAuthManager_WithProviders(t *testing.T) {
	// Test with config containing providers.
	authConfig := &schema.AuthConfig{
		Realm: "test-realm",
		Providers: map[string]schema.Provider{
			"test-provider": {
				Kind: "mock",
			},
		},
		Identities:   map[string]schema.Identity{},
		Integrations: map[string]schema.Integration{},
	}

	manager, err := createAuthManager(authConfig, "")
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestCreateAuthManager_WithIntegrations(t *testing.T) {
	// Test with config containing integrations.
	authConfig := &schema.AuthConfig{
		Realm:     "test-realm",
		Providers: map[string]schema.Provider{},
		Identities: map[string]schema.Identity{
			"test-identity": {
				Kind: "aws/user",
			},
		},
		Integrations: map[string]schema.Integration{
			"ecr/test": {
				Kind: "aws/ecr",
				Via: &schema.IntegrationVia{
					Identity: "test-identity",
				},
			},
		},
	}

	manager, err := createAuthManager(authConfig, "")
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestLoginCmd_FlagDefaults(t *testing.T) {
	// Verify --registry flag has empty array default.
	registryFlag := loginCmd.Flags().Lookup("registry")
	require.NotNil(t, registryFlag)
	assert.Equal(t, "[]", registryFlag.DefValue) // Empty array stringified.
	assert.Equal(t, "stringArray", registryFlag.Value.Type())
}

func TestLoginCmd_CommandStructure(t *testing.T) {
	// Verify command is properly configured.
	assert.NotNil(t, loginCmd.Args) // Args validator is set.
	assert.NotNil(t, loginCmd.RunE)
	assert.False(t, loginCmd.FParseErrWhitelist.UnknownFlags)
}

func TestLoginCmd_ParentIsEcrCmd(t *testing.T) {
	// Verify that login is a child of ecr command.
	require.NotNil(t, loginCmd.Parent())
	assert.Equal(t, "ecr", loginCmd.Parent().Name())
}

func TestLoginCmd_LongDescription(t *testing.T) {
	// Verify long description contains expected content.
	assert.Contains(t, loginCmd.Long, "named integration")
	assert.Contains(t, loginCmd.Long, "--identity")
	assert.Contains(t, loginCmd.Long, "--registry")
	assert.Contains(t, loginCmd.Long, "Examples:")
}

func TestLoginCmd_ExamplesInLong(t *testing.T) {
	// Verify examples are included in long description.
	assert.Contains(t, loginCmd.Long, "atmos aws ecr login dev/ecr")
	assert.Contains(t, loginCmd.Long, "atmos aws ecr login --identity dev-admin")
	assert.Contains(t, loginCmd.Long, "atmos aws ecr login --registry")
}

func TestLoginCmd_HelpArgIsNotTreatedAsIntegration(t *testing.T) {
	// Verify "help" positional arg shows help instead of being treated as integration name.
	// The command should return nil (help was displayed) rather than an integration error.
	err := executeLoginCommand(loginCmd, []string{"help"})
	assert.NoError(t, err)
}

func TestExecuteWithAuthManager_NoArgs(t *testing.T) {
	// No identity and no integration — should return ErrECRLoginNoArgs.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Realm: "test",
		},
	}
	err := executeWithAuthManager(context.Background(), atmosConfig, "", "")
	assert.ErrorIs(t, err, errUtils.ErrECRLoginNoArgs)
}

func TestExecuteWithAuthManager_SelectSentinel(t *testing.T) {
	// __SELECT__ sentinel should return ErrECRIdentitySelect.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Realm: "test",
		},
	}
	err := executeWithAuthManager(context.Background(), atmosConfig, cfg.IdentityFlagSelectValue, "")
	assert.ErrorIs(t, err, errUtils.ErrECRIdentitySelect)
}

func TestExecuteWithAuthManager_MutuallyExclusiveFlags(t *testing.T) {
	// Both integration name and --identity should be rejected.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Realm: "test",
		},
	}
	err := executeWithAuthManager(context.Background(), atmosConfig, "some-identity", "some-integration")
	assert.ErrorIs(t, err, errUtils.ErrMutuallyExclusiveFlags)
}
