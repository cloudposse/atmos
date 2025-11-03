package flagparser

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// HelmfileParser handles flag parsing for Helmfile commands.
// Returns strongly-typed HelmfileInterpreter.
type HelmfileParser struct {
	parser *PassThroughFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewHelmfileParser creates a parser for Helmfile commands.
func NewHelmfileParser() *HelmfileParser {
	defer perf.Track(nil, "flagparser.NewHelmfileParser")()

	return &HelmfileParser{
		parser: NewPassThroughFlagParser(WithHelmfileFlags()),
	}
}

// RegisterFlags adds Helmfile flags to the Cobra command.
func (p *HelmfileParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.HelmfileParser.RegisterFlags")()

	p.cmd = cmd
	// Helmfile passes subcommand separately to helmfileRun, so only extract 1 positional arg (component).
	p.parser.SetPositionalArgsCount(1)
	p.parser.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *HelmfileParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.HelmfileParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// Parse processes command-line arguments and returns strongly-typed HelmfileInterpreter.
func (p *HelmfileParser) Parse(ctx context.Context, args []string) (*HelmfileInterpreter, error) {
	defer perf.Track(nil, "flagparser.HelmfileParser.Parse")()

	// Use underlying parser to extract Atmos flags and separate pass-through args.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Convert to strongly-typed interpreter.
	interpreter := HelmfileInterpreter{
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
		Stack:           getString(parsedConfig.AtmosFlags, "stack"),
		Identity:        getIdentitySelector(parsedConfig.AtmosFlags, "identity"),
		DryRun:          getBool(parsedConfig.AtmosFlags, "dry-run"),
		positionalArgs:  parsedConfig.PositionalArgs,
		passThroughArgs: parsedConfig.PassThroughArgs,
	}

	return &interpreter, nil
}
