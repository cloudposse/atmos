package terraform

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Parser handles flag parsing for Terraform commands.
// Returns strongly-typed Options instead of weak map-based ParsedConfig.
type Parser struct {
	flagRegistry *flags.FlagRegistry
	cmd          *cobra.Command
	viper        *viper.Viper
}

// NewParser creates a parser for Terraform commands.
//
// Usage:
//
//	parser := terraform.NewParser()
//	parser.RegisterFlags(cmd)
//	parser.BindToViper(viper.GetViper())
//
//	// In command execution:
//	opts, err := parser.Parse(ctx, args)
//	if opts.Stack == "" {
//	    return errors.New("stack is required")
//	}
func NewParser() *Parser {
	defer perf.Track(nil, "terraform.NewParser")()

	return &Parser{
		flagRegistry: flags.TerraformFlags(),
	}
}

// RegisterFlags adds Terraform flags to the Cobra command.
func (p *Parser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "terraform.Parser.RegisterFlags")()

	p.cmd = cmd
	p.flagRegistry.RegisterFlags(cmd)
}

// RegisterPersistentFlags adds Terraform flags as persistent flags (inherited by subcommands).
func (p *Parser) RegisterPersistentFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.Parser.RegisterPersistentFlags")()

	p.cmd = cmd
	// https://github.com/spf13/cobra/issues/739
	// DisableFlagParsing=true prevents Cobra from parsing flags, but flags can still be registered.
	// Our manual parsers extract flag values from os.Args directly.
	cmd.DisableFlagParsing = true
	p.flagRegistry.RegisterPersistentFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *Parser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.Parser.BindToViper")()

	p.viper = v
	return p.flagRegistry.BindToViper(v)
}

// Parse processes command-line arguments and returns strongly-typed Options.
//
// This replaces the old ParsedConfig approach:
//
//	// ❌ Old way: Weak typing with runtime assertions
//	parsedConfig, err := parser.Parse(ctx, args)
//	stack := parsedConfig.Flags["stack"].(string)  // Can panic!
//
//	// ✅ New way: Strong typing with compile-time safety
//	opts, err := parser.Parse(ctx, args)
//	stack := opts.Stack  // Type-safe!
func (p *Parser) Parse(ctx context.Context, args []string) (*Options, error) {
	defer perf.Track(nil, "flagparser.Parser.Parse")()

	// Extract subcommand from args for per-command compatibility aliases.
	// The subcommand is the first positional argument (e.g., "plan", "apply", "init").
	subcommand := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		subcommand = args[0]
	}

	// Create compatibility translator for terraform flags.
	// This preprocesses args BEFORE Cobra sees them:
	//   - MapToAtmosFlag: -s → --stack (normalized to Cobra format)
	//   - AppendToSeparated: -var, -var-file, -out, etc → separated args (pass-through to terraform)
	// Uses per-command compatibility aliases based on subcommand.
	translator := flags.NewCompatibilityAliasTranslator(CompatibilityAliases(subcommand))

	// Create AtmosFlagParser with translator and registry.
	// This combines compatibility alias translation with standard Cobra flag parsing.
	// The registry enables NoOptDefVal preprocessing for identity and pager flags.
	flagParser := flags.NewAtmosFlagParser(p.cmd, p.viper, translator, p.flagRegistry)

	// Parse args using AtmosFlagParser.
	parsedConfig, err := flagParser.Parse(args)
	if err != nil {
		return nil, err
	}

	// Convert to strongly-typed Options using ParseFlags.
	opts := ParseFlags(p.cmd, p.viper, parsedConfig.PositionalArgs, parsedConfig.SeparatedArgs)
	return &opts, nil
}
