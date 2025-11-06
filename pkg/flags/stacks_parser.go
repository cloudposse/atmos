package flags

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// DescribeStacksParser parses flags for describe stacks command.
type DescribeStacksParser struct {
	Parser *StandardFlagParser // Exported for builder in parent flags package
	cmd    *cobra.Command
	viper  *viper.Viper
}

// NewDescribeStacksParser creates a new parser for describe stacks command flags.
func NewDescribeStacksParser() *DescribeStacksParser {
	defer perf.Track(nil, "flags.NewDescribeStacksParser")()

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
	p.Parser.RegisterFlags(cmd)
}

// BindToViper binds all flags to viper for environment variable and config file support.
func (p *DescribeStacksParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flags.DescribeStacksParser.BindToViper")()

	p.viper = v
	return p.Parser.BindToViper(v)
}

// Parse parses the flags and returns strongly-typed options.
func (p *DescribeStacksParser) Parse(ctx context.Context, args []string) (*DescribeStacksOptions, error) {
	defer perf.Track(nil, "flags.DescribeStacksParser.Parse")()

	parsedConfig, err := p.Parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	return &DescribeStacksOptions{
		GlobalFlags: GlobalFlags{
			Chdir:           GetString(parsedConfig.Flags, "chdir"),
			BasePath:        GetString(parsedConfig.Flags, "base-path"),
			Config:          GetStringSlice(parsedConfig.Flags, "config"),
			ConfigPath:      GetStringSlice(parsedConfig.Flags, "config-path"),
			LogsLevel:       GetString(parsedConfig.Flags, "logs-level"),
			LogsFile:        GetString(parsedConfig.Flags, "logs-file"),
			NoColor:         GetBool(parsedConfig.Flags, "no-color"),
			Pager:           GetPagerSelector(parsedConfig.Flags, "pager"),
			Identity:        GetIdentitySelector(parsedConfig.Flags, "identity"),
			ProfilerEnabled: GetBool(parsedConfig.Flags, "profiler-enabled"),
			ProfilerPort:    GetInt(parsedConfig.Flags, "profiler-port"),
			ProfilerHost:    GetString(parsedConfig.Flags, "profiler-host"),
			ProfileFile:     GetString(parsedConfig.Flags, "profile-file"),
			ProfileType:     GetString(parsedConfig.Flags, "profile-type"),
			Heatmap:         GetBool(parsedConfig.Flags, "heatmap"),
			HeatmapMode:     GetString(parsedConfig.Flags, "heatmap-mode"),
			RedirectStderr:  GetString(parsedConfig.Flags, "redirect-stderr"),
			Version:         GetBool(parsedConfig.Flags, "version"),
		},
		Stack:              GetString(parsedConfig.Flags, "stack"),
		Format:             GetString(parsedConfig.Flags, "format"),
		File:               GetString(parsedConfig.Flags, "file"),
		ProcessTemplates:   GetBool(parsedConfig.Flags, "process-templates"),
		ProcessFunctions:   GetBool(parsedConfig.Flags, "process-functions"),
		Components:         GetStringSlice(parsedConfig.Flags, "components"),
		ComponentTypes:     GetStringSlice(parsedConfig.Flags, "component-types"),
		Sections:           GetStringSlice(parsedConfig.Flags, "sections"),
		IncludeEmptyStacks: GetBool(parsedConfig.Flags, "include-empty-stacks"),
		Skip:               GetStringSlice(parsedConfig.Flags, "skip"),
		Query:              GetString(parsedConfig.Flags, "query"),
	}, nil
}
