package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	ansiEscape   = "\x1b["
	testSecret   = "doctor-startup-secret-value"
	colorEnvTrue = "true"
)

// colorEnvVars are the process-level switches that choose a color profile ahead of any parsed flag.
var colorEnvVars = []string{"NO_COLOR", "ATMOS_NO_COLOR", "ATMOS_FORCE_COLOR", "FORCE_COLOR", "CLICOLOR_FORCE"}

// startupHarness runs doctor beneath a root that carries the real global persistent flags, the way the
// executable dispatches it, starting from the writers and masker that exist before any flag is parsed.
type startupHarness struct {
	stderrPath string
	root       *cobra.Command
}

func newStartupHarness(t *testing.T) *startupHarness {
	t.Helper()
	isolateAmbientConfig(t)
	newProjectDir(t, map[string]string{"atmos.yaml": legacyCommandConfig})
	for _, name := range colorEnvVars {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}

	viper.Reset()
	iolib.Reset()
	t.Cleanup(func() {
		viper.Reset()
		iolib.Reset()
		ui.Reset()
	})

	// Mirror the root command: global flags are persistent flags bound to the global Viper.
	parser := flags.NewGlobalOptionsBuilder().Build()
	root := &cobra.Command{Use: "atmos", SilenceUsage: true, SilenceErrors: true}
	parser.RegisterPersistentFlags(root)
	require.NoError(t, parser.BindToViper(viper.GetViper()))
	// The root also binds --mask to Viper explicitly.
	require.NoError(t, viper.BindPFlag("mask", root.PersistentFlags().Lookup("mask")))
	root.AddCommand(NewCommand())

	// Route the process stderr to a file; the I/O layer resolves os.Stderr at write time.
	stderrFile, err := os.Create(filepath.Join(t.TempDir(), "stderr.txt"))
	require.NoError(t, err)
	previous := os.Stderr
	os.Stderr = stderrFile
	t.Cleanup(func() {
		os.Stderr = previous
		_ = stderrFile.Close()
	})

	// The root's init() builds the writers before flags are parsed, from defaults and the environment only.
	return &startupHarness{stderrPath: stderrFile.Name(), root: root}
}

// initPreParseWriters builds the writers the way the root's init() does, before any flag is parsed.
func initPreParseWriters(t *testing.T) {
	t.Helper()
	ctx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ctx)
	data.InitWriter(ctx)
}

// run executes `doctor --offline <args>` and returns everything written to stderr.
func (h *startupHarness) run(t *testing.T, args ...string) string {
	t.Helper()
	initPreParseWriters(t)
	argv := append([]string{"doctor", "--offline"}, args...)
	// Several color decisions read the process arguments directly, as they do in the executable.
	previous := os.Args
	os.Args = append([]string{"atmos"}, argv...)
	t.Cleanup(func() { os.Args = previous })
	h.root.SetArgs(argv)
	require.NoError(t, h.root.Execute())
	out, err := os.ReadFile(h.stderrPath)
	require.NoError(t, err)
	return string(out)
}

func TestStartupAppliesColorFlags(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		args      []string
		wantColor bool
	}{
		{name: "plain stderr file has no color by default", wantColor: false},
		// The executable's init() turns a --force-color found in the process arguments into CLICOLOR_FORCE.
		{name: "--force-color emits ANSI on a non-TTY", env: map[string]string{"CLICOLOR_FORCE": "1"}, args: []string{"--force-color"}, wantColor: true},
		{name: "--no-color beats color forced by the environment", env: map[string]string{"ATMOS_FORCE_COLOR": colorEnvTrue}, args: []string{"--no-color"}, wantColor: false},
		{name: "environment alone forces color", env: map[string]string{"ATMOS_FORCE_COLOR": colorEnvTrue}, wantColor: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newStartupHarness(t)
			for name, value := range tt.env {
				t.Setenv(name, value)
			}

			output := h.run(t, tt.args...)

			require.Contains(t, output, "Legacy command-provides declaration", "the report must reach the redirected stderr")
			if tt.wantColor {
				assert.Contains(t, output, ansiEscape)
			} else {
				assert.NotContains(t, output, ansiEscape)
			}
		})
	}
}

func TestStartupAppliesMaskFlag(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantEnabled bool
	}{
		{name: "masking stays on by default", wantEnabled: true},
		{name: "--mask=false disables masking", args: []string{"--mask=false"}, wantEnabled: false},
		{name: "--mask=true keeps masking on", args: []string{"--mask=true"}, wantEnabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newStartupHarness(t)
			// The writers that exist before parsing mask a registered secret.
			initPreParseWriters(t)
			require.NoError(t, iolib.Initialize())
			iolib.RegisterValue(testSecret)
			require.True(t, iolib.MaskingEnabled(), "precondition: masking is on before flags are parsed")
			require.NotEqual(t, testSecret, iolib.MaskString(testSecret), "precondition: the secret is masked")

			h.run(t, tt.args...)

			assert.Equal(t, tt.wantEnabled, iolib.MaskingEnabled())
			if tt.wantEnabled {
				assert.NotEqual(t, testSecret, iolib.MaskString(testSecret))
			} else {
				assert.Equal(t, testSecret, iolib.MaskString(testSecret))
			}
		})
	}
}
