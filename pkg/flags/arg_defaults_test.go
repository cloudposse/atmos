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

	t.Run("split-token default is fully removed when CLI sets the flag", func(t *testing.T) {
		rawSplit := map[string]any{
			"describe": map[string]any{
				"args": []any{"--identity", "default"},
			},
		}
		// Both the flag and its paired value token must be dropped; otherwise "default"
		// is left orphaned as a positional argument.
		got := InjectDefaultArgs(root, rawSplit, []string{"describe", "component", "--identity=cli"})
		assert.Equal(t, []string{"describe", "component", "--identity=cli"}, got)
	})

	t.Run("split-token default is fully removed when ENV sets the flag", func(t *testing.T) {
		t.Setenv("ATMOS_IDENTITY", "me")
		rawSplit := map[string]any{
			"describe": map[string]any{
				"args": []any{"--identity", "default"},
			},
		}
		got := InjectDefaultArgs(root, rawSplit, []string{"describe", "component", "vpc"})
		assert.Equal(t, []string{"describe", "component", "vpc"}, got)
	})

	t.Run("split-token default is injected (flag and value adjacent) when not suppressed", func(t *testing.T) {
		rawSplit := map[string]any{
			"describe": map[string]any{
				"args": []any{"--identity", "default"},
			},
		}
		// Tokens stay adjacent so the NoOptDefVal preprocessor can later pair them.
		got := InjectDefaultArgs(root, rawSplit, []string{"describe", "component", "vpc"})
		assert.Equal(t, []string{"describe", "component", "--identity", "default", "vpc"}, got)
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

	t.Run("help short form -h is never mutated", func(t *testing.T) {
		got := InjectDefaultArgs(root, raw, []string{"describe", "component", "-h"})
		assert.Equal(t, []string{"describe", "component", "-h"}, got)
	})

	t.Run("unknown command is a no-op", func(t *testing.T) {
		in := []string{"nonexistent", "sub"}
		assert.Equal(t, in, InjectDefaultArgs(root, raw, in))
	})

	t.Run("empty raw config is a no-op", func(t *testing.T) {
		in := []string{"describe", "component", "vpc"}
		assert.Equal(t, in, InjectDefaultArgs(root, nil, in))
	})
}

func TestSkipDefaultArgsForCommand(t *testing.T) {
	assert.True(t, skipDefaultArgsForCommand(nil, nil), "empty command path is skipped")
	assert.True(t, skipDefaultArgsForCommand([]string{"help"}, nil), "help is skipped")
	assert.True(t, skipDefaultArgsForCommand([]string{"completion", "bash"}, nil), "completion is skipped")
	assert.True(t, skipDefaultArgsForCommand([]string{"__complete"}, nil), "cobra completion command is skipped")
	assert.True(t, skipDefaultArgsForCommand([]string{"describe", "component"}, []string{"--help"}), "--help in rest is skipped")
	assert.True(t, skipDefaultArgsForCommand([]string{"describe", "component"}, []string{"-h"}), "-h in rest is skipped")
	assert.False(t, skipDefaultArgsForCommand([]string{"describe", "component"}, []string{"-s", "dev"}), "normal command is not skipped")
}

func TestDefaultArgHasSeparateValue(t *testing.T) {
	// Attached value (--flag=value): nothing separate to drop.
	assert.False(t, defaultArgHasSeparateValue([]string{"--identity=admin", "default"}, 0))
	// Bare flag followed by a positional value: the value pairs with the flag.
	assert.True(t, defaultArgHasSeparateValue([]string{"--identity", "default"}, 0))
	// Bare flag at the end of the list: no following token to pair with.
	assert.False(t, defaultArgHasSeparateValue([]string{"--identity"}, 0))
	// Bare flag followed by another flag: the next token is not a value.
	assert.False(t, defaultArgHasSeparateValue([]string{"--identity", "--process-functions"}, 0))
}

func TestFilterDefaultArgs(t *testing.T) {
	t.Run("nothing suppressed passes through unchanged", func(t *testing.T) {
		got := filterDefaultArgs([]string{"--identity", "default", "--skip=x"}, nil)
		assert.Equal(t, []string{"--identity", "default", "--skip=x"}, got)
	})

	t.Run("split-token suppression drops both flag and value", func(t *testing.T) {
		got := filterDefaultArgs([]string{"--identity", "default", "--skip=x"}, []string{"--identity=cli"})
		assert.Equal(t, []string{"--skip=x"}, got)
	})

	t.Run("attached-value suppression drops only the flag token", func(t *testing.T) {
		got := filterDefaultArgs([]string{"--skip=x", "positional"}, []string{"--skip=y"})
		assert.Equal(t, []string{"positional"}, got)
	})
}
