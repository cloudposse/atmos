package flags

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// AuthShellParser handles flag parsing for auth shell command.
// Returns strongly-typed AuthShellOptions with identity, shell, and pass-through args.
type AuthShellParser struct {
	parser *PassThroughFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewAuthShellParser creates a parser for auth shell command.
//
// Usage:
//
//	parser := flags.NewAuthShellParser()
//	parser.RegisterFlags(cmd)
//	parser.BindToViper(viper.GetViper())
//
//	// In command execution:
//	opts, err := parser.Parse(ctx, args)
//	if opts.Identity.IsEmpty() {
//	    // Use default identity
//	}
func NewAuthShellParser() *AuthShellParser {
	defer perf.Track(nil, "flagparser.NewAuthShellParser")()

	// Create a registry with identity and shell flags.
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

	// Shell flag.
	registry.Register(&StringFlag{
		Name:        "shell",
		Shorthand:   "s",
		Default:     "",
		Description: "Specify the shell to use (defaults to $SHELL, then bash, then sh)",
		Required:    false,
		EnvVars:     []string{"ATMOS_SHELL", "SHELL"},
	})

	return &AuthShellParser{
		parser: NewPassThroughFlagParserFromRegistry(registry),
	}
}

// RegisterFlags adds auth shell flags to the Cobra command.
func (p *AuthShellParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.AuthShellParser.RegisterFlags")()

	p.cmd = cmd
	p.parser.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *AuthShellParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.AuthShellParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// Parse parses command-line arguments and returns strongly-typed AuthShellOptions.
func (p *AuthShellParser) Parse(ctx context.Context, args []string) (*AuthShellOptions, error) {
	defer perf.Track(nil, "flagparser.AuthShellParser.Parse")()

	// Parse with the underlying PassThroughFlagParser.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Extract values from parsed flags.
	identityValue := getString(parsedConfig.Flags, cfg.IdentityFlagName)
	shellValue := getString(parsedConfig.Flags, "shell")

	// Build strongly-typed options.
	opts := &AuthShellOptions{
		GlobalFlags: GlobalFlags{
			LogsLevel: getString(parsedConfig.Flags, "logs-level"),
			LogsFile:  getString(parsedConfig.Flags, "logs-file"),
			NoColor:   getBool(parsedConfig.Flags, "no-color"),
		},
		Identity:        NewIdentityFlag(identityValue),
		Shell:           shellValue,
		PositionalArgs:  parsedConfig.PositionalArgs,
		PassThroughArgs: parsedConfig.PassThroughArgs,
	}

	return opts, nil
}
