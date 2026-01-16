package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDuCommandProvider_GetCommand(t *testing.T) {
	provider := &DuCommandProvider{}
	cmd := provider.GetCommand()

	assert.NotNil(t, cmd)
	assert.Equal(t, "du", cmd.Use)
	assert.Equal(t, "Show disk space usage for installed tools", cmd.Short)
	assert.True(t, cmd.SilenceUsage)
	assert.True(t, cmd.SilenceErrors)
}

func TestDuCommandProvider_GetName(t *testing.T) {
	provider := &DuCommandProvider{}
	name := provider.GetName()

	assert.Equal(t, "du", name)
}

func TestDuCommandProvider_GetGroup(t *testing.T) {
	provider := &DuCommandProvider{}
	group := provider.GetGroup()

	assert.Equal(t, "Toolchain Commands", group)
}

func TestDuCommandProvider_GetFlagsBuilder(t *testing.T) {
	provider := &DuCommandProvider{}
	builder := provider.GetFlagsBuilder()

	assert.Nil(t, builder)
}

func TestDuCommandProvider_GetPositionalArgsBuilder(t *testing.T) {
	provider := &DuCommandProvider{}
	builder := provider.GetPositionalArgsBuilder()

	assert.Nil(t, builder)
}

func TestDuCommandProvider_GetCompatibilityFlags(t *testing.T) {
	provider := &DuCommandProvider{}
	flags := provider.GetCompatibilityFlags()

	assert.Nil(t, flags)
}

func TestDuCommandProvider_GetAliases(t *testing.T) {
	provider := &DuCommandProvider{}
	aliases := provider.GetAliases()

	assert.Nil(t, aliases)
}

func TestDuCommandProvider_IsExperimental(t *testing.T) {
	provider := &DuCommandProvider{}
	experimental := provider.IsExperimental()

	assert.False(t, experimental)
}
