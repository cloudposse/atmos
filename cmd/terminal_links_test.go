package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

// newLinksTestCommand builds a child command that inherits the real global flags from a parent.
func newLinksTestCommand(t *testing.T, args []string) *cobra.Command {
	t.Helper()

	parent := &cobra.Command{Use: "atmos"}
	flags.NewGlobalOptionsBuilder().Build().RegisterPersistentFlags(parent)
	child := &cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}}
	parent.AddCommand(child)
	require.NoError(t, child.ParseFlags(args))
	return child
}

func TestResolveLinksSetting(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name       string
		args       []string
		configured string
		want       string
	}{
		{name: "flag set overrides config", args: []string{"--links", "clipboard"}, configured: "osc8", want: "clipboard"},
		{name: "flag set with equals form", args: []string{"--links=raw"}, configured: "", want: "raw"},
		{name: "flag unset uses config value", args: nil, configured: "osc8", want: "osc8"},
		{name: "flag unset and config empty", args: nil, configured: "", want: ""},
		{name: "explicit empty flag overrides config with auto", args: []string{"--links="}, configured: "raw", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newLinksTestCommand(t, tt.args)
			assert.Equal(t, tt.want, resolveLinksSetting(cmd, tt.configured))
		})
	}
}

func TestResolveLinksSetting_FlagNotRegistered(t *testing.T) {
	_ = NewTestKit(t)

	assert.Equal(t, "osc8", resolveLinksSetting(&cobra.Command{Use: "bare"}, "osc8"))
	assert.Equal(t, "osc8", resolveLinksSetting(nil, "osc8"))
}

func TestIsRootValueFlag_Links(t *testing.T) {
	assert.True(t, isRootValueFlag("--links"))
	assert.True(t, isRootValueFlag("--links=osc8"))
}
