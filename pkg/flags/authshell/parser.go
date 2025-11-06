package authshell

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/flags"
)

// AuthShellParser handles flag parsing for auth shell command.
// Returns strongly-typed AuthShellOptions with identity, shell, and pass-through args.
type AuthShellParser struct {
	flagRegistry *flags.FlagRegistry
	cmd          *cobra.Command
	viper        *viper.Viper
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
	registry := flags.NewFlagRegistry()

	// Identity flag with NoOptDefVal for interactive selection.
	registry.Register(&flags.StringFlag{
		Name:        cfg.IdentityFlagName,
		Shorthand:   "i",
		Default:     "",
		Description: "Identity to use for authentication (use without value to select interactively)",
		Required:    false,
		EnvVars:     []string{"ATMOS_IDENTITY", "IDENTITY"},
		NoOptDefVal: cfg.IdentityFlagSelectValue,
	})

	// Shell flag.
	registry.Register(&flags.StringFlag{
		Name:        "shell",
		Shorthand:   "s",
		Default:     "",
		Description: "Specify the shell to use (defaults to $SHELL, then bash, then sh)",
		Required:    false,
		EnvVars:     []string{"ATMOS_SHELL", "SHELL"},
	})

	return &AuthShellParser{
		flagRegistry: registry,
	}
}

// RegisterFlags adds auth shell flags to the Cobra command.
func (p *AuthShellParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.AuthShellParser.RegisterFlags")()

	p.cmd = cmd
	p.flagRegistry.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *AuthShellParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.AuthShellParser.BindToViper")()

	p.viper = v
	return p.flagRegistry.BindToViper(v)
}

// Parse parses command-line arguments and returns strongly-typed AuthShellOptions.
func (p *AuthShellParser) Parse(ctx context.Context, args []string) (*AuthShellOptions, error) {
	defer perf.Track(nil, "flagparser.AuthShellParser.Parse")()

	// Create an empty compatibility translator (auth shell doesn't need compatibility aliases).
	// All args after the flags are passed through to the shell.
	translator := flags.NewCompatibilityAliasTranslator(map[string]flags.CompatibilityAlias{})

	// Create AtmosFlagParser with translator.
	flagParser := flags.NewAtmosFlagParser(p.cmd, p.viper, translator)

	// Parse args using AtmosFlagParser.
	parsedConfig, err := flagParser.Parse(args)
	if err != nil {
		return nil, err
	}

	// Extract values from parsed flags.
	identityValue := flags.GetString(parsedConfig.Flags, cfg.IdentityFlagName)
	shellValue := flags.GetString(parsedConfig.Flags, "shell")

	// Build strongly-typed options.
	opts := &AuthShellOptions{
		GlobalFlags: flags.GlobalFlags{
			LogsLevel: flags.GetString(parsedConfig.Flags, "logs-level"),
			LogsFile:  flags.GetString(parsedConfig.Flags, "logs-file"),
			NoColor:   flags.GetBool(parsedConfig.Flags, "no-color"),
		},
		Identity:        flags.NewIdentitySelector(identityValue, true),
		Shell:           shellValue,
		PositionalArgs:  parsedConfig.PositionalArgs,
		SeparatedArgs: parsedConfig.SeparatedArgs,
	}

	return opts, nil
}
