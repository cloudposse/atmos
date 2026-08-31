package azure

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAzureCommandProvider_GetCommand(t *testing.T) {
	provider := &AzureCommandProvider{}
	cmd := provider.GetCommand()

	assert.NotNil(t, cmd)
	assert.Equal(t, "azure", cmd.Use)
	assert.Contains(t, cmd.Short, "Azure")
}

func TestAzureCommandProvider_GetName(t *testing.T) {
	provider := &AzureCommandProvider{}
	assert.Equal(t, "azure", provider.GetName())
}

func TestAzureCommandProvider_GetGroup(t *testing.T) {
	provider := &AzureCommandProvider{}
	assert.Equal(t, "Cloud Integration", provider.GetGroup())
}

func TestAzureCommandProvider_GetAliases(t *testing.T) {
	provider := &AzureCommandProvider{}
	assert.Nil(t, provider.GetAliases())
}

func TestAzureCommandProvider_GetFlagsBuilder(t *testing.T) {
	provider := &AzureCommandProvider{}
	assert.Nil(t, provider.GetFlagsBuilder())
}

func TestAzureCommandProvider_GetPositionalArgsBuilder(t *testing.T) {
	provider := &AzureCommandProvider{}
	assert.Nil(t, provider.GetPositionalArgsBuilder())
}

func TestAzureCommandProvider_GetCompatibilityFlags(t *testing.T) {
	provider := &AzureCommandProvider{}
	assert.Nil(t, provider.GetCompatibilityFlags())
}

func TestAzureCommandProvider_IsExperimental(t *testing.T) {
	provider := &AzureCommandProvider{}
	assert.False(t, provider.IsExperimental())
}

func TestAzureCommand_HasAksAndAcrSubcommands(t *testing.T) {
	provider := &AzureCommandProvider{}
	cmd := provider.GetCommand()

	found := map[string]bool{}
	for _, subCmd := range cmd.Commands() {
		found[subCmd.Use] = true
	}

	assert.True(t, found["aks"], "azure command should have aks subcommand")
	assert.True(t, found["acr"], "azure command should have acr subcommand")
}
