package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// mockAuthConfig builds a credential-free auth configuration using the built-in "mock" provider
// and identity kinds (pkg/auth/providers/mock), which simulate authentication deterministically
// without any network access or real cloud credentials. The in-memory keyring avoids touching the
// host system keyring during tests.
func mockAuthConfig(identityDefault bool) schema.AuthConfig {
	return schema.AuthConfig{
		Keyring: schema.KeyringConfig{Type: "memory"},
		Providers: map[string]schema.Provider{
			"mock-provider": {Kind: "mock"},
		},
		Identities: map[string]schema.Identity{
			"mock-identity": {
				Kind:    "mock",
				Default: identityDefault,
				Via:     &schema.IdentityVia{Provider: "mock-provider"},
			},
		},
	}
}

func TestAuthWhoamiTool_Interface(t *testing.T) {
	tool := NewAuthWhoamiTool(&schema.AtmosConfiguration{})

	assert.Equal(t, "atmos_auth_whoami", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	require.Len(t, params, 1)
	assert.Equal(t, "identity", params[0].Name)
	assert.False(t, params[0].Required)
}

func TestAuthWhoamiTool_NewAuthWhoamiTool(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	tool := NewAuthWhoamiTool(config)

	assert.NotNil(t, tool)
	assert.Equal(t, config, tool.atmosConfig)
}

func TestAuthWhoamiTool_Execute_NilConfig(t *testing.T) {
	tool := NewAuthWhoamiTool(nil)

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

func TestAuthWhoamiTool_Execute_NoIdentitiesConfigured(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth:          schema.AuthConfig{Keyring: schema.KeyringConfig{Type: "memory"}},
		CliConfigPath: t.TempDir(),
	}
	tool := NewAuthWhoamiTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestAuthWhoamiTool_Execute_UnknownIdentity(t *testing.T) {
	authConfig := mockAuthConfig(true)
	atmosConfig := &schema.AtmosConfiguration{
		Auth:          authConfig,
		CliConfigPath: t.TempDir(),
	}
	tool := NewAuthWhoamiTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"identity": "does-not-exist",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestAuthWhoamiTool_Execute_DefaultMockIdentity(t *testing.T) {
	authConfig := mockAuthConfig(true)
	atmosConfig := &schema.AtmosConfiguration{
		Auth:          authConfig,
		CliConfigPath: t.TempDir(),
	}
	tool := NewAuthWhoamiTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "mock-identity", result.Data["identity"])
	assert.Contains(t, result.Output, "mock-identity")
	// Credentials must never be present in the output or data.
	assert.NotContains(t, result.Output, "MOCK_SECRET")
	assert.NotContains(t, result.Output, "MOCK_TOKEN")
}

func TestAuthWhoamiTool_Execute_ExplicitIdentity(t *testing.T) {
	authConfig := mockAuthConfig(false)
	atmosConfig := &schema.AtmosConfiguration{
		Auth:          authConfig,
		CliConfigPath: t.TempDir(),
	}
	tool := NewAuthWhoamiTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"identity": "mock-identity",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "mock-identity", result.Data["identity"])
	assert.Equal(t, true, result.Data["valid"])
}

func TestWhoamiCredentialsValid_NilInputs(t *testing.T) {
	assert.False(t, whoamiCredentialsValid(nil))
}
