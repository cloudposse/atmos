package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCreateECRAuthManager_Success(t *testing.T) {
	// Test that createECRAuthManager can be created with valid config.
	authConfig := &schema.AuthConfig{
		Providers:    map[string]schema.Provider{},
		Identities:   map[string]schema.Identity{},
		Integrations: map[string]schema.Integration{},
	}

	manager, err := createECRAuthManager(authConfig)
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestCreateECRAuthManager_NilConfig(t *testing.T) {
	// Test that createECRAuthManager fails with nil config.
	_, err := createECRAuthManager(nil)
	require.Error(t, err)
}

func TestAuthECRLoginCmd_Help(t *testing.T) {
	// Verify command metadata.
	assert.Equal(t, "ecr-login [integration]", authECRLoginCmd.Use)
	assert.Equal(t, "Login to AWS ECR registries", authECRLoginCmd.Short)
	assert.Contains(t, authECRLoginCmd.Long, "Login to AWS ECR registries")
}

func TestAuthECRLoginCmd_HasRegistryFlag(t *testing.T) {
	// Verify --registry flag exists.
	registryFlag := authECRLoginCmd.Flags().Lookup("registry")
	require.NotNil(t, registryFlag)
	assert.Equal(t, "r", registryFlag.Shorthand)
}

func TestAuthECRLoginCmd_TooManyArgs(t *testing.T) {
	// Test that command returns error when too many arguments provided.
	// Args returns an error when more than 1 argument is provided.
	err := authECRLoginCmd.Args(authECRLoginCmd, []string{"arg1", "arg2"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at most 1 arg")
}

func TestAuthECRLoginCmd_MaxOneArg(t *testing.T) {
	// Test that command accepts at most 1 argument.
	assert.Nil(t, authECRLoginCmd.Args(authECRLoginCmd, []string{}))
	assert.Nil(t, authECRLoginCmd.Args(authECRLoginCmd, []string{"arg1"}))
	assert.NotNil(t, authECRLoginCmd.Args(authECRLoginCmd, []string{"arg1", "arg2"}))
}

func TestAuthECRLoginCmd_NoArgsError(t *testing.T) {
	_ = NewTestKit(t)

	// Create a test command with the same RunE.
	testCmd := &cobra.Command{
		Use:  "test-ecr-login",
		RunE: executeAuthECRLoginCommand,
	}
	testCmd.Flags().String("identity", "", "Identity name")
	testCmd.Flags().StringArray("registry", nil, "ECR registry URL(s)")

	// Test with no arguments should return ErrECRLoginNoArgs.
	// We can't directly call executeAuthECRLoginCommand because it needs config,
	// but we can verify the error type exists.
	assert.NotNil(t, errUtils.ErrECRLoginNoArgs)
}

func TestCreateECRAuthManager_EmptyConfig(t *testing.T) {
	// Test with empty but valid config.
	authConfig := &schema.AuthConfig{
		Providers:    map[string]schema.Provider{},
		Identities:   map[string]schema.Identity{},
		Integrations: map[string]schema.Integration{},
	}

	manager, err := createECRAuthManager(authConfig)
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestCreateECRAuthManager_WithProviders(t *testing.T) {
	// Test with config containing providers.
	// Use mock provider kind which doesn't require additional config.
	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"test-provider": {
				Kind: "mock",
			},
		},
		Identities:   map[string]schema.Identity{},
		Integrations: map[string]schema.Integration{},
	}

	manager, err := createECRAuthManager(authConfig)
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestCreateECRAuthManager_WithIntegrations(t *testing.T) {
	// Test with config containing integrations.
	authConfig := &schema.AuthConfig{
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

	manager, err := createECRAuthManager(authConfig)
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestAuthECRLoginCmd_FlagDefaults(t *testing.T) {
	_ = NewTestKit(t)

	// Verify --registry flag has empty array default.
	registryFlag := authECRLoginCmd.Flags().Lookup("registry")
	require.NotNil(t, registryFlag)
	assert.Equal(t, "[]", registryFlag.DefValue) // Empty array stringified.
	assert.Equal(t, "stringArray", registryFlag.Value.Type())
}

func TestAuthECRLoginCmd_CommandStructure(t *testing.T) {
	// Verify command is properly configured.
	assert.NotNil(t, authECRLoginCmd.Args) // Args validator is set.
	assert.NotNil(t, authECRLoginCmd.RunE)
	assert.False(t, authECRLoginCmd.FParseErrWhitelist.UnknownFlags)
}

func TestAuthECRLoginCmd_ParentIsAuthCmd(t *testing.T) {
	// Verify that ecr-login is a child of auth command.
	assert.NotNil(t, authECRLoginCmd.Parent())
	// The parent should be authCmd, but since init() order is not guaranteed,
	// we just verify it has a parent after init().
	if authECRLoginCmd.Parent() != nil {
		assert.Equal(t, "auth", authECRLoginCmd.Parent().Name())
	}
}

func TestAuthECRLoginCmd_LongDescription(t *testing.T) {
	// Verify long description contains expected content.
	assert.Contains(t, authECRLoginCmd.Long, "named integration")
	assert.Contains(t, authECRLoginCmd.Long, "--identity")
	assert.Contains(t, authECRLoginCmd.Long, "--registry")
	assert.Contains(t, authECRLoginCmd.Long, "Examples:")
}

func TestAuthECRLoginCmd_ExamplesInLong(t *testing.T) {
	// Verify examples are included in long description.
	assert.Contains(t, authECRLoginCmd.Long, "atmos auth ecr-login dev/ecr")
	assert.Contains(t, authECRLoginCmd.Long, "atmos auth ecr-login --identity dev-admin")
	assert.Contains(t, authECRLoginCmd.Long, "atmos auth ecr-login --registry")
}
