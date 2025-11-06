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
	flagRegistry *flags.FlagRegistry
	cmd          *cobra.Command
	viper        *viper.Viper
}

// NewParser creates a parser for Packer commands.
func NewParser() *Parser {
	defer perf.Track(nil, "flagparser.NewPackerParser")()

	return &Parser{
		flagRegistry: flags.PackerFlags(),
	}
}

// RegisterFlags adds Packer flags to the Cobra command.
func (p *Parser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.PackerParser.RegisterFlags")()

	p.cmd = cmd
	p.flagRegistry.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *Parser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.PackerParser.BindToViper")()

	p.viper = v
	return p.flagRegistry.BindToViper(v)
}

// Parse processes command-line arguments and returns strongly-typed Options.
//
//nolint:dupl // Similar to HelmfileParser.Parse but returns different types
func (p *Parser) Parse(ctx context.Context, args []string) (*Options, error) {
	defer perf.Track(nil, "flagparser.PackerParser.Parse")()

	// Create an empty compatibility translator (packer doesn't need compatibility aliases).
	// All args after the component are passed through to packer.
	translator := flags.NewCompatibilityAliasTranslator(map[string]flags.CompatibilityAlias{})

	// Create AtmosFlagParser with translator.
	flagParser := flags.NewAtmosFlagParser(p.cmd, p.viper, translator)

	// Parse args using AtmosFlagParser.
	parsedConfig, err := flagParser.Parse(args)
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
	p.flagRegistry.RegisterPersistentFlags(cmd)
}
