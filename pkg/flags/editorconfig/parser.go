package editorconfig

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/flags"
)

// EditorConfigParser handles flag parsing for validate editorconfig command.
// Returns strongly-typed EditorConfigOptions with all parsed flags.
type EditorConfigParser struct {
	parser *flags.StandardFlagParser
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
		Exclude:                       flags.GetString(parsedConfig.Flags, "exclude"),
		Init:                          flags.GetBool(parsedConfig.Flags, "init"),
		IgnoreDefaults:                flags.GetBool(parsedConfig.Flags, "ignore-defaults"),
		DryRun:                        flags.GetBool(parsedConfig.Flags, "dry-run"),
		ShowVersion:                   flags.GetBool(parsedConfig.Flags, "show-version"),
		Format:                        flags.GetString(parsedConfig.Flags, "format"),
		DisableTrimTrailingWhitespace: flags.GetBool(parsedConfig.Flags, "disable-trim-trailing-whitespace"),
		DisableEndOfLine:              flags.GetBool(parsedConfig.Flags, "disable-end-of-line"),
		DisableInsertFinalNewline:     flags.GetBool(parsedConfig.Flags, "disable-insert-final-newline"),
		DisableIndentation:            flags.GetBool(parsedConfig.Flags, "disable-indentation"),
		DisableIndentSize:             flags.GetBool(parsedConfig.Flags, "disable-indent-size"),
		DisableMaxLineLength:          flags.GetBool(parsedConfig.Flags, "disable-max-line-length"),
	}

	return &opts, nil
}
