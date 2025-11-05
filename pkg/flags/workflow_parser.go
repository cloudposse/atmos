package flags

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// WorkflowParser handles flag parsing for workflow commands.
// Returns strongly-typed WorkflowOptions with all parsed flags.
type WorkflowParser struct {
	parser *StandardFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewWorkflowParser creates a parser for workflow commands with specified flags.
// Use existing Option functions (WithStringFlag, WithBoolFlag, etc.) to add workflow-specific flags.
//
// Example:
//
//	parser := NewWorkflowParser(
//	    WithStringFlag("file", "f", "", "Workflow file"),
//	    WithBoolFlag("dry-run", "", false, "Dry run mode"),
//	    WithStringFlag("from-step", "", "", "Resume from step"),
//	)
func NewWorkflowParser(opts ...Option) *WorkflowParser {
	defer perf.Track(nil, "flags.NewWorkflowParser")()

	return &WorkflowParser{
		parser: NewStandardFlagParser(opts...),
	}
}

// RegisterFlags adds flags to the Cobra command.
func (p *WorkflowParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flags.WorkflowParser.RegisterFlags")()

	p.cmd = cmd
	p.parser.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *WorkflowParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flags.WorkflowParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// BindFlagsToViper binds Cobra flags to Viper for precedence handling.
func (p *WorkflowParser) BindFlagsToViper(cmd *cobra.Command, v *viper.Viper) error {
	defer perf.Track(nil, "flags.WorkflowParser.BindFlagsToViper")()

	return p.parser.BindFlagsToViper(cmd, v)
}

// Parse processes command-line arguments and returns strongly-typed WorkflowOptions.
//
// Handles precedence (CLI > ENV > config > defaults) via Viper.
// Extracts positional arguments (e.g., workflow name) and populates WorkflowName field.
func (p *WorkflowParser) Parse(ctx context.Context, args []string) (*WorkflowOptions, error) {
	defer perf.Track(nil, "flags.WorkflowParser.Parse")()

	// Use underlying parser to parse flags and extract positional args.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Extract workflow name from positional args
	// Workflow command: atmos workflow <workflow-name>
	// positionalArgs[0] = workflow name (e.g., "deploy", "test")
	workflowName := ""
	if len(parsedConfig.PositionalArgs) >= 1 {
		workflowName = parsedConfig.PositionalArgs[0]
	}

	// Convert to strongly-typed options.
	options := WorkflowOptions{
		StandardOptions: StandardOptions{
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
			Stack:                getString(parsedConfig.Flags, "stack"),
			Component:            getString(parsedConfig.Flags, "component"),
			Format:               getString(parsedConfig.Flags, "format"),
			File:                 getString(parsedConfig.Flags, "file"),
			ProcessTemplates:     getBool(parsedConfig.Flags, "process-templates"),
			ProcessYamlFunctions: getBool(parsedConfig.Flags, "process-functions"),
			Skip:                 getStringSlice(parsedConfig.Flags, "skip"),
			DryRun:               getBool(parsedConfig.Flags, "dry-run"),
			Query:                getString(parsedConfig.Flags, "query"),
			Provenance:           getBool(parsedConfig.Flags, "provenance"),
			positionalArgs:       parsedConfig.PositionalArgs,
		},
		WorkflowName: workflowName,
		FromStep:     getString(parsedConfig.Flags, "from-step"),
	}

	return &options, nil
}
