package cast

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	flagspkg "github.com/cloudposse/atmos/pkg/flags"
)

func TestRenderFlagsRegisteredThroughParser(t *testing.T) {
	if renderParser == nil {
		t.Fatal("expected render parser")
	}

	for _, name := range []string{"output", "format"} {
		if renderParser.Registry().Get(name) == nil {
			t.Fatalf("expected render parser registry to include %q", name)
		}
		if renderCmd.Flags().Lookup(name) == nil {
			t.Fatalf("expected render command to include %q flag", name)
		}
	}
	if renderParser.Registry().Count() != 2 {
		t.Fatalf("render parser flag count = %d, want 2", renderParser.Registry().Count())
	}
	for _, name := range []string{"gif", "mp4", "html", "ascii", "png", "jpg"} {
		if renderCmd.Flags().Lookup(name) != nil {
			t.Fatalf("did not expect render command to include removed %q flag", name)
		}
	}
}

func TestRenderOptionsFromFlags(t *testing.T) {
	gifOutput := filepath.Join(t.TempDir(), "out.gif")
	jpegOutput := filepath.Join(t.TempDir(), "out.jpeg")
	htmlOutput := filepath.Join(t.TempDir(), "out.custom")
	tests := []struct {
		name     string
		setFlags map[string]string
		want     asciicast.RenderOptions
	}{
		{
			name:     "infer gif from output extension",
			setFlags: map[string]string{"output": gifOutput},
			want:     asciicast.RenderOptions{GIF: gifOutput},
		},
		{
			name:     "infer jpeg from output extension",
			setFlags: map[string]string{"output": jpegOutput},
			want:     asciicast.RenderOptions{JPEG: jpegOutput},
		},
		{
			name: "use explicit format without recognized extension",
			setFlags: map[string]string{
				"output": htmlOutput,
				"format": "html",
			},
			want: asciicast.RenderOptions{HTML: htmlOutput},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetRenderCommand(t)
			for flag, value := range tt.setFlags {
				if err := renderCmd.Flags().Set(flag, value); err != nil {
					t.Fatal(err)
				}
			}

			opts, remainingArgs, err := renderOptionsFromFlags(renderCmd, []string{"input.cast"})
			if err != nil {
				t.Fatal(err)
			}
			if len(remainingArgs) != 1 || remainingArgs[0] != "input.cast" {
				t.Fatalf("remaining args = %#v, want input.cast", remainingArgs)
			}
			if opts != tt.want {
				t.Fatalf("options = %#v, want %#v", opts, tt.want)
			}
		})
	}
}

func TestRenderOptionsFromFlagsMissingOutput(t *testing.T) {
	resetRenderCommand(t)

	_, _, err := renderOptionsFromFlags(renderCmd, []string{"input.cast"})
	if !errors.Is(err, errUtils.ErrMissingRenderOutput) {
		t.Fatalf("error = %v, want ErrMissingRenderOutput", err)
	}
}

func TestRenderOptionsFromFlagsFormatConflict(t *testing.T) {
	resetRenderCommand(t)
	if err := renderCmd.Flags().Set("output", filepath.Join(t.TempDir(), "out.gif")); err != nil {
		t.Fatal(err)
	}
	if err := renderCmd.Flags().Set("format", "html"); err != nil {
		t.Fatal(err)
	}

	_, _, err := renderOptionsFromFlags(renderCmd, []string{"input.cast"})
	if !errors.Is(err, errUtils.ErrInvalidFlag) {
		t.Fatalf("error = %v, want ErrInvalidFlag", err)
	}
}

func TestRenderOptionsFromFlagsUnsupportedExtension(t *testing.T) {
	resetRenderCommand(t)
	if err := renderCmd.Flags().Set("output", filepath.Join(t.TempDir(), "out.custom")); err != nil {
		t.Fatal(err)
	}

	_, _, err := renderOptionsFromFlags(renderCmd, []string{"input.cast"})
	if !errors.Is(err, errUtils.ErrUnsupportedCastOutputExtension) {
		t.Fatalf("error = %v, want ErrUnsupportedCastOutputExtension", err)
	}
}

func TestRenderOptionsFromFlagsUnsupportedFormat(t *testing.T) {
	resetRenderCommand(t)
	if err := renderCmd.Flags().Set("output", filepath.Join(t.TempDir(), "out.custom")); err != nil {
		t.Fatal(err)
	}
	if err := renderCmd.Flags().Set("format", "bogus"); err != nil {
		t.Fatal(err)
	}

	_, _, err := renderOptionsFromFlags(renderCmd, []string{"input.cast"})
	if !errors.Is(err, errUtils.ErrInvalidFlag) {
		t.Fatalf("error = %v, want ErrInvalidFlag", err)
	}
}

func TestRenderOptionsFromBoundFlags(t *testing.T) {
	output := filepath.Join(t.TempDir(), "out.png")
	viper.Set(renderFlagOutput, output)
	viper.Set(renderFlagFormat, renderFormatPNG)
	t.Cleanup(func() {
		viper.Set(renderFlagOutput, "")
		viper.Set(renderFlagFormat, "")
	})

	opts, err := renderOptionsFromBoundFlags()
	if err != nil {
		t.Fatal(err)
	}
	if opts != (asciicast.RenderOptions{PNG: output}) {
		t.Fatalf("options = %#v, want PNG output", opts)
	}
}

func TestRenderOptionsFromBoundFlagsMissingOutput(t *testing.T) {
	viper.Set(renderFlagOutput, "")
	viper.Set(renderFlagFormat, "")
	t.Cleanup(func() {
		viper.Set(renderFlagOutput, "")
		viper.Set(renderFlagFormat, "")
	})

	_, err := renderOptionsFromBoundFlags()
	if !errors.Is(err, errUtils.ErrMissingRenderOutput) {
		t.Fatalf("error = %v, want ErrMissingRenderOutput", err)
	}
}

func TestCommandProvider(t *testing.T) {
	provider := &CommandProvider{}

	if provider.GetCommand() != castCmd {
		t.Fatal("provider returned unexpected command")
	}
	if provider.GetName() != "cast" {
		t.Fatalf("name = %q, want cast", provider.GetName())
	}
	if provider.GetGroup() != "Other Commands" {
		t.Fatalf("group = %q, want Other Commands", provider.GetGroup())
	}
	if provider.GetFlagsBuilder() != nil {
		t.Fatal("expected nil flags builder")
	}
	if provider.GetPositionalArgsBuilder() != nil {
		t.Fatal("expected nil positional args builder")
	}
	if provider.GetCompatibilityFlags() != nil {
		t.Fatal("expected nil compatibility flags")
	}
	if provider.GetAliases() != nil {
		t.Fatal("expected nil aliases")
	}
	if provider.IsExperimental() {
		t.Fatal("expected non-experimental provider")
	}
}

func resetRenderCommand(t *testing.T) {
	t.Helper()
	flagspkg.ResetCommandFlags(renderCmd)
}

// clearRenderViperOverrides removes any explicit viper.Set() override left by
// other tests (e.g. TestRenderOptionsFromBoundFlagsMissingOutput's cleanup)
// so that renderCmd.RunE's BindFlagsToViper -> BindPFlag wiring is not shadowed
// by a stale explicit value, which viper always prioritizes over bound flags.
func clearRenderViperOverrides(t *testing.T) {
	t.Helper()
	v := viper.GetViper()
	for _, key := range []string{renderFlagOutput, renderFlagFormat} {
		v.Set(key, nil)
	}
	t.Cleanup(func() {
		for _, key := range []string{renderFlagOutput, renderFlagFormat} {
			v.Set(key, nil)
		}
	})
}

func TestPlayCmdRunEPlaysCastFile(t *testing.T) {
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	castContent := `{"version":3,"term":{"cols":10,"rows":2}}` + "\n" + `[0,"o","hello\n"]` + "\n"
	if err := os.WriteFile(castPath, []byte(castContent), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := playCmd.RunE(playCmd, []string{castPath}); err != nil {
		t.Fatalf("play command failed: %v", err)
	}
}

func TestPlayCmdRunEReturnsErrorForMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.cast")
	if err := playCmd.RunE(playCmd, []string{missing}); err == nil {
		t.Fatal("expected an error for a missing cast file")
	}
}

func TestRenderCmdRunERendersFromBoundFlags(t *testing.T) {
	resetRenderCommand(t)
	clearRenderViperOverrides(t)
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	castContent := `{"version":3,"term":{"cols":10,"rows":2}}` + "\n" + `[0,"o","hello\n"]` + "\n"
	if err := os.WriteFile(castPath, []byte(castContent), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "out.html")
	if err := renderCmd.Flags().Set(renderFlagOutput, output); err != nil {
		t.Fatal(err)
	}

	if err := renderCmd.RunE(renderCmd, []string{castPath}); err != nil {
		t.Fatalf("render command failed: %v", err)
	}
	info, err := os.Stat(output)
	if err != nil || info.Size() == 0 {
		t.Fatalf("rendered output missing or empty: %v", err)
	}
}

func TestRenderCmdRunEReturnsErrorForMissingOutput(t *testing.T) {
	resetRenderCommand(t)
	clearRenderViperOverrides(t)
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	if err := os.WriteFile(castPath, []byte(`{"version":3,"term":{"cols":10,"rows":2}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := renderCmd.RunE(renderCmd, []string{castPath})
	if !errors.Is(err, errUtils.ErrMissingRenderOutput) {
		t.Fatalf("expected ErrMissingRenderOutput, got %v", err)
	}
}

func TestRenderCmdRunEReturnsRenderError(t *testing.T) {
	resetRenderCommand(t)
	clearRenderViperOverrides(t)
	missing := filepath.Join(t.TempDir(), "missing.cast")
	output := filepath.Join(t.TempDir(), "out.html")
	if err := renderCmd.Flags().Set(renderFlagOutput, output); err != nil {
		t.Fatal(err)
	}

	if err := renderCmd.RunE(renderCmd, []string{missing}); err == nil {
		t.Fatal("expected render to fail for a missing input cast file")
	}
}

func TestRenderOptionsForFormatDefaultReturnsEmptyOptions(t *testing.T) {
	opts := renderOptionsForFormat("out.bin", "unknown-format")
	if opts != (asciicast.RenderOptions{}) {
		t.Fatalf("options = %#v, want zero value", opts)
	}
}

func TestRenderOptionsForFormatMP4(t *testing.T) {
	output := "out.mp4"
	opts := renderOptionsForFormat(output, renderFormatMP4)
	if opts != (asciicast.RenderOptions{MP4: output}) {
		t.Fatalf("options = %#v, want MP4 output", opts)
	}
}
