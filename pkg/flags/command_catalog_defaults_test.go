package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCommandCatalogDefaultFlag(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "atmos"}
	list := &cobra.Command{Use: "list", Aliases: []string{"ls"}}
	child := &cobra.Command{Use: "components", Aliases: []string{"comps"}}
	child.Flags().StringP("format", "f", "table", "")
	child.Flags().String("query", "", "")
	list.AddCommand(child, &cobra.Command{Use: "plain"})
	root.AddCommand(list)
	catalog := NewCommandCatalog(root, nil)
	for _, tc := range []struct {
		args, want []string
	}{
		{[]string{"list", "components"}, []string{"list", "components", "--format=json"}},
		{[]string{"ls", "comps", "--", "--format=yaml"}, []string{"ls", "comps", "--format=json", "--", "--format=yaml"}},
		{[]string{"list", "components", "--query", "--format=yaml"}, []string{"list", "components", "--query", "--format=yaml", "--format=json"}},
		{[]string{"list", "components", "--query", "--"}, []string{"list", "components", "--query", "--", "--format=json"}},
	} {
		before := append([]string{}, tc.args...)
		assert.Equal(t, tc.want, catalog.WithDefaultFlag(tc.args, tc.args, "format", "json"))
		assert.Equal(t, before, tc.args)
	}
	for _, args := range [][]string{
		{"list", "components", "--format", "yaml"},
		{"list", "components", "--format=yaml"},
		{"list", "components", "-f", "yaml"},
		{"list", "components", "-f=yaml"},
		{"list", "components", "-fyaml"},
		{"list", "plain"},
		{"missing"},
		nil,
	} {
		assert.Equal(t, args, catalog.WithDefaultFlag(args, args, "format", "json"))
	}
	assert.Equal(t, []string{"list", "components"}, catalog.CommandPath([]string{"ls", "comps", "positional"}))
	path := catalog.CommandPath([]string{"list", "components"})
	path[0] = "changed"
	assert.Equal(t, []string{"list", "components"}, catalog.CommandPath([]string{"list", "components"}))
	var empty *CommandCatalog
	assert.Nil(t, empty.CommandPath(nil))
	assert.Nil(t, empty.WithDefaultFlag(nil, nil, "format", "json"))
}
