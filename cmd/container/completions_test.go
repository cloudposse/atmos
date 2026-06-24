package container

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withViperBasePath points config discovery at an isolated (empty) directory so
// completion runs deterministically without a real Atmos project, and restores
// the prior value afterwards.
func withViperBasePath(t *testing.T, path string) {
	t.Helper()
	v := viper.GetViper()
	orig := v.GetString("base-path")
	t.Cleanup(func() { v.Set("base-path", orig) })
	v.Set("base-path", path)
}

func TestGlobalInfoForCompletion(t *testing.T) {
	withViperBasePath(t, "some/base")
	info := globalInfoForCompletion(&cobra.Command{Use: "container"})
	assert.Equal(t, "some/base", info.AtmosBasePath)
}

func TestComponentArgCompletion_AlreadyHasArg(t *testing.T) {
	// With a component already provided, completion offers nothing more.
	values, directive := componentArgCompletion(&cobra.Command{Use: "ps"}, []string{"api"}, "")
	assert.Nil(t, values)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestStackFlagCompletion_NoProjectDegradesGracefully(t *testing.T) {
	// Pointed at an empty directory there is no project to complete from; the
	// function still returns the NoFileComp directive rather than erroring.
	withViperBasePath(t, t.TempDir())
	_, directive := stackFlagCompletion(&cobra.Command{Use: "container"}, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestComponentArgCompletion_NoProjectDegradesGracefully(t *testing.T) {
	withViperBasePath(t, t.TempDir())
	_, directive := componentArgCompletion(&cobra.Command{Use: "ps"}, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestRegisterContainerCompletions(t *testing.T) {
	// Every subcommand gets the component-arg completion function attached.
	parent := &cobra.Command{Use: "container"}
	child := &cobra.Command{Use: "ps"}
	parent.AddCommand(child)

	RegisterContainerCompletions(parent)
	require.NotNil(t, child.ValidArgsFunction)
}
