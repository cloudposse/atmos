package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthListTool_Interface(t *testing.T) {
	tool := NewAuthListTool(&schema.AtmosConfiguration{})

	assert.Equal(t, "atmos_auth_list", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	require.Len(t, params, 2)
	assert.Equal(t, "providers", params[0].Name)
	assert.Equal(t, "identities", params[1].Name)
	assert.False(t, params[0].Required)
	assert.False(t, params[1].Required)
}

func TestAuthListTool_NewAuthListTool(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	tool := NewAuthListTool(config)

	assert.NotNil(t, tool)
	assert.Equal(t, config, tool.atmosConfig)
}

func TestAuthListTool_Execute_NilConfig(t *testing.T) {
	tool := NewAuthListTool(nil)

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestAuthListTool_Execute_MutuallyExclusiveFilters(t *testing.T) {
	authConfig := mockAuthConfig(true)
	atmosConfig := &schema.AtmosConfiguration{Auth: authConfig, CliConfigPath: t.TempDir()}
	tool := NewAuthListTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"providers":  "mock-provider",
		"identities": "mock-identity",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.ErrorIs(t, err, errUtils.ErrMutuallyExclusiveFlags)
}

func TestAuthListTool_Execute_Empty(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth:          schema.AuthConfig{Keyring: schema.KeyringConfig{Type: "memory"}},
		CliConfigPath: t.TempDir(),
	}
	tool := NewAuthListTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Providers (0)")
	assert.Contains(t, result.Output, "Identities (0)")
}

func TestAuthListTool_Execute_ListsProvidersAndIdentities(t *testing.T) {
	authConfig := mockAuthConfig(true)
	atmosConfig := &schema.AtmosConfiguration{Auth: authConfig, CliConfigPath: t.TempDir()}
	tool := NewAuthListTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "mock-provider")
	assert.Contains(t, result.Output, "mock-identity")
	assert.Contains(t, result.Output, "(default)")

	providers, ok := result.Data["providers"].(map[string]authProviderSummary)
	require.True(t, ok)
	require.Contains(t, providers, "mock-provider")
	assert.Equal(t, "mock", providers["mock-provider"].Kind)

	identities, ok := result.Data["identities"].(map[string]authIdentitySummary)
	require.True(t, ok)
	require.Contains(t, identities, "mock-identity")
	assert.Equal(t, "mock", identities["mock-identity"].Kind)
	assert.True(t, identities["mock-identity"].Default)
}

func TestAuthListTool_Execute_FilterProviders(t *testing.T) {
	authConfig := mockAuthConfig(true)
	atmosConfig := &schema.AtmosConfiguration{Auth: authConfig, CliConfigPath: t.TempDir()}
	tool := NewAuthListTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"providers": "mock-provider",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	identities, ok := result.Data["identities"].(map[string]authIdentitySummary)
	require.True(t, ok)
	assert.Empty(t, identities)
}

func TestAuthListTool_Execute_FilterUnknownProvider(t *testing.T) {
	authConfig := mockAuthConfig(true)
	atmosConfig := &schema.AtmosConfiguration{Auth: authConfig, CliConfigPath: t.TempDir()}
	tool := NewAuthListTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"providers": "does-not-exist",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.ErrorIs(t, err, errUtils.ErrProviderNotFound)
}

func TestAuthListTool_Execute_FilterUnknownIdentity(t *testing.T) {
	authConfig := mockAuthConfig(true)
	atmosConfig := &schema.AtmosConfiguration{Auth: authConfig, CliConfigPath: t.TempDir()}
	tool := NewAuthListTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"identities": "does-not-exist",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
}

func TestParseNameList(t *testing.T) {
	assert.Nil(t, parseNameList(""))
	assert.Equal(t, []string{"a", "b"}, parseNameList("a, b"))
	assert.Equal(t, []string{"a"}, parseNameList(" a , , "))
}
