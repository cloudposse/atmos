package packer

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Parser handles flag parsing for Packer commands.
// Returns strongly-typed Options.
type Parser struct {
	parser *flags.PassThroughFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewParser creates a parser for Packer commands.
func NewParser() *Parser {
	defer perf.Track(nil, "flagparser.NewPackerParser")()

	return &Parser{
		parser: flags.NewPassThroughFlagParser(flags.WithPackerFlags()),
	}
}

// RegisterFlags adds Packer flags to the Cobra command.
func (p *Parser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.PackerParser.RegisterFlags")()

	p.cmd = cmd
	// https://github.com/spf13/cobra/issues/739
	// DisableFlagParsing=true prevents Cobra from parsing flags, but flags can still be registered.
	// Our manual parsers extract flag values from os.Args directly.
	cmd.DisableFlagParsing = true
	// Packer passes subcommand separately to packerRun, so only extract 1 positional arg (component).
	p.parser.SetPositionalArgsCount(1)
	p.parser.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *Parser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.PackerParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// Parse processes command-line arguments and returns strongly-typed Options.
//
//nolint:dupl // Similar to HelmfileParser.Parse but returns different types
func (p *Parser) Parse(ctx context.Context, args []string) (*Options, error) {
	defer perf.Track(nil, "flagparser.PackerParser.Parse")()

	// Use underlying parser to extract Atmos flags and separate pass-through args.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Convert to strongly-typed Options using ParseFlags.
	opts := ParseFlags(p.cmd, p.viper, parsedConfig.PositionalArgs, parsedConfig.SeparatedArgs)

	return &opts, nil
}

// RegisterPersistentFlags adds Packer flags to the Cobra command as persistent flags (inherited by subcommands).
func (p *Parser) RegisterPersistentFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.PackerParser.RegisterPersistentFlags")()

	p.cmd = cmd
	// https://github.com/spf13/cobra/issues/739
	// DisableFlagParsing=true prevents Cobra from parsing flags, but flags can still be registered.
	// Our manual parsers extract flag values from os.Args directly.
	cmd.DisableFlagParsing = true
	// Packer passes subcommand separately to packerRun, so only extract 1 positional arg (component).
	p.parser.SetPositionalArgsCount(1)
	p.parser.RegisterPersistentFlags(cmd)
}

// BindFlagsToViper binds Cobra flags to Viper for CLI flag precedence.
func (p *Parser) BindFlagsToViper(cmd *cobra.Command, v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.PackerParser.BindFlagsToViper")()

	return p.parser.BindFlagsToViper(cmd, v)
}
