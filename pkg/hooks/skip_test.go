package hooks

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// newSkipHooksCmd builds a cobra.Command carrying a `skip-hooks` flag that
// mirrors the real global flag (NoOptDefVal="*"), so ParseFlags sets `Changed`
// exactly the way Cobra does at runtime. This exercises ResolveSkipHooks
// against a genuine parsed flag — not a viper.Set shortcut, which is what let
// the before-event skip bug slip through.
func newSkipHooksCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "plan"}
	cmd.Flags().String("skip-hooks", "", "")
	cmd.Flags().Lookup("skip-hooks").NoOptDefVal = "*"
	return cmd
}

// TestResolveSkipHooks locks in the precedence that the bug violated: the
// explicitly-set CLI flag wins, with viper (ATMOS_SKIP_HOOKS / default) as the
// fallback. Before the fix, RunAll read viper.GetString directly — which during
// PreRunE (before BindFlagsToViper runs in RunE) never sees the CLI value, so
// before-* hooks were never skipped. These cases would fail against that
// implementation.
func TestResolveSkipHooks(t *testing.T) {
	t.Run("nil cmd falls back to viper", func(t *testing.T) {
		prev := viper.Get("skip-hooks")
		viper.Set("skip-hooks", "cost")
		t.Cleanup(func() { viper.Set("skip-hooks", prev) })

		assert.Equal(t, "cost", ResolveSkipHooks(nil))
	})

	t.Run("bare --skip-hooks resolves to star via NoOptDefVal", func(t *testing.T) {
		cmd := newSkipHooksCmd(t)
		require := assert.New(t)
		require.NoError(cmd.ParseFlags([]string{"--skip-hooks"}))

		assert.Equal(t, "*", ResolveSkipHooks(cmd))
	})

	t.Run("--skip-hooks=list resolves to the list", func(t *testing.T) {
		cmd := newSkipHooksCmd(t)
		assert.NoError(t, cmd.ParseFlags([]string{"--skip-hooks=cost,security"}))

		assert.Equal(t, "cost,security", ResolveSkipHooks(cmd))
	})

	t.Run("flag not changed falls back to viper env value", func(t *testing.T) {
		prev := viper.Get("skip-hooks")
		viper.Set("skip-hooks", "audit")
		t.Cleanup(func() { viper.Set("skip-hooks", prev) })

		cmd := newSkipHooksCmd(t)
		// No ParseFlags / flag not present on the command line → Changed=false.
		assert.Equal(t, "audit", ResolveSkipHooks(cmd))
	})

	t.Run("explicit CLI flag wins over viper", func(t *testing.T) {
		prev := viper.Get("skip-hooks")
		viper.Set("skip-hooks", "from-env")
		t.Cleanup(func() { viper.Set("skip-hooks", prev) })

		cmd := newSkipHooksCmd(t)
		assert.NoError(t, cmd.ParseFlags([]string{"--skip-hooks=cost"}))

		assert.Equal(t, "cost", ResolveSkipHooks(cmd))
	})
}

func TestNewSkipPredicate(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		hookName string
		wantSkip bool
	}{
		{name: "empty runs everything", raw: "", hookName: "cost", wantSkip: false},
		{name: "explicit false runs everything", raw: "false", hookName: "cost", wantSkip: false},
		{name: "False case-insensitive", raw: "False", hookName: "cost", wantSkip: false},

		{name: "star skips all", raw: "*", hookName: "anything", wantSkip: true},
		{name: "true skips all", raw: "true", hookName: "anything", wantSkip: true},
		{name: "True case-insensitive", raw: "True", hookName: "anything", wantSkip: true},

		{name: "single name matches", raw: "cost", hookName: "cost", wantSkip: true},
		{name: "single name doesn't match other", raw: "cost", hookName: "security", wantSkip: false},

		{name: "comma list matches first", raw: "cost,security", hookName: "cost", wantSkip: true},
		{name: "comma list matches second", raw: "cost,security", hookName: "security", wantSkip: true},
		{name: "comma list misses absent name", raw: "cost,security", hookName: "audit", wantSkip: false},

		{name: "tolerates whitespace around names", raw: "  cost ,  security ", hookName: "security", wantSkip: true},
		{name: "tolerates trailing comma", raw: "cost,", hookName: "cost", wantSkip: true},
		{name: "tolerates empty list element", raw: ",,cost", hookName: "cost", wantSkip: true},

		// Hook name is case-sensitive — matching by exact name is the contract
		// users see, mirroring how stack YAML keys are matched.
		{name: "case sensitive miss", raw: "Cost", hookName: "cost", wantSkip: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pred := NewSkipPredicate(tt.raw)
			assert.Equal(t, tt.wantSkip, pred(tt.hookName))
		})
	}
}
