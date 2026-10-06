package atmos

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

func testCatalog() *flags.CommandCatalog {
	root := &cobra.Command{Use: "atmos"}
	for _, name := range []string{"run", "terraform", "helm", "toolchain", "version", "vendor", "describe"} {
		command := &cobra.Command{Use: name}
		command.PersistentFlags().StringP("format", "f", "", "format")
		if name == "describe" {
			child := &cobra.Command{Use: "component"}
			child.Flags().StringP("query", "q", "", "query")
			command.AddCommand(child)
		}
		root.AddCommand(command)
	}
	return flags.NewCommandCatalog(root, func(path []string) map[string]compat.CompatibilityFlag {
		if path[0] == "terraform" {
			return map[string]compat.CompatibilityFlag{"-detailed-exitcode": {Behavior: compat.AppendToSeparated}}
		}
		return nil
	})
}

func TestModuleBuildsLiteralArguments(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		source   string
		argv     []string
		detailed bool
	}{
		{`atmos.run(["version","--help"])`, []string{"version", "--help"}, false},
		{`atmos.run(("version",))`, []string{"version"}, false},
		{`atmos.version()`, []string{"version"}, false},
		{`atmos.describe("component","api",flags={"f":"json"},args=["--help"])`, []string{"describe", "component", "api", "--format=json", "--help"}, false},
		{`atmos.describe(args=["component","api"],flags={"q":"vars"})`, []string{"describe", "--query=vars", "component", "api"}, false},
		{`atmos.vendor("pull",flags={"component":["api","worker"],"dry-run":False,"verbose":True,"count":2})`, []string{"vendor", "pull", "--component=api", "--component=worker", "--count=2", "--dry-run=false", "--verbose"}, false},
		{`atmos.terraform("plan","api","dev",flags={"detailed-exitcode":True})`, []string{"terraform", "plan", "api", "--stack=dev", "-detailed-exitcode"}, true},
		{`atmos.terraform("plan","api","dev",args=["-detailed-exitcode=true","-detailed-exitcode=false"])`, []string{"terraform", "plan", "api", "--stack=dev", "-detailed-exitcode=true", "-detailed-exitcode=false"}, false},
		{`atmos.helm("template","api","dev",flags={"--values":"two words;$HOME"})`, []string{"helm", "template", "api", "--stack=dev", "--values=two words;$HOME"}, false},
		{`atmos.toolchain("install","helm",flags={"version":"3.1"},args=["--help"])`, []string{"toolchain", "install", "helm", "--version=3.1", "--help"}, false},
		{`atmos.toolchain("list")`, []string{"toolchain", "list"}, false},
	} {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()
			calls := 0
			module := New("project", testCatalog(), func(_ *starlark.Thread, argv []string, opts Options, detailed bool) (starlark.Value, error) {
				calls++
				assert.Equal(t, tc.argv, argv)
				assert.Equal(t, tc.detailed, detailed)
				assert.Equal(t, Options{Dir: "project", Output: "stream", Check: true}, opts)
				return starlark.String("host result"), nil
			})
			result, err := starlark.EvalOptions(&syntax.FileOptions{}, &starlark.Thread{}, "atmos.star", tc.source, starlark.StringDict{"atmos": module})
			require.NoError(t, err)
			assert.Equal(t, starlark.String("host result"), result)
			assert.Equal(t, 1, calls)
		})
	}
}

func TestModulePreservesInvocationPolicies(t *testing.T) {
	t.Parallel()
	for _, call := range []string{`atmos.run(["version"],`, `atmos.vendor("pull",`, `atmos.toolchain("list",`, `atmos.helm("template","api","dev",`} {
		t.Run(call, func(t *testing.T) {
			t.Parallel()
			calls := 0
			module := New("project", testCatalog(), func(_ *starlark.Thread, _ []string, opts Options, _ bool) (starlark.Value, error) {
				calls++
				assert.Equal(t, "child", opts.Dir)
				assert.Equal(t, "capture", opts.Output)
				assert.False(t, opts.Check)
				require.NotNil(t, opts.Env)
				value, found, err := opts.Env.Get(starlark.String("KEY"))
				require.NoError(t, err)
				require.True(t, found)
				assert.Equal(t, starlark.String("value"), value)
				return starlark.None, nil
			})
			_, err := starlark.EvalOptions(&syntax.FileOptions{}, &starlark.Thread{}, "atmos.star", call+`working_directory="child",env={"KEY":"value"},output="capture",check=False)`, starlark.StringDict{"atmos": module})
			require.NoError(t, err)
			assert.Equal(t, 1, calls)
		})
	}
}

func TestModuleRejectsInvalidCallsBeforeRunning(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`atmos.run()`, `atmos.run("version")`, `atmos.run([])`, `atmos.run([""])`, `atmos.run([1])`,
		`atmos.terraform("plan")`, `atmos.terraform("", "api", "dev")`, `atmos.terraform("plan", "-api", "dev")`,
		`atmos.terraform("plan","api","dev",flags={"s":"other"})`, `atmos.helm("template","api","dev",args=1)`,
		`atmos.toolchain()`, `atmos.toolchain("")`, `atmos.toolchain("install","--help")`,
		`atmos.toolchain("list",flags={"":True})`, `atmos.toolchain("list",args=[1])`,
		`atmos.version(unknown=True)`, `atmos.version(1)`, `atmos.version(args="invalid")`,
		`atmos.version(flags={1:True})`, `atmos.version(flags={"has space":True})`, `atmos.version(flags={"a=b":True})`,
		`atmos.version(flags={"f":"json","format":"yaml"})`, `atmos.version(flags={"a":None})`, `atmos.version(flags={"a":[None]})`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			module := New("project", testCatalog(), func(*starlark.Thread, []string, Options, bool) (starlark.Value, error) {
				t.Error("invalid call must not reach the host runner")
				return starlark.None, nil
			})
			_, err := starlark.EvalOptions(&syntax.FileOptions{}, &starlark.Thread{}, "atmos.star", source, starlark.StringDict{"atmos": module})
			require.Error(t, err)
		})
	}
}
