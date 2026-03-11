package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMCPCommandProvider tests the MCPCommandProvider implementation.
func TestMCPCommandProvider(t *testing.T) {
	provider := &MCPCommandProvider{}

	// Test GetCommand.
	cmd := provider.GetCommand()
	assert.NotNil(t, cmd)
	assert.Equal(t, "mcp", cmd.Use)

	// Test GetName.
	assert.Equal(t, "mcp", provider.GetName())

	// Test GetGroup.
	assert.Equal(t, "AI Commands", provider.GetGroup())

	// Test IsExperimental.
	assert.True(t, provider.IsExperimental(), "MCP command should be marked as experimental")
}

// TestMCPCmd_BasicProperties tests the basic properties of the mcp command.
func TestMCPCmd_BasicProperties(t *testing.T) {
	cmd := mcpCmd

	assert.Equal(t, "mcp", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.Contains(t, cmd.Short, "MCP")
	assert.Contains(t, cmd.Long, "MCP")
}

// TestMCPCommandProvider_GetFlagsBuilder tests that GetFlagsBuilder returns nil.
func TestMCPCommandProvider_GetFlagsBuilder(t *testing.T) {
	provider := &MCPCommandProvider{}

	flagsBuilder := provider.GetFlagsBuilder()
	assert.Nil(t, flagsBuilder, "MCP command should not have a flags builder")
}

// TestMCPCommandProvider_GetPositionalArgsBuilder tests that GetPositionalArgsBuilder returns nil.
func TestMCPCommandProvider_GetPositionalArgsBuilder(t *testing.T) {
	provider := &MCPCommandProvider{}

	argsBuilder := provider.GetPositionalArgsBuilder()
	assert.Nil(t, argsBuilder, "MCP command should not have a positional args builder")
}

// TestMCPCommandProvider_GetCompatibilityFlags tests that GetCompatibilityFlags returns nil.
func TestMCPCommandProvider_GetCompatibilityFlags(t *testing.T) {
	provider := &MCPCommandProvider{}

	compatFlags := provider.GetCompatibilityFlags()
	assert.Nil(t, compatFlags, "MCP command should not have compatibility flags")
}

// TestMCPCommandProvider_GetAliases tests that GetAliases returns nil.
func TestMCPCommandProvider_GetAliases(t *testing.T) {
	provider := &MCPCommandProvider{}

	aliases := provider.GetAliases()
	assert.Nil(t, aliases, "MCP command should not have aliases")
}

// TestMCPCmd_HasSubcommands tests that mcp command has the start subcommand.
func TestMCPCmd_HasSubcommands(t *testing.T) {
	cmd := mcpCmd

	// Check that start subcommand exists.
	startSubCmd, _, err := cmd.Find([]string{"start"})
	assert.NoError(t, err)
	assert.NotNil(t, startSubCmd)
	assert.Equal(t, "start", startSubCmd.Use)
}

// TestMCPCmd_LongDescriptionContent tests the content of the long description.
func TestMCPCmd_LongDescriptionContent(t *testing.T) {
	cmd := mcpCmd

	// Verify it mentions key concepts.
	assert.Contains(t, cmd.Long, "Model Context Protocol")
	assert.Contains(t, cmd.Long, "AI assistants")
	assert.Contains(t, cmd.Long, "Claude")
}

// TestMCPCmd_ShortDescription tests the short description.
func TestMCPCmd_ShortDescription(t *testing.T) {
	cmd := mcpCmd

	// Short description should mention MCP and be concise.
	assert.Contains(t, cmd.Short, "MCP")
	assert.Less(t, len(cmd.Short), 100, "Short description should be concise")
}
