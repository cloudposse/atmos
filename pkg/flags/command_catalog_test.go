package flags

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCommandCatalogUsesRegisteredFlagsAndCompatibility(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "atmos"}
	root.PersistentFlags().Bool("verbose", false, "")
	command := &cobra.Command{Use: "deploy", Aliases: []string{"ship"}}
	command.Flags().String("local", "", "")
	child := &cobra.Command{Use: "plan", Aliases: []string{"preview"}}
	child.Flags().StringP("stack", "s", "", "")
	command.AddCommand(child)
	root.AddCommand(command)
	compatibility := map[string]compat.CompatibilityFlag{
		"-native": {Behavior: compat.AppendToSeparated},
		"-legacy": {Behavior: compat.MapToAtmosFlag, Target: "--modern"},
	}
	catalog := NewCommandCatalog(root, func(_ []string) map[string]compat.CompatibilityFlag { return compatibility })
	assert.Equal(t, []string{"deploy", "ship"}, catalog.Names())
	for _, tc := range []struct{ key, want string }{
		{"verbose", "--verbose"},
		{"s", "--stack"},
		{"stack", "--stack"},
		{"native", "-native"},
		{"legacy", "--modern"},
		{"unknown", "--unknown"},
		{"-native", "-native"},
		{"--stack", "--stack"},
	} {
		assert.Equal(t, tc.want, catalog.FlagName([]string{"ship", "preview", "component"}, tc.key))
	}
	// A registered command and an existing snapshot remain isolated in both directions.
	compatibility["-native"] = compat.CompatibilityFlag{Behavior: compat.MapToAtmosFlag, Target: "--changed"}
	child.Flags().String("later", "", "")
	root.AddCommand(&cobra.Command{Use: "later"})
	names := catalog.Names()
	names[0] = "changed"
	assert.Equal(t, []string{"deploy", "ship"}, catalog.Names())
	assert.Equal(t, "-native", catalog.FlagName([]string{"deploy", "plan"}, "native"))
	assert.Equal(t, "--unknown", catalog.FlagName([]string{"missing"}, "unknown"))
	assert.Equal(t, "--unknown", catalog.FlagName(nil, "unknown"))
}

func TestCommandCatalogDoesNotInheritLocalShorthands(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "root"}
	parent := &cobra.Command{Use: "parent"}
	parent.Flags().StringP("local", "l", "", "")
	parent.PersistentFlags().StringP("persistent", "p", "", "")
	parent.AddCommand(&cobra.Command{Use: "child"})
	root.AddCommand(parent)
	catalog := NewCommandCatalog(root, nil)
	assert.Equal(t, "--local", catalog.FlagName([]string{"parent"}, "l"))
	assert.Equal(t, "--l", catalog.FlagName([]string{"parent", "child"}, "l"))
	assert.Equal(t, "--persistent", catalog.FlagName([]string{"parent", "child"}, "p"))
	var empty *CommandCatalog
	assert.Nil(t, empty.Names())
	assert.Equal(t, "--flag", empty.FlagName(nil, "flag"))
}

func TestCommandCatalogFindsSubcommandThroughRegisteredFlags(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "root"}
	parent := &cobra.Command{Use: "list"}
	parent.PersistentFlags().Bool("verbose", false, "")
	parent.PersistentFlags().StringP("format", "f", "", "")
	child := &cobra.Command{Use: "components"}
	child.Flags().StringP("stack", "s", "", "")
	parent.AddCommand(child)
	root.AddCommand(parent)
	catalog := NewCommandCatalog(root, nil)
	for _, args := range [][]string{
		{"list", "--verbose", "components"},
		{"list", "--format", "json", "components"},
		{"list", "-f=json", "components"},
		{"list", "--verbose=false", "--format=json", "components", "positional"},
	} {
		assert.Equal(t, "--stack", catalog.FlagName(args, "s"), args)
	}
	for _, args := range [][]string{
		{"list", "--", "components"},
		{"list", "--unknown", "components"},
		{"list", "positional", "components"},
		{"list", "--format", "components"},
	} {
		assert.Equal(t, "--s", catalog.FlagName(args, "s"), args)
	}
}
