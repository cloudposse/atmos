package flags

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// EditorConfigParser handles flag parsing for validate editorconfig command.
// Returns strongly-typed EditorConfigOptions with all parsed flags.
type EditorConfigParser struct {
	parser *StandardFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// RegisterFlags adds editorconfig flags to the Cobra command.
func (p *EditorConfigParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flags.EditorConfigParser.RegisterFlags")()

	p.cmd = cmd
	p.parser.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *EditorConfigParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flags.EditorConfigParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// Parse processes command-line arguments and returns strongly-typed EditorConfigOptions.
//
// This provides type-safe access to editorconfig command flags:
//
//	opts, err := parser.Parse(ctx, args)
//	if opts.Init {
//	    // Create initial configuration
//	}
//	if opts.DryRun {
//	    // Show files that would be checked
//	}
func (p *EditorConfigParser) Parse(ctx context.Context, args []string) (*EditorConfigOptions, error) {
	defer perf.Track(nil, "flags.EditorConfigParser.Parse")()

	// Use underlying parser to parse flags.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Convert to strongly-typed options.
	opts := EditorConfigOptions{
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
		Exclude:                       getString(parsedConfig.Flags, "exclude"),
		Init:                          getBool(parsedConfig.Flags, "init"),
		IgnoreDefaults:                getBool(parsedConfig.Flags, "ignore-defaults"),
		DryRun:                        getBool(parsedConfig.Flags, "dry-run"),
		ShowVersion:                   getBool(parsedConfig.Flags, "version"),
		Format:                        getString(parsedConfig.Flags, "format"),
		DisableTrimTrailingWhitespace: getBool(parsedConfig.Flags, "disable-trim-trailing-whitespace"),
		DisableEndOfLine:              getBool(parsedConfig.Flags, "disable-end-of-line"),
		DisableInsertFinalNewline:     getBool(parsedConfig.Flags, "disable-insert-final-newline"),
		DisableIndentation:            getBool(parsedConfig.Flags, "disable-indentation"),
		DisableIndentSize:             getBool(parsedConfig.Flags, "disable-indent-size"),
		DisableMaxLineLength:          getBool(parsedConfig.Flags, "disable-max-line-length"),
	}

	return &opts, nil
}
