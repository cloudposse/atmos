package workflow

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
)

// WorkflowParser handles flag parsing for workflow commands.
// Returns strongly-typed WorkflowOptions with all parsed flags.
type WorkflowParser struct {
	parser *flags.StandardFlagParser
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
func NewWorkflowParser(opts ...flags.Option) *WorkflowParser {
	defer perf.Track(nil, "flags.NewWorkflowParser")()

	return &WorkflowParser{
		parser: flags.NewStandardFlagParser(opts...),
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
		StandardOptions: flags.StandardOptions{
			Flags: global.Flags{
				Chdir:           flags.GetString(parsedConfig.Flags, "chdir"),
				BasePath:        flags.GetString(parsedConfig.Flags, "base-path"),
				Config:          flags.GetStringSlice(parsedConfig.Flags, "config"),
				ConfigPath:      flags.GetStringSlice(parsedConfig.Flags, "config-path"),
				LogsLevel:       flags.GetString(parsedConfig.Flags, "logs-level"),
				LogsFile:        flags.GetString(parsedConfig.Flags, "logs-file"),
				NoColor:         flags.GetBool(parsedConfig.Flags, "no-color"),
				Pager:           flags.GetPagerSelector(parsedConfig.Flags, "pager"),
				Identity:        flags.GetIdentitySelector(parsedConfig.Flags, "identity"),
				ProfilerEnabled: flags.GetBool(parsedConfig.Flags, "profiler-enabled"),
				ProfilerPort:    flags.GetInt(parsedConfig.Flags, "profiler-port"),
				ProfilerHost:    flags.GetString(parsedConfig.Flags, "profiler-host"),
				ProfileFile:     flags.GetString(parsedConfig.Flags, "profile-file"),
				ProfileType:     flags.GetString(parsedConfig.Flags, "profile-type"),
				Heatmap:         flags.GetBool(parsedConfig.Flags, "heatmap"),
				HeatmapMode:     flags.GetString(parsedConfig.Flags, "heatmap-mode"),
				RedirectStderr:  flags.GetString(parsedConfig.Flags, "redirect-stderr"),
				Version:         flags.GetBool(parsedConfig.Flags, "version"),
			},
			Stack:                flags.GetString(parsedConfig.Flags, "stack"),
			Component:            flags.GetString(parsedConfig.Flags, "component"),
			Format:               flags.GetString(parsedConfig.Flags, "format"),
			File:                 flags.GetString(parsedConfig.Flags, "file"),
			ProcessTemplates:     flags.GetBool(parsedConfig.Flags, "process-templates"),
			ProcessYamlFunctions: flags.GetBool(parsedConfig.Flags, "process-functions"),
			Skip:                 flags.GetStringSlice(parsedConfig.Flags, "skip"),
			DryRun:               flags.GetBool(parsedConfig.Flags, "dry-run"),
			Query:                flags.GetString(parsedConfig.Flags, "query"),
			Provenance:           flags.GetBool(parsedConfig.Flags, "provenance"),
		},
		WorkflowName: workflowName,
		FromStep:     flags.GetString(parsedConfig.Flags, "from-step"),
	}

	// Set positional args on the embedded StandardOptions.
	options.StandardOptions.SetPositionalArgs(parsedConfig.PositionalArgs)

	return &options, nil
}
