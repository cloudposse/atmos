package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// newDescribeTree builds a synthetic `atmos describe component` command tree.
func newDescribeTree() *cobra.Command {
	root := &cobra.Command{Use: "atmos"}
	describe := &cobra.Command{Use: "describe"}
	component := &cobra.Command{Use: "component", Run: func(*cobra.Command, []string) {}}
	stacks := &cobra.Command{Use: "stacks", Run: func(*cobra.Command, []string) {}}
	describe.AddCommand(component, stacks)
	root.AddCommand(describe)
	root.AddCommand(&cobra.Command{Use: "version", Run: func(*cobra.Command, []string) {}})
	return root
}

func TestToStringList(t *testing.T) {
	assert.Nil(t, toStringList(nil))
	assert.Equal(t, []string{"a"}, toStringList("a"))
	assert.Equal(t, []string{"a", "b"}, toStringList([]string{"a", "b"}))
	assert.Equal(t, []string{"--x=1", "--y"}, toStringList([]any{"--x=1", "--y"}))
	assert.Nil(t, toStringList(42))
}

func TestFlagNameFromArg(t *testing.T) {
	assert.Equal(t, "skip", flagNameFromArg("--skip=terraform.state"))
	assert.Equal(t, "process-functions", flagNameFromArg("--process-functions"))
	assert.Equal(t, "i", flagNameFromArg("-i=admin"))
	assert.Equal(t, "", flagNameFromArg("positional"))
}

func TestFlagPresentInArgs(t *testing.T) {
	assert.True(t, flagPresentInArgs("identity", []string{"-s", "dev", "--identity=admin"}))
	assert.True(t, flagPresentInArgs("identity", []string{"--identity", "admin"}))
	assert.False(t, flagPresentInArgs("identity", []string{"-s", "dev"}))
	// Anything after `--` is pass-through and must not count.
	assert.False(t, flagPresentInArgs("identity", []string{"--", "--identity=admin"}))
}

func TestCollectDefaultArgs(t *testing.T) {
	raw := map[string]any{
		"args": []any{"--global-flag"},
		"describe": map[string]any{
			"args": []any{"--process-functions=false"},
			"component": map[string]any{
				"args": []any{"--skip=terraform.state"},
			},
		},
	}

	t.Run("global + command + subcommand, most-specific last", func(t *testing.T) {
		got := collectDefaultArgs(raw, []string{"describe", "component"})
		assert.Equal(t, []string{"--global-flag", "--process-functions=false", "--skip=terraform.state"}, got)
	})

	t.Run("command level only when subcommand has none", func(t *testing.T) {
		got := collectDefaultArgs(raw, []string{"describe", "stacks"})
		assert.Equal(t, []string{"--global-flag", "--process-functions=false"}, got)
	})

	t.Run("global only for unrelated command", func(t *testing.T) {
		got := collectDefaultArgs(raw, []string{"version"})
		assert.Equal(t, []string{"--global-flag"}, got)
	})
}

func TestInjectDefaultArgs(t *testing.T) {
	root := newDescribeTree()
	raw := map[string]any{
		"describe": map[string]any{
			"args": []any{"--process-functions=false"},
		},
	}

	t.Run("injects after command path, before user args", func(t *testing.T) {
		got := InjectDefaultArgs(root, raw, []string{"describe", "component", "vpc", "-s", "dev"})
		assert.Equal(t,
			[]string{"describe", "component", "--process-functions=false", "vpc", "-s", "dev"},
			got)
	})

	t.Run("CLI flag suppresses the default (CLI wins)", func(t *testing.T) {
		got := InjectDefaultArgs(root, raw, []string{"describe", "component", "--process-functions=true"})
		assert.Equal(t, []string{"describe", "component", "--process-functions=true"}, got)
	})

	t.Run("ENV var suppresses the default (ENV wins)", func(t *testing.T) {
		t.Setenv("ATMOS_PROCESS_FUNCTIONS", "true")
		got := InjectDefaultArgs(root, raw, []string{"describe", "component", "vpc"})
		assert.Equal(t, []string{"describe", "component", "vpc"}, got, "default suppressed when env is set")
	})

	t.Run("describe.args applies to the whole describe subtree", func(t *testing.T) {
		got := InjectDefaultArgs(root, raw, []string{"describe", "stacks", "-s", "dev"})
		assert.Equal(t,
			[]string{"describe", "stacks", "--process-functions=false", "-s", "dev"},
			got)
	})

	t.Run("no defaults for an unrelated command", func(t *testing.T) {
		got := InjectDefaultArgs(root, raw, []string{"version"})
		assert.Equal(t, []string{"version"}, got)
	})

	t.Run("help is never mutated", func(t *testing.T) {
		got := InjectDefaultArgs(root, raw, []string{"describe", "component", "--help"})
		assert.Equal(t, []string{"describe", "component", "--help"}, got)
	})

	t.Run("empty raw config is a no-op", func(t *testing.T) {
		in := []string{"describe", "component", "vpc"}
		assert.Equal(t, in, InjectDefaultArgs(root, nil, in))
	})
}
