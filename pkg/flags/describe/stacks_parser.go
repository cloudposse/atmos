package describe

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// StacksParser parses flags for describe stacks command.
type StacksParser struct {
	Parser *flags.StandardFlagParser // Exported for builder in parent flags package
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewStacksParser creates a new parser for describe stacks command flags.
func NewStacksParser() *StacksParser {
	defer perf.Track(nil, "flags.NewStacksParser")()

	return NewStacksBuilder().
		WithStack().
		WithFormat().
		WithFile().
		WithProcessTemplates().
		WithProcessFunctions().
		WithComponents().
		WithComponentTypes().
		WithSections().
		WithIncludeEmptyStacks().
		WithSkip().
		WithQuery().
		Build()
}

// RegisterFlags registers all flags with the cobra command.
func (p *StacksParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flags.StacksParser.RegisterFlags")()

	p.cmd = cmd
	p.Parser.RegisterFlags(cmd)
}

// BindToViper binds all flags to viper for environment variable and config file support.
func (p *StacksParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flags.StacksParser.BindToViper")()

	p.viper = v
	return p.Parser.BindToViper(v)
}

// Parse parses the flags and returns strongly-typed options.
func (p *StacksParser) Parse(ctx context.Context, args []string) (*StacksOptions, error) {
	defer perf.Track(nil, "flags.StacksParser.Parse")()

	parsedConfig, err := p.Parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	return &StacksOptions{
		GlobalFlags: flags.GlobalFlags{
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
		Stack:              flags.GetString(parsedConfig.Flags, "stack"),
		Format:             flags.GetString(parsedConfig.Flags, "format"),
		File:               flags.GetString(parsedConfig.Flags, "file"),
		ProcessTemplates:   flags.GetBool(parsedConfig.Flags, "process-templates"),
		ProcessFunctions:   flags.GetBool(parsedConfig.Flags, "process-functions"),
		Components:         flags.GetStringSlice(parsedConfig.Flags, "components"),
		ComponentTypes:     flags.GetStringSlice(parsedConfig.Flags, "component-types"),
		Sections:           flags.GetStringSlice(parsedConfig.Flags, "sections"),
		IncludeEmptyStacks: flags.GetBool(parsedConfig.Flags, "include-empty-stacks"),
		Skip:               flags.GetStringSlice(parsedConfig.Flags, "skip"),
		Query:              flags.GetString(parsedConfig.Flags, "query"),
	}, nil
}
