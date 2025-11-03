package flags

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// TerraformParser handles flag parsing for Terraform commands.
// Returns strongly-typed TerraformOptions instead of weak map-based ParsedConfig.
type TerraformParser struct {
	parser *PassThroughFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewTerraformParser creates a parser for Terraform commands.
//
// Usage:
//
//	parser := flagparser.NewTerraformParser()
//	parser.RegisterFlags(cmd)
//	parser.BindToViper(viper.GetViper())
//
//	// In command execution:
//	interpreter, err := parser.Parse(ctx, args)
//	if interpreter.Stack == "" {
//	    return errors.New("stack is required")
//	}
func NewTerraformParser() *TerraformParser {
	defer perf.Track(nil, "flagparser.NewTerraformParser")()

	return &TerraformParser{
		parser: NewPassThroughFlagParser(WithTerraformFlags()),
	}
}

// RegisterFlags adds Terraform flags to the Cobra command.
func (p *TerraformParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.TerraformParser.RegisterFlags")()

	p.cmd = cmd
	p.parser.RegisterFlags(cmd)
}

// RegisterPersistentFlags adds Terraform flags as persistent flags (inherited by subcommands).
func (p *TerraformParser) RegisterPersistentFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.TerraformParser.RegisterPersistentFlags")()

	p.cmd = cmd
	p.parser.RegisterPersistentFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *TerraformParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.TerraformParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// Parse processes command-line arguments and returns strongly-typed TerraformOptions.
//
// This replaces the old ParsedConfig approach:
//
//	// ❌ Old way: Weak typing with runtime assertions
//	parsedConfig, err := parser.Parse(ctx, args)
//	stack := parsedConfig.Flags["stack"].(string)  // Can panic!
//
//	// ✅ New way: Strong typing with compile-time safety
//	interpreter, err := parser.Parse(ctx, args)
//	stack := interpreter.Stack  // Type-safe!
func (p *TerraformParser) Parse(ctx context.Context, args []string) (*TerraformOptions, error) {
	defer perf.Track(nil, "flagparser.TerraformParser.Parse")()

	// Use underlying parser to extract Atmos flags and separate pass-through args.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Convert to strongly-typed interpreter.
	opts := parsedConfig.ToTerraformOptions()
	return &opts, nil
}
