package flags

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// AuthExecParser handles flag parsing for auth exec and auth shell commands.
// Returns strongly-typed AuthExecOptions with identity flag and pass-through args.
type AuthExecParser struct {
	parser *PassThroughFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewAuthExecParser creates a parser for auth exec/shell commands.
//
// Usage:
//
//	parser := flags.NewAuthExecParser()
//	parser.RegisterFlags(cmd)
//	parser.BindToViper(viper.GetViper())
//
//	// In command execution:
//	opts, err := parser.Parse(ctx, args)
//	if opts.Identity.IsEmpty() {
//	    // Use default identity
//	}
func NewAuthExecParser() *AuthExecParser {
	defer perf.Track(nil, "flagparser.NewAuthExecParser")()

	// Create a registry with just the identity flag.
	registry := NewFlagRegistry()

	// Identity flag with NoOptDefVal for interactive selection.
	registry.Register(&StringFlag{
		Name:        cfg.IdentityFlagName,
		Shorthand:   "i",
		Default:     "",
		Description: "Identity to use for authentication (use without value to select interactively)",
		Required:    false,
		EnvVars:     []string{"ATMOS_IDENTITY", "IDENTITY"},
		NoOptDefVal: cfg.IdentityFlagSelectValue,
	})

	return &AuthExecParser{
		parser: NewPassThroughFlagParserFromRegistry(registry),
	}
}

// RegisterFlags adds auth exec flags to the Cobra command.
func (p *AuthExecParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.AuthExecParser.RegisterFlags")()

	p.cmd = cmd
	p.parser.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *AuthExecParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.AuthExecParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// Parse parses command-line arguments and returns strongly-typed AuthExecOptions.
func (p *AuthExecParser) Parse(ctx context.Context, args []string) (*AuthExecOptions, error) {
	defer perf.Track(nil, "flagparser.AuthExecParser.Parse")()

	// Parse with the underlying PassThroughFlagParser.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Extract identity value from parsed flags.
	identityValue := getString(parsedConfig.Flags, cfg.IdentityFlagName)

	// Build strongly-typed options.
	opts := &AuthExecOptions{
		GlobalFlags: GlobalFlags{
			LogsLevel: getString(parsedConfig.Flags, "logs-level"),
			LogsFile:  getString(parsedConfig.Flags, "logs-file"),
			NoColor:   getBool(parsedConfig.Flags, "no-color"),
		},
		Identity:        NewIdentityFlag(identityValue),
		PositionalArgs:  parsedConfig.PositionalArgs,
		PassThroughArgs: parsedConfig.PassThroughArgs,
	}

	return opts, nil
}
