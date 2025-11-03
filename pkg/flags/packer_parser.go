package flags

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// PackerParser handles flag parsing for Packer commands.
// Returns strongly-typed PackerOptions.
type PackerParser struct {
	parser *PassThroughFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewPackerParser creates a parser for Packer commands.
func NewPackerParser() *PackerParser {
	defer perf.Track(nil, "flagparser.NewPackerParser")()

	return &PackerParser{
		parser: NewPassThroughFlagParser(WithPackerFlags()),
	}
}

// RegisterFlags adds Packer flags to the Cobra command.
func (p *PackerParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.PackerParser.RegisterFlags")()

	p.cmd = cmd
	// Packer passes subcommand separately to packerRun, so only extract 1 positional arg (component).
	p.parser.SetPositionalArgsCount(1)
	p.parser.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *PackerParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.PackerParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// Parse processes command-line arguments and returns strongly-typed PackerOptions.
//
//nolint:dupl // Similar to HelmfileParser.Parse but returns different types
func (p *PackerParser) Parse(ctx context.Context, args []string) (*PackerOptions, error) {
	defer perf.Track(nil, "flagparser.PackerParser.Parse")()

	// Use underlying parser to extract Atmos flags and separate pass-through args.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Convert to strongly-typed interpreter.
	opts := PackerOptions{
		GlobalFlags: GlobalFlags{
			Chdir:           getString(parsedConfig.Flags, "chdir"),
			BasePath:        getString(parsedConfig.Flags, "base-path"),
			Config:          getStringSlice(parsedConfig.Flags, "config"),
			ConfigPath:      getStringSlice(parsedConfig.Flags, "config-path"),
			LogsLevel:       getString(parsedConfig.Flags, "logs-level"),
			LogsFile:        getString(parsedConfig.Flags, "logs-file"),
			NoColor:         getBool(parsedConfig.Flags, "no-color"),
			Pager:           getPagerSelector(parsedConfig.Flags, "pager"),
			Identity:        getIdentitySelector(parsedConfig.Flags, "identity"),
			ProfilerEnabled: getBool(parsedConfig.Flags, "profiler-enabled"),
			ProfilerPort:    getInt(parsedConfig.Flags, "profiler-port"),
			ProfilerHost:    getString(parsedConfig.Flags, "profiler-host"),
			ProfileFile:     getString(parsedConfig.Flags, "profile-file"),
			ProfileType:     getString(parsedConfig.Flags, "profile-type"),
			Heatmap:         getBool(parsedConfig.Flags, "heatmap"),
			HeatmapMode:     getString(parsedConfig.Flags, "heatmap-mode"),
			RedirectStderr:  getString(parsedConfig.Flags, "redirect-stderr"),
			Version:         getBool(parsedConfig.Flags, "version"),
		},
		Stack:           getString(parsedConfig.Flags, "stack"),
		Identity:        getIdentitySelector(parsedConfig.Flags, "identity"),
		DryRun:          getBool(parsedConfig.Flags, "dry-run"),
		positionalArgs:  parsedConfig.PositionalArgs,
		passThroughArgs: parsedConfig.PassThroughArgs,
	}

	return &opts, nil
}

// RegisterPersistentFlags adds Packer flags to the Cobra command as persistent flags (inherited by subcommands).
func (p *PackerParser) RegisterPersistentFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.PackerParser.RegisterPersistentFlags")()

	p.cmd = cmd
	// Packer passes subcommand separately to packerRun, so only extract 1 positional arg (component).
	p.parser.SetPositionalArgsCount(1)
	p.parser.RegisterPersistentFlags(cmd)
}

// BindFlagsToViper binds Cobra flags to Viper for CLI flag precedence.
func (p *PackerParser) BindFlagsToViper(cmd *cobra.Command, v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.PackerParser.BindFlagsToViper")()

	return p.parser.BindFlagsToViper(cmd, v)
}
