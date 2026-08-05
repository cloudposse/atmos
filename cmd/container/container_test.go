package container

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
		"build", "push", "pull", "run", "up", "start", "ps",
		"logs", "exec", "attach", "restart", "stop", "rm", "down",
	} {
		assert.Contains(t, names, want)
	}
}

// requireComponentVerbs need exactly one component positional arg. The
// bulk-capable lifecycle verbs (build/push/pull/up/start/restart/stop/rm/down),
// `logs`, and `ps` make it optional (no component => --all/interactive/all), and
// `list` takes none.
var requireComponentVerbs = map[string]bool{
	"run": true, "exec": true, "attach": true,
}

// noAllFlagVerbs must NOT register an `--all` flag.
var noAllFlagVerbs = map[string]bool{
	"run": true, "exec": true, "attach": true, "ps": true,
}

func TestContainerSingleComponentVerbsDeferMissingComponentToPrompt(t *testing.T) {
	for _, c := range containerCmd.Commands() {
		if c.Args == nil || !requireComponentVerbs[c.Name()] {
			continue
		}
		require.NoError(t, c.Args(c, []string{}), "subcommand %q should defer a missing component to the prompt flow", c.Name())
		require.NoError(t, c.Args(c, []string{"api"}), "subcommand %q should accept one component", c.Name())
		require.Error(t, c.Args(c, []string{"api", "extra"}), "subcommand %q should reject extra positional arguments", c.Name())
	}
}

func TestPsAllowsNoComponent(t *testing.T) {
	var ps *cobra.Command
	for _, c := range containerCmd.Commands() {
		if c.Name() == "ps" {
			ps = c
		}
	}
	require.NotNil(t, ps)
	require.NoError(t, ps.Args(ps, []string{}), "ps should allow zero components (lists all)")
	require.NoError(t, ps.Args(ps, []string{"api"}), "ps should accept one component")
	assert.Nil(t, ps.Flags().Lookup("all"), "ps must not register --all (no component already means all)")
}

func TestContainerBulkVerbsAllowNoComponent(t *testing.T) {
	bulkVerbs := []string{"build", "push", "pull", "up", "start", "restart", "stop", "rm", "down"}
	cmds := map[string]*cobra.Command{}
	for _, c := range containerCmd.Commands() {
		cmds[c.Name()] = c
	}
	for _, name := range bulkVerbs {
		c := cmds[name]
		require.NotNil(t, c, "bulk verb %q should be registered", name)
		require.NoError(t, c.Args(c, []string{}), "bulk verb %q should allow zero components", name)
		require.NoError(t, c.Args(c, []string{"api"}), "bulk verb %q should accept one component", name)
		require.NotNil(t, c.Flags().Lookup("all"), "bulk verb %q should register --all", name)
	}
}

func TestLogsCommandFlags(t *testing.T) {
	var logs *cobra.Command
	for _, c := range containerCmd.Commands() {
		if c.Name() == "logs" {
			logs = c
		}
	}
	require.NotNil(t, logs)
	// logs allows zero or one component and supports --all/--follow/--tail.
	require.NoError(t, logs.Args(logs, []string{}), "logs should allow zero components")
	require.NoError(t, logs.Args(logs, []string{"api"}), "logs should accept one component")
	assert.NotNil(t, logs.Flags().Lookup("all"), "logs should register --all")
	assert.NotNil(t, logs.Flags().Lookup("follow"), "logs should register --follow")
	assert.NotNil(t, logs.Flags().ShorthandLookup("f"), "logs --follow should have -f shorthand")
	assert.NotNil(t, logs.Flags().Lookup("tail"), "logs should register --tail")
}

func TestSingleComponentVerbsRejectAllFlag(t *testing.T) {
	for _, c := range containerCmd.Commands() {
		if !noAllFlagVerbs[c.Name()] {
			continue
		}
		assert.Nil(t, c.Flags().Lookup("all"), "verb %q must not register --all", c.Name())
	}
}

func TestStackFlagBindsToViper(t *testing.T) {
	// Regression: the --stack flag value must reach Viper via the per-execution
	// BindFlagsToViper rebind in runVerb. Without it only ATMOS_STACK would be
	// honored and the flag would be silently dropped. Use a fresh Viper so no
	// pre-Set "stack" key (which would outrank a bound flag) masks the binding.
	v := viper.New()
	c := &cobra.Command{Use: "ps"}
	c.Flags().String("stack", "", "")
	require.NoError(t, c.Flags().Set("stack", "ue2-dev"))

	require.NoError(t, containerParser.BindFlagsToViper(c, v))

	assert.Equal(t, "ue2-dev", v.GetString("stack"))
}

func TestStackFlagIsPersistent(t *testing.T) {
	// The shared --stack/-s and --dry-run flags are inherited by subcommands.
	require.NotNil(t, containerCmd.PersistentFlags().Lookup("stack"))
	require.NotNil(t, containerCmd.PersistentFlags().Lookup("dry-run"))
}
