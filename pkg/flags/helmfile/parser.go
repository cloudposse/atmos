package helmfile

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Parser handles flag parsing for Helmfile commands.
// Returns strongly-typed Options.
type Parser struct {
	parser *flags.PassThroughFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewParser creates a parser for Helmfile commands.
func NewParser() *Parser {
	defer perf.Track(nil, "flagparser.NewHelmfileParser")()

	return &Parser{
		parser: flags.NewPassThroughFlagParser(flags.WithHelmfileFlags()),
	}
}

// RegisterFlags adds Helmfile flags to the Cobra command.
func (p *Parser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.HelmfileParser.RegisterFlags")()

	p.cmd = cmd
	// https://github.com/spf13/cobra/issues/739
	// DisableFlagParsing=true prevents Cobra from parsing flags, but flags can still be registered.
	// Our manual parsers extract flag values from os.Args directly.
	cmd.DisableFlagParsing = true
	// Helmfile passes subcommand separately to helmfileRun, so only extract 1 positional arg (component).
	p.parser.SetPositionalArgsCount(1)
	p.parser.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *Parser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.HelmfileParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// Parse processes command-line arguments and returns strongly-typed Options.
//
//nolint:dupl // Similar to PackerParser.Parse but returns different types
func (p *Parser) Parse(ctx context.Context, args []string) (*Options, error) {
	defer perf.Track(nil, "flagparser.HelmfileParser.Parse")()

	// Use underlying parser to extract Atmos flags and separate pass-through args.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Convert to strongly-typed Options using ParseFlags.
	opts := ParseFlags(p.cmd, p.viper, parsedConfig.PositionalArgs, parsedConfig.SeparatedArgs)
	return &opts, nil
}
