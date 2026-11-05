package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGraduatedCommandTrees checks the real registered tree, including inherited
// annotations. The experimental-parent lookup drives both enforcement and notices.
func TestGraduatedCommandTrees(t *testing.T) {
	NewTestKit(t)
	for _, name := range []string{"ci", "auth", "toolchain", "emulator", "pro", "terraform source", "terraform workdir"} {
		t.Run(name, func(t *testing.T) {
			command, _, err := RootCmd.Find(strings.Fields(name))
			require.NoError(t, err)
			require.Equal(t, "atmos "+name, command.CommandPath())
			var visit func(*cobra.Command)
			visit = func(c *cobra.Command) {
				assert.Empty(t, findExperimentalParent(c), c.CommandPath())
				_, badge := getExperimentalBadge(c, false)
				assert.Empty(t, badge, c.CommandPath())
				for _, child := range c.Commands() {
					visit(child)
				}
			}
			visit(command)
		})
	}
	for _, name := range []string{"devcontainer", "ai", "helm", "kubernetes", "list affected", "terraform backend", "terraform cache", "terraform migrate"} {
		t.Run("still experimental/"+name, func(t *testing.T) {
			command, _, err := RootCmd.Find(strings.Fields(name))
			require.NoError(t, err)
			assert.NotEmpty(t, findExperimentalParent(command))
		})
	}
}
