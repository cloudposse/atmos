package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerCommandProvider(t *testing.T) {
	provider := &ContainerCommandProvider{}

	cmd := provider.GetCommand()
	require.NotNil(t, cmd)
	assert.Equal(t, "container", cmd.Use)
	assert.Equal(t, []string{"c"}, cmd.Aliases)
	assert.Equal(t, "container", provider.GetName())
	assert.Equal(t, "Core Stack Commands", provider.GetGroup())
	assert.Nil(t, provider.GetAliases())
	assert.Nil(t, provider.GetFlagsBuilder())
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
	assert.False(t, provider.IsExperimental())
}

func TestContainerCommandStructure(t *testing.T) {
	subcommands := containerCmd.Commands()
	names := make([]string, len(subcommands))
	for i, c := range subcommands {
		names[i] = c.Name()
	}
	for _, want := range []string{
		"build", "push", "pull", "run", "up", "ps",
		"logs", "exec", "attach", "restart", "stop", "rm", "down",
	} {
		assert.Contains(t, names, want)
	}
}

func TestContainerSubcommandsRequireComponent(t *testing.T) {
	// Every verb except `list` requires exactly the component positional arg;
	// calling the validator with no args must error.
	for _, c := range containerCmd.Commands() {
		if c.Args == nil || c.Name() == "list" {
			continue
		}
		require.Error(t, c.Args(c, []string{}), "subcommand %q should require a component", c.Name())
		require.NoError(t, c.Args(c, []string{"api"}), "subcommand %q should accept one component", c.Name())
	}
}

func TestStackFlagIsPersistent(t *testing.T) {
	// The shared --stack/-s and --dry-run flags are inherited by subcommands.
	require.NotNil(t, containerCmd.PersistentFlags().Lookup("stack"))
	require.NotNil(t, containerCmd.PersistentFlags().Lookup("dry-run"))
}
