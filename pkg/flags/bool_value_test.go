package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/flags/preprocess"
)

// Compile-time guard: BoolFlag must satisfy the optional preprocess.BoolFlagInfo interface.
var _ preprocess.BoolFlagInfo = (*BoolFlag)(nil)

func TestBoolFlag_IsBool(t *testing.T) {
	assert.True(t, (&BoolFlag{Name: "dry-run"}).IsBool())
	var s preprocess.FlagInfo = &StringFlag{Name: "stack"}
	_, ok := s.(preprocess.BoolFlagInfo)
	assert.False(t, ok, "string flags must not be reported as boolean")
}

func TestGlobalRegistry_ColorFlagsAreBool(t *testing.T) {
	registry := GlobalFlagsRegistry()
	for _, name := range []string{"no-color", "logs-color", "force-color"} {
		flag := registry.Get(name)
		require.NotNil(t, flag, name)
		b, ok := flag.(preprocess.BoolFlagInfo)
		require.True(t, ok, name)
		assert.True(t, b.IsBool(), name)
	}
}

func TestAtmosFlagParser_Parse_BoolFlagSpaceValue(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantDryRun     bool
		wantPositional []string
	}{
		{"false literal is the value", []string{"--dry-run", "false", "plan", "vpc"}, false, []string{"plan", "vpc"}},
		{"uppercase true literal is the value", []string{"--dry-run", "TRUE", "plan", "vpc"}, true, []string{"plan", "vpc"}},
		{"bare flag keeps the subcommand", []string{"--dry-run", "plan", "vpc"}, true, []string{"plan", "vpc"}},
		{"equals form", []string{"--dry-run=false", "plan", "vpc"}, false, []string{"plan", "vpc"}},
		{"flag after positionals", []string{"plan", "vpc", "--dry-run", "false"}, false, []string{"plan", "vpc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
			cmd.Flags().Bool("dry-run", false, "Dry run")
			registry := NewFlagRegistry()
			registry.RegisterBoolFlag("dry-run", "", false, "Dry run")

			v := viper.New()
			parser := NewAtmosFlagParser(cmd, v, compat.NewCompatibilityFlagTranslator(nil), registry)
			result, err := parser.Parse(tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDryRun, v.GetBool("dry-run"))
			assert.Equal(t, tt.wantPositional, result.PositionalArgs)
		})
	}
}
