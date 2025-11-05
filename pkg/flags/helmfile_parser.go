package flags

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// HelmfileParser handles flag parsing for Helmfile commands.
// Returns strongly-typed HelmfileOptions.
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
	// https://github.com/spf13/cobra/issues/739
	// DisableFlagParsing=true prevents Cobra from parsing flags, but flags can still be registered.
	// Our manual parsers extract flag values from os.Args directly.
	cmd.DisableFlagParsing = true
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

// Parse processes command-line arguments and returns strongly-typed HelmfileOptions.
//
//nolint:dupl // Similar to PackerParser.Parse but returns different types
func (p *HelmfileParser) Parse(ctx context.Context, args []string) (*HelmfileOptions, error) {
	defer perf.Track(nil, "flagparser.HelmfileParser.Parse")()

	// Use underlying parser to extract Atmos flags and separate pass-through args.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Extract component from positional args using helper.
	// Helmfile commands: atmos helmfile <subcommand> <component>
	// positionalArgs[0] = subcommand (sync, apply, diff, etc.)
	// positionalArgs[1] = component name (nginx, redis, etc.)
	component := extractComponent(parsedConfig.PositionalArgs, 1)

	// Convert to strongly-typed interpreter.
	opts := HelmfileOptions{
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
		DryRun:          getBool(parsedConfig.Flags, "dry-run"),
		Component:       component,
		positionalArgs:  parsedConfig.PositionalArgs,
		passThroughArgs: parsedConfig.PassThroughArgs,
	}

	return &opts, nil
}
