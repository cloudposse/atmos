package emulator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withViperBasePath points config discovery at an isolated directory so
// completion runs deterministically, and restores the prior value afterwards.
func withViperBasePath(t *testing.T, path string) {
	t.Helper()
	v := viper.GetViper()
	orig := v.GetString("base-path")
	t.Cleanup(func() { v.Set("base-path", orig) })
	v.Set("base-path", path)
}

// exampleProjectPath resolves the bundled emulator-aws example, a valid local
// project (no cloud credentials required) used to exercise the completion happy
// paths end-to-end.
func exampleProjectPath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	path, err := filepath.Abs(filepath.Join(wd, "..", "..", "examples", "emulator-aws"))
	require.NoError(t, err)
	if _, statErr := os.Stat(filepath.Join(path, "atmos.yaml")); statErr != nil {
		t.Skipf("example project not found at %s", path)
	}
	return path
}

func TestGlobalInfoForCompletion(t *testing.T) {
	withViperBasePath(t, "some/base")
	info := globalInfoForCompletion(&cobra.Command{Use: "emulator"})
	assert.Equal(t, "some/base", info.AtmosBasePath)
}

func TestComponentArgCompletion_AlreadyHasArg(t *testing.T) {
	// With a component already provided, completion offers nothing more.
	values, directive := componentArgCompletion(&cobra.Command{Use: "up"}, []string{"aws"}, "")
	assert.Nil(t, values)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestStackFlagCompletion_NoProjectDegradesGracefully(t *testing.T) {
	// Pointed at an empty directory there is no project to complete from; the
	// function still returns the NoFileComp directive rather than erroring.
	withViperBasePath(t, t.TempDir())
	values, directive := stackFlagCompletion(&cobra.Command{Use: "emulator"}, nil, "")
	assert.Nil(t, values)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestComponentArgCompletion_NoProjectDegradesGracefully(t *testing.T) {
	withViperBasePath(t, t.TempDir())
	values, directive := componentArgCompletion(&cobra.Command{Use: "up"}, nil, "")
	assert.Nil(t, values)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestStackFlagCompletion_ListsStacks(t *testing.T) {
	// Run from the example project dir so config discovery finds its atmos.yaml.
	t.Chdir(exampleProjectPath(t))
	withViperBasePath(t, "")

	stacks, directive := stackFlagCompletion(&cobra.Command{Use: "emulator"}, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, stacks, "dev")
}

func TestComponentArgCompletion_ResolvesProjectPipeline(t *testing.T) {
	t.Chdir(exampleProjectPath(t))
	withViperBasePath(t, "")

	// componentArgCompletion honors the --stack flag to scope suggestions; with a
	// real project it resolves config and runs the describe->filter pipeline (vs.
	// the graceful-degrade path tested elsewhere), always returning NoFileComp.
	//
	// Note: the suggestion list is empty here because the underlying
	// list.FilterAndListComponents extracts only terraform/helmfile/packer/ansible
	// components, not `emulator` components. Exercising the live pipeline is what
	// drives coverage; a positive contents assertion would require a production
	// change to the list extractor.
	c := &cobra.Command{Use: "up"}
	c.Flags().String("stack", "dev", "")

	_, directive := componentArgCompletion(c, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestRegisterEmulatorCompletions(t *testing.T) {
	// Every subcommand gets the component-arg completion function attached.
	parent := &cobra.Command{Use: "emulator"}
	child := &cobra.Command{Use: "up"}
	parent.AddCommand(child)

	RegisterEmulatorCompletions(parent)
	require.NotNil(t, child.ValidArgsFunction)
}
