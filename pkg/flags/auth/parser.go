package auth

import (
	"context"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/flags"
)

// AuthParser handles flag parsing for auth commands.
// Returns strongly-typed AuthOptions with all parsed flags.
//
// Auth commands include: auth console, auth exec, auth shell, auth validate, auth whoami, etc.
type AuthParser struct {
	parser *flags.StandardFlagParser
	cmd    *cobra.Command
	viper  *viper.Viper
}

// RegisterFlags adds auth flags to the Cobra command.
func (p *AuthParser) RegisterFlags(cmd *cobra.Command) {
	defer perf.Track(nil, "flags.AuthParser.RegisterFlags")()

	p.cmd = cmd
	p.parser.RegisterFlags(cmd)
}

// BindToViper binds flags to Viper for precedence handling.
func (p *AuthParser) BindToViper(v *viper.Viper) error {
	defer perf.Track(nil, "flags.AuthParser.BindToViper")()

	p.viper = v
	return p.parser.BindToViper(v)
}

// Parse processes command-line arguments and returns strongly-typed AuthOptions.
//
// This provides type-safe access to auth command flags:
//
//	// ✅ New way: Strong typing with compile-time safety
//	opts, err := parser.Parse(ctx, args)
//	if opts.Verbose {
//	    log.Info("Verbose mode enabled")
//	}
//	if opts.Output == "json" {
//	    printJSON()
//	}
func (p *AuthParser) Parse(ctx context.Context, args []string) (*AuthOptions, error) {
	defer perf.Track(nil, "flags.AuthParser.Parse")()

	// Use underlying parser to parse flags.
	parsedConfig, err := p.parser.Parse(ctx, args)
	if err != nil {
		return nil, err
	}

	// Convert to strongly-typed options.
	durationStr := flags.GetString(parsedConfig.Flags, "duration")

	// Check if duration flag was explicitly provided via CLI.
	// Using flag.Changed is more reliable than checking for non-empty string,
	// as it correctly handles --duration="" vs no flag at all.
	// Use ParsedFlags (the combined FlagSet from Parse) instead of p.cmd.Flags()
	// to correctly detect changes when DisableFlagParsing is enabled.
	durationProvided := false
	if parsedFlags := p.parser.ParsedFlags(); parsedFlags != nil {
		if flag := parsedFlags.Lookup("duration"); flag != nil {
			durationProvided = flag.Changed
		}
	}

	opts := AuthOptions{
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
		Verbose:          flags.GetBool(parsedConfig.Flags, "verbose"),
		Output:           flags.GetString(parsedConfig.Flags, "output"),
		Destination:      flags.GetString(parsedConfig.Flags, "destination"),
		Duration:         parseDuration(durationStr),
		DurationProvided: durationProvided,
		Issuer:           flags.GetString(parsedConfig.Flags, "issuer"),
		PrintOnly:        flags.GetBool(parsedConfig.Flags, "print-only"),
		NoOpen:           flags.GetBool(parsedConfig.Flags, "no-open"),
	}

	return &opts, nil
}

// parseDuration parses a duration string, returning 0 on error.
func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}
