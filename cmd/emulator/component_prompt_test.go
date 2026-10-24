package emulator

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestComponentArgCompletionListsEmulatorComponents(t *testing.T) {
	t.Chdir(exampleProjectPath(t))
	withViperBasePath(t, "")

	cmd := &cobra.Command{Use: "up"}
	cmd.Flags().String("stack", "local", "")
	components, directive := componentArgCompletion(cmd, nil, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, components, "aws")
}
