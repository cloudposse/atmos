package flagparser

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// AuthParser handles flag parsing for auth commands.
// Returns strongly-typed AuthInterpreter.
type AuthParser struct {
	parser *PassThroughFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewAuthExecParser creates a parser for auth exec command.
// Only includes identity flag - all other args are command args.
func NewAuthExecParser() *AuthParser {
	defer perf.Track(nil, "flagparser.NewAuthExecParser")()

	return &AuthParser{
		parser: NewPassThroughFlagParser(
			WithRegistry(func() *FlagRegistry {
				registry := NewFlagRegistry()
				// Identity flag with interactive selection support.
				identityFlag := GlobalFlagsRegistry().Get("identity")
				registry.Register(identityFlag)
				return registry
			}()),
		),
	}
}

// NewAuthShellParser creates a parser for auth shell command.
// Includes identity and shell flags - all other args are shell args.
func NewAuthShellParser() *AuthParser {
	defer perf.Track(nil, "flagparser.NewAuthShellParser")()

	return &AuthParser{
		parser: NewPassThroughFlagParser(
			WithRegistry(func() *FlagRegistry {
				registry := NewFlagRegistry()
				// Identity flag with interactive selection support.
				identityFlag := GlobalFlagsRegistry().Get("identity")
				registry.Register(identityFlag)
				// Shell flag.
				registry.Register(&StringFlag{
					Name:        "shell",
					Shorthand:   "s",
					Default:     "",
					Description: "Specify the shell to launch (default: $SHELL or /bin/sh)",
					EnvVars:     []string{"ATMOS_SHELL", "SHELL"},
				})
				return registry
			}()),
		),
	}
}

// RegisterFlags adds auth flags to the Cobra command.
func (p *AuthParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.AuthParser.RegisterFlags")()

	p.cmd = cmd
	// Auth commands don't extract positional args - all args after flags are pass-through.
	p.parser.DisablePositionalExtraction()
	p.parser.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *AuthParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.AuthParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// GetRegistry returns the underlying flag registry for advanced customization.
func (p *AuthParser) GetRegistry() *FlagRegistry {
	defer perf.Track(nil, "flagparser.AuthParser.GetRegistry")()

	return p.parser.GetRegistry()
}

// Parse processes command-line arguments and returns strongly-typed AuthInterpreter.
func (p *AuthParser) Parse(ctx context.Context, args []string) (*AuthInterpreter, error) {
	defer perf.Track(nil, "flagparser.AuthParser.Parse")()

	// Use underlying parser to extract Atmos flags and separate pass-through args.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Convert to strongly-typed interpreter.
	interpreter := AuthInterpreter{
		GlobalFlags: GlobalFlags{
			Chdir:           getString(parsedConfig.AtmosFlags, "chdir"),
			BasePath:        getString(parsedConfig.AtmosFlags, "base-path"),
			Config:          getStringSlice(parsedConfig.AtmosFlags, "config"),
			ConfigPath:      getStringSlice(parsedConfig.AtmosFlags, "config-path"),
			LogsLevel:       getString(parsedConfig.AtmosFlags, "logs-level"),
			LogsFile:        getString(parsedConfig.AtmosFlags, "logs-file"),
			NoColor:         getBool(parsedConfig.AtmosFlags, "no-color"),
			Pager:           getPagerSelector(parsedConfig.AtmosFlags, "pager"),
			Identity:        getIdentitySelector(parsedConfig.AtmosFlags, "identity"),
			ProfilerEnabled: getBool(parsedConfig.AtmosFlags, "profiler-enabled"),
			ProfilerPort:    getInt(parsedConfig.AtmosFlags, "profiler-port"),
			ProfilerHost:    getString(parsedConfig.AtmosFlags, "profiler-host"),
			ProfileFile:     getString(parsedConfig.AtmosFlags, "profile-file"),
			ProfileType:     getString(parsedConfig.AtmosFlags, "profile-type"),
			Heatmap:         getBool(parsedConfig.AtmosFlags, "heatmap"),
			HeatmapMode:     getString(parsedConfig.AtmosFlags, "heatmap-mode"),
			RedirectStderr:  getString(parsedConfig.AtmosFlags, "redirect-stderr"),
			Version:         getBool(parsedConfig.AtmosFlags, "version"),
		},
		Identity:        getIdentitySelector(parsedConfig.AtmosFlags, "identity"),
		Shell:           getString(parsedConfig.AtmosFlags, "shell"),
		positionalArgs:  parsedConfig.PositionalArgs,
		passThroughArgs: parsedConfig.PassThroughArgs,
	}

	return &interpreter, nil
}
