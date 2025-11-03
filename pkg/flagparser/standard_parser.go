package flagparser

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// StandardParser handles flag parsing for standard Atmos commands (non-pass-through).
// Returns strongly-typed StandardInterpreter with all parsed flags.
//
// Standard commands include: describe, list, validate, vendor, workflow, aws, pro, etc.
// These commands don't pass arguments through to external tools.
type StandardParser struct {
	parser *StandardFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewStandardParser creates a parser for standard commands with specified flags.
// Use existing Option functions (WithStringFlag, WithBoolFlag, etc.) to add command-specific flags.
//
// Example:
//
//	parser := NewStandardParser(
//	    WithStringFlag("stack", "s", "", "Stack name"),
//	    WithStringFlag("format", "f", "yaml", "Output format"),
//	    WithBoolFlag("dry-run", "", false, "Dry run mode"),
//	)
func NewStandardParser(opts ...Option) *StandardParser {
	defer perf.Track(nil, "flagparser.NewStandardParser")()

	return &StandardParser{
		parser: NewStandardFlagParser(opts...),
	}
}

// RegisterFlags adds flags to the Cobra command.
func (p *StandardParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flagparser.StandardParser.RegisterFlags")()

	p.cmd = cmd
	p.parser.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *StandardParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flagparser.StandardParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// Parse processes command-line arguments and returns strongly-typed StandardInterpreter.
//
// Handles precedence (CLI > ENV > config > defaults) via Viper.
// Extracts positional arguments (e.g., component name).
func (p *StandardParser) Parse(ctx context.Context, args []string) (*StandardInterpreter, error) {
	defer perf.Track(nil, "flagparser.StandardParser.Parse")()

	// Use underlying parser to parse flags and extract positional args.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Convert to strongly-typed interpreter.
	interpreter := StandardInterpreter{
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
		Stack:                getString(parsedConfig.AtmosFlags, "stack"),
		Component:            getString(parsedConfig.AtmosFlags, "component"),
		Format:               getString(parsedConfig.AtmosFlags, "format"),
		File:                 getString(parsedConfig.AtmosFlags, "file"),
		ProcessTemplates:     getBool(parsedConfig.AtmosFlags, "process-templates"),
		ProcessYamlFunctions: getBool(parsedConfig.AtmosFlags, "process-functions"),
		Skip:                 getStringSlice(parsedConfig.AtmosFlags, "skip"),
		DryRun:               getBool(parsedConfig.AtmosFlags, "dry-run"),
		Query:                getString(parsedConfig.AtmosFlags, "query"),
		Provenance:           getBool(parsedConfig.AtmosFlags, "provenance"),
		positionalArgs:       parsedConfig.PositionalArgs,
	}

	return &interpreter, nil
}
