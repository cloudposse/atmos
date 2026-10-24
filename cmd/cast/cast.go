package cast

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/markdown"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

var castCmd = &cobra.Command{
	Use:   "cast",
	Short: "Play and render Atmos asciicast recordings",
}

const (
	renderFormatASCII = "ascii"
	renderFormatGIF   = "gif"
	renderFormatHTML  = "html"
	renderFormatJPEG  = "jpeg"
	renderFormatJPG   = "jpg"
	renderFormatMP4   = "mp4"
	renderFormatPNG   = "png"
	renderFlagFormat  = "format"
	renderFlagOutput  = "output"
)

var renderParser = newRenderParser()

var playCmd = &cobra.Command{
	Use:   "play <input.cast>",
	Short: "Play an asciicast recording in the terminal",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return asciicast.Play(args[0], os.Stdout)
	},
}

var renderCmd = &cobra.Command{
	Use:     "render <input.cast>",
	Short:   "Render an asciicast recording to GIF, MP4, HTML, ASCII, PNG, or JPEG",
	Example: markdown.CastRenderUsageMarkdown,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := renderParser.BindFlagsToViper(cmd, viper.GetViper()); err != nil {
			return err
		}
		opts, err := renderOptionsFromBoundFlags()
		if err != nil {
			return err
		}
		return asciicast.Render(args[0], &opts)
	},
}

func init() {
	renderParser.RegisterFlags(renderCmd)
	if err := renderParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	castCmd.AddCommand(playCmd, renderCmd)
	internal.Register(&CommandProvider{})
}

func newRenderParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithStringFlag(renderFlagOutput, "o", "", "Write rendered output to this path"),
		flags.WithEnvVars(renderFlagOutput, "ATMOS_CAST_RENDER_OUTPUT"),
		flags.WithStringFlag(renderFlagFormat, "f", "", "Output format (gif, mp4, html, ascii, png, jpg, jpeg)"),
		flags.WithEnvVars(renderFlagFormat, "ATMOS_CAST_RENDER_FORMAT"),
		flags.WithValidValues(renderFlagFormat, renderFormatGIF, renderFormatMP4, renderFormatHTML, renderFormatASCII, renderFormatPNG, renderFormatJPG, renderFormatJPEG),
	)
}

func renderOptionsFromBoundFlags() (asciicast.RenderOptions, error) {
	output := viper.GetString(renderFlagOutput)
	if output == "" {
		return asciicast.RenderOptions{}, fmt.Errorf("%w", errUtils.ErrMissingRenderOutput)
	}
	format := viper.GetString(renderFlagFormat)
	return renderOptionsForOutput(output, format, format != "")
}

func renderOptionsFromFlags(cmd *cobra.Command, args []string) (asciicast.RenderOptions, []string, error) {
	remainingArgs := append([]string(nil), args...)

	output, err := flags.ResolveExplicitStringFlag(cmd, remainingArgs, renderFlagOutput)
	if err != nil {
		return asciicast.RenderOptions{}, remainingArgs, err
	}
	remainingArgs = output.Args
	if !output.Changed || output.Value == "" {
		return asciicast.RenderOptions{}, remainingArgs, fmt.Errorf("%w", errUtils.ErrMissingRenderOutput)
	}

	format, err := flags.ResolveExplicitStringFlag(cmd, remainingArgs, renderFlagFormat)
	if err != nil {
		return asciicast.RenderOptions{}, remainingArgs, err
	}
	remainingArgs = format.Args

	opts, err := renderOptionsForOutput(output.Value, format.Value, format.Changed)
	if err != nil {
		return asciicast.RenderOptions{}, remainingArgs, err
	}
	return opts, remainingArgs, nil
}

func renderOptionsForOutput(output, format string, explicitFormat bool) (asciicast.RenderOptions, error) {
	outputFormat, hasOutputFormat := renderFormatFromExtension(output)
	if explicitFormat {
		selectedFormat, ok := normalizeRenderFormat(format)
		if !ok {
			return asciicast.RenderOptions{}, fmt.Errorf("%w: unsupported format: %s", errUtils.ErrInvalidFlag, format)
		}
		if hasOutputFormat && selectedFormat != outputFormat {
			return asciicast.RenderOptions{}, fmt.Errorf("%w: --format %s conflicts with output extension %s", errUtils.ErrInvalidFlag, format, filepath.Ext(output))
		}
		return renderOptionsForFormat(output, selectedFormat), nil
	}
	if !hasOutputFormat {
		return asciicast.RenderOptions{}, fmt.Errorf("%w: %s", errUtils.ErrUnsupportedCastOutputExtension, output)
	}
	return renderOptionsForFormat(output, outputFormat), nil
}

func renderFormatFromExtension(output string) (string, bool) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(output)), ".")
	return normalizeRenderFormat(ext)
}

func normalizeRenderFormat(format string) (string, bool) {
	switch strings.ToLower(format) {
	case renderFormatGIF, renderFormatMP4, renderFormatHTML, renderFormatASCII, renderFormatPNG:
		return strings.ToLower(format), true
	case renderFormatJPG, renderFormatJPEG:
		return renderFormatJPEG, true
	default:
		return "", false
	}
}

func renderOptionsForFormat(output, format string) asciicast.RenderOptions {
	switch format {
	case renderFormatGIF:
		return asciicast.RenderOptions{GIF: output}
	case renderFormatMP4:
		return asciicast.RenderOptions{MP4: output}
	case renderFormatHTML:
		return asciicast.RenderOptions{HTML: output}
	case renderFormatASCII:
		return asciicast.RenderOptions{ASCII: output}
	case renderFormatPNG:
		return asciicast.RenderOptions{PNG: output}
	case renderFormatJPEG:
		return asciicast.RenderOptions{JPEG: output}
	default:
		return asciicast.RenderOptions{}
	}
}

type CommandProvider struct{}

func (p *CommandProvider) GetCommand() *cobra.Command {
	return castCmd
}

func (p *CommandProvider) GetName() string {
	return "cast"
}

func (p *CommandProvider) GetGroup() string {
	return "Other Commands"
}

func (p *CommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (p *CommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (p *CommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

func (p *CommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

func (p *CommandProvider) IsExperimental() bool {
	return false
}
