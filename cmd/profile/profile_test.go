package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/internal"
)

// TestProfileCommandProvider_GetCommand tests the GetCommand method.
func TestProfileCommandProvider_GetCommand(t *testing.T) {
	provider := &ProfileCommandProvider{}

	cmd := provider.GetCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "profile", cmd.Use)
	assert.Equal(t, "Manage configuration profiles", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
}

// TestProfileCommandProvider_GetName tests the GetName method.
func TestProfileCommandProvider_GetName(t *testing.T) {
	provider := &ProfileCommandProvider{}

	name := provider.GetName()

	assert.Equal(t, "profile", name)
}

// TestProfileCommandProvider_GetGroup tests the GetGroup method.
func TestProfileCommandProvider_GetGroup(t *testing.T) {
	provider := &ProfileCommandProvider{}

	group := provider.GetGroup()

	assert.Equal(t, "Configuration Management", group)
}

// TestProfileCommandProvider_GetAliases tests the GetAliases method.
func TestProfileCommandProvider_GetAliases(t *testing.T) {
	provider := &ProfileCommandProvider{}

	aliases := provider.GetAliases()

	require.Len(t, aliases, 1)

	// Verify the "list profiles" alias.
	listAlias := aliases[0]
	assert.Equal(t, "list", listAlias.Subcommand)
	assert.Equal(t, "list", listAlias.ParentCommand)
	assert.Equal(t, "profiles", listAlias.Name)
	assert.Equal(t, "List available configuration profiles", listAlias.Short)
	assert.Contains(t, listAlias.Long, "alias for \"atmos profile list\"")
	assert.Contains(t, listAlias.Example, "atmos list profiles")
}

// TestProfileCommandProvider_ImplementsInterface tests that ProfileCommandProvider
// implements the CommandProvider interface.
func TestProfileCommandProvider_ImplementsInterface(t *testing.T) {
	var _ internal.CommandProvider = (*ProfileCommandProvider)(nil)
}
