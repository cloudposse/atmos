package container

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exampleProjectPath resolves the bundled container-component example, which is a
// valid local project (no cloud credentials required) used to exercise the
// completion happy paths end-to-end.
func exampleProjectPath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	path, err := filepath.Abs(filepath.Join(wd, "..", "..", "examples", "container-component"))
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(path, "atmos.yaml"))
	require.NoErrorf(t, statErr, "example project not found at %s", path)
	return path
}

func TestStackFlagCompletion_ListsStacks(t *testing.T) {
	// Run from the example project dir so config discovery finds its atmos.yaml.
	t.Chdir(exampleProjectPath(t))
	withViperBasePath(t, "")

	stacks, directive := stackFlagCompletion(&cobra.Command{Use: "container"}, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, stacks, "local")
}

func TestComponentArgCompletion_ListsContainerComponents(t *testing.T) {
	t.Chdir(exampleProjectPath(t))
	withViperBasePath(t, "")

	// componentArgCompletion honors the --stack flag to scope suggestions; it
	// resolves the project and runs the describe→filter pipeline (vs. the
	// graceful-degrade path tested elsewhere), always returning NoFileComp.
	c := &cobra.Command{Use: "ps"}
	c.Flags().String("stack", "local", "")

	_, directive := componentArgCompletion(c, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}
