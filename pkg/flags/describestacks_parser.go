package flags

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// DescribeStacksParser parses flags for describe stacks command.
type DescribeStacksParser struct {
	parser *StandardFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewDescribeStacksParser creates a new parser for describe stacks command flags.
func NewDescribeStacksParser() *DescribeStacksParser {
	return NewDescribeStacksOptionsBuilder().
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
func (p *DescribeStacksParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flags.DescribeStacksParser.RegisterFlags")()

	p.cmd = cmd
	p.parser.RegisterFlags(cmd)
}

// BindToViper binds all flags to viper for environment variable and config file support.
func (p *DescribeStacksParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flags.DescribeStacksParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// Parse parses the flags and returns strongly-typed options.
func (p *DescribeStacksParser) Parse(ctx context.Context, args []string) (*DescribeStacksOptions, error) {
	defer perf.Track(nil, "flags.DescribeStacksParser.Parse")()

	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	return &DescribeStacksOptions{
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
		Stack:              getString(parsedConfig.Flags, "stack"),
		Format:             getString(parsedConfig.Flags, "format"),
		File:               getString(parsedConfig.Flags, "file"),
		ProcessTemplates:   getBool(parsedConfig.Flags, "process-templates"),
		ProcessFunctions:   getBool(parsedConfig.Flags, "process-functions"),
		Components:         getStringSlice(parsedConfig.Flags, "components"),
		ComponentTypes:     getStringSlice(parsedConfig.Flags, "component-types"),
		Sections:           getStringSlice(parsedConfig.Flags, "sections"),
		IncludeEmptyStacks: getBool(parsedConfig.Flags, "include-empty-stacks"),
		Skip:               getStringSlice(parsedConfig.Flags, "skip"),
		Query:              getString(parsedConfig.Flags, "query"),
	}, nil
}
