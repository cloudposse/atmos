package composition

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompositionCommandProvider(t *testing.T) {
	provider := &CompositionCommandProvider{}

	cmd := provider.GetCommand()
	require.NotNil(t, cmd)
	assert.Equal(t, "composition", cmd.Use)
	assert.Equal(t, "composition", provider.GetName())
	assert.Equal(t, "Core Stack Commands", provider.GetGroup())
	assert.Nil(t, provider.GetAliases())
	assert.Nil(t, provider.GetFlagsBuilder())
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
	assert.False(t, provider.IsExperimental())
}

func TestCompositionCommandStructure(t *testing.T) {
	subcommands := compositionCmd.Commands()
	names := make([]string, len(subcommands))
	for i, c := range subcommands {
		names[i] = c.Name()
	}
	assert.Contains(t, names, "validate")
}

func TestValidateRequiresExactlyOneArg(t *testing.T) {
	// validate <composition> takes exactly one positional argument.
	require.Error(t, validateCmd.Args(validateCmd, []string{}))
	require.NoError(t, validateCmd.Args(validateCmd, []string{"storefront"}))
	require.Error(t, validateCmd.Args(validateCmd, []string{"a", "b"}))
}

func TestBuildConfigAndStacksInfo_ResolvesStack(t *testing.T) {
	// Stack is resolved via viper so the full precedence chain is honored.
	v := viper.GetViper()
	orig := v.GetString("stack")
	t.Cleanup(func() { v.Set("stack", orig) })

	v.Set("stack", "dev")
	info := buildConfigAndStacksInfo(compositionCmd)
	assert.Equal(t, "dev", info.Stack)

	v.Set("stack", "")
	info = buildConfigAndStacksInfo(compositionCmd)
	assert.Empty(t, info.Stack)
}
