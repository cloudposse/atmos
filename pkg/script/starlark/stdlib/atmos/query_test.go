package atmos

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/cloudposse/atmos/pkg/flags"
)

func queryCatalog() *flags.CommandCatalog {
	root := &cobra.Command{Use: "atmos"}
	for _, name := range []string{"list", "describe", "config", "stack"} {
		command := &cobra.Command{Use: name}
		root.AddCommand(command)
		for _, sub := range []string{"affected", "get", "set", "plain"} {
			child := &cobra.Command{Use: sub}
			if sub != "plain" {
				child.Flags().StringP("format", "f", "", "")
				child.Flags().String("stack", "", "")
			}
			command.AddCommand(child)
		}
		if name == "stack" {
			config := &cobra.Command{Use: "config"}
			get := &cobra.Command{Use: "get"}
			get.Flags().StringP("format", "f", "", "")
			config.AddCommand(get)
			command.AddCommand(config)
		}
	}
	return flags.NewCommandCatalog(root, nil)
}

func TestQueryDefaults(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		source, output string
		argv           []string
	}{
		{`atmos.describe("affected")`, "capture", []string{"describe", "affected", "--format=json"}},
		{`atmos.list("affected")`, "capture", []string{"list", "affected", "--format=json"}},
		{`atmos.describe(args=["affected"])`, "capture", []string{"describe", "affected", "--format=json"}},
		{`atmos.list(args=["affected"], flags={"stack":"dev"})`, "capture", []string{"list", "--stack=dev", "affected", "--format=json"}},
		{`atmos.config("get", "logs.level")`, "capture", []string{"config", "get", "logs.level", "--format=json"}},
		{`atmos.stack("config", "get", "vars.region")`, "capture", []string{"stack", "config", "get", "vars.region", "--format=json"}},
		{`atmos.stack("get", "vars.region")`, "capture", []string{"stack", "get", "vars.region", "--format=json"}},
		{`atmos.config("set", "logs.level", "Info")`, "stream", []string{"config", "set", "logs.level", "Info"}},
		{`atmos.list("plain")`, "capture", []string{"list", "plain"}},
		{`atmos.list("affected", flags={"f":"yaml"}, output="stream")`, "stream", []string{"list", "affected", "--format=yaml"}},
		{`atmos.list("affected", args=["-fyaml"])`, "capture", []string{"list", "affected", "-fyaml"}},
		{`atmos.describe("affected", args=["--format", "yaml"])`, "capture", []string{"describe", "affected", "--format", "yaml"}},
		{`atmos.run(["describe", "affected"])`, "stream", []string{"describe", "affected"}},
	} {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()
			module := New("project", queryCatalog(), func(_ *starlark.Thread, argv []string, opts Options, _ bool) (starlark.Value, error) {
				assert.Equal(t, tc.argv, argv)
				assert.Equal(t, tc.output, opts.Output)
				return starlark.None, nil
			})
			_, err := starlark.EvalOptions(&syntax.FileOptions{}, &starlark.Thread{}, "query.star", tc.source, starlark.StringDict{"atmos": module})
			require.NoError(t, err)
		})
	}
}
