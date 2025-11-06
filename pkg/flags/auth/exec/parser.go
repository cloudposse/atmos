package exec

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
)

// AuthExecParser handles flag parsing for auth exec and auth shell commands.
// Returns strongly-typed AuthExecOptions with identity flag and pass-through args.
type AuthExecParser struct {
	flagRegistry *flags.FlagRegistry
	cmd          *cobra.Command
	viper        *viper.Viper
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

	return &AuthExecParser{
		flagRegistry: registry,
	}
}

// RegisterFlags adds auth exec flags to the Cobra command.
func (p *AuthExecParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.AuthExecParser.RegisterFlags")()

	p.cmd = cmd
	p.flagRegistry.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *AuthExecParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.AuthExecParser.BindToViper")()

	p.viper = v
	return p.flagRegistry.BindToViper(v)
}

// Parse parses command-line arguments and returns strongly-typed AuthExecOptions.
func (p *AuthExecParser) Parse(ctx context.Context, args []string) (*AuthExecOptions, error) {
	defer perf.Track(nil, "flagparser.AuthExecParser.Parse")()

	// Create an empty compatibility translator (auth exec doesn't need compatibility aliases).
	// All args after the identity flag are passed through to the command.
	translator := flags.NewCompatibilityAliasTranslator(map[string]flags.CompatibilityAlias{})

	// Create AtmosFlagParser with translator.
	flagParser := flags.NewAtmosFlagParser(p.cmd, p.viper, translator)

	// Parse args using AtmosFlagParser.
	parsedConfig, err := flagParser.Parse(args)
	if err != nil {
		return nil, err
	}

	// Extract identity value from parsed flags.
	identityValue := flags.GetString(parsedConfig.Flags, cfg.IdentityFlagName)

	// Build strongly-typed options.
	opts := &AuthExecOptions{
		Flags: global.Flags{
			LogsLevel: flags.GetString(parsedConfig.Flags, "logs-level"),
			LogsFile:  flags.GetString(parsedConfig.Flags, "logs-file"),
			NoColor:   flags.GetBool(parsedConfig.Flags, "no-color"),
		},
		Identity:       NewIdentityFlag(identityValue),
		PositionalArgs: parsedConfig.PositionalArgs,
		SeparatedArgs:  parsedConfig.SeparatedArgs,
	}

	return opts, nil
}
