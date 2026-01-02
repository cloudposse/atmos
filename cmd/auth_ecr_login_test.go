package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCreateECRAuthManager_Success(t *testing.T) {
	// Test that createECRAuthManager can be created with valid config.
	authConfig := &schema.AuthConfig{
		Providers:  map[string]schema.Provider{},
		Identities: map[string]schema.Identity{},
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
