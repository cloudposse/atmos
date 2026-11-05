package startup

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

const testSecret = "startup-test-secret-value"

// resetGlobals isolates tests from the process-wide Viper, I/O context, and formatter.
func resetGlobals(t *testing.T) {
	t.Helper()
	viper.Reset()
	iolib.Reset()
	t.Setenv("NO_COLOR", "")
	t.Cleanup(func() {
		viper.Reset()
		iolib.Reset()
		ui.Reset()
		lipgloss.SetColorProfile(termenv.Ascii)
	})
}

// globalFlagsCommand builds a command carrying the global flags the startup reads and parses args against them,
// the way Cobra does before running a pre-run hook.
func globalFlagsCommand(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "atmos"}
	f := cmd.PersistentFlags()
	f.StringSlice(profileFlagName, nil, "")
	f.String(identityFlagName, "", "")
	f.String(editionFlagName, "", "")
	f.Bool("no-color", false, "")
	f.Bool("force-color", false, "")
	f.Bool(maskFlagName, true, "")
	require.NoError(t, cmd.ParseFlags(args))
	return cmd
}

func TestSyncGlobalFlagsToViper(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T)
	}{
		{
			name: "unchanged flags are not synced",
			check: func(t *testing.T) {
				assert.False(t, viper.IsSet(profileFlagName) || viper.IsSet(identityFlagName) || viper.IsSet(editionFlagName))
			},
		},
		{
			name: "changed flags reach Viper",
			args: []string{"--profile=dev", "--identity=admin", "--edition=2025-09"},
			check: func(t *testing.T) {
				assert.Equal(t, []string{"dev"}, viper.GetStringSlice(profileFlagName))
				assert.Equal(t, "admin", viper.GetString(identityFlagName))
				assert.Equal(t, "2025-09", viper.GetString(editionFlagName))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGlobals(t)
			cmd := globalFlagsCommand(t, tt.args...)

			SyncGlobalFlagsToViper(cmd)

			tt.check(t)
		})
	}
}

func TestConfigureEarlyColorProfile(t *testing.T) {
	tests := []struct {
		name    string
		noColor string
		args    []string
		want    termenv.Profile
	}{
		{name: "--no-color disables color", args: []string{"--no-color"}, want: termenv.Ascii},
		{name: "--force-color forces TrueColor", args: []string{"--force-color"}, want: termenv.TrueColor},
		{name: "NO_COLOR beats --force-color", noColor: "1", args: []string{"--force-color"}, want: termenv.Ascii},
		{name: "--no-color beats --force-color", args: []string{"--no-color", "--force-color"}, want: termenv.Ascii},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGlobals(t)
			t.Setenv("NO_COLOR", tt.noColor)
			// Start from the opposite profile so an ignored flag is visible.
			lipgloss.SetColorProfile(termenv.ANSI)
			cmd := globalFlagsCommand(t, tt.args...)

			ConfigureEarlyColorProfile(cmd)

			assert.Equal(t, tt.want, lipgloss.ColorProfile())
		})
	}
}

func TestReconcileMaskingHonorsShadowingFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "default keeps masking on", want: true},
		{name: "changed flag disables masking", args: []string{"--mask=false"}, want: false},
		{name: "changed flag keeps masking on", args: []string{"--mask=true"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGlobals(t)
			require.NoError(t, iolib.Initialize())
			require.True(t, iolib.MaskingEnabled())
			parent := globalFlagsCommand(t, tt.args...)
			leaf := &cobra.Command{Use: "leaf"}
			parent.AddCommand(leaf)

			ReconcileMasking(leaf, nil)

			assert.Equal(t, tt.want, iolib.MaskingEnabled())
		})
	}
}

func TestReconcileMaskingToleratesNilCommand(t *testing.T) {
	resetGlobals(t)
	require.NoError(t, iolib.Initialize())

	assert.NotPanics(t, func() { ReconcileMasking(nil, nil) })
	assert.True(t, iolib.MaskingEnabled())
}

func TestConfigureRebuildsWritersFromParsedFlags(t *testing.T) {
	resetGlobals(t)
	// Writers from before flag parsing, as the root's init() builds them.
	early, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(early)
	data.InitWriter(early)
	require.NoError(t, iolib.Initialize())
	iolib.RegisterValue(testSecret)
	require.NotEqual(t, testSecret, iolib.MaskString(testSecret))

	cmd := globalFlagsCommand(t, "--mask=false", "--edition=2025-09")

	require.NoError(t, Configure(cmd))

	assert.False(t, iolib.MaskingEnabled(), "--mask=false must reach the masker")
	assert.Equal(t, testSecret, iolib.MaskString(testSecret))
	assert.Equal(t, "2025-09", viper.GetString(editionFlagName), "flags are synced to Viper")
	assert.NotNil(t, ui.Format, "the UI formatter is rebuilt")
}
