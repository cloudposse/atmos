package flags

import (
	"context"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// AuthParser handles flag parsing for auth commands.
// Returns strongly-typed AuthOptions with all parsed flags.
//
// Auth commands include: auth console, auth exec, auth shell, auth validate, auth whoami, etc.
type AuthParser struct {
	parser *StandardFlagParser
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
	durationStr := getString(parsedConfig.Flags, "duration")

	// Check if duration flag was explicitly provided via CLI.
	// Using flag.Changed is more reliable than checking for non-empty string,
	// as it correctly handles --duration="" vs no flag at all.
	// Use parsedFlags (the combined FlagSet from Parse) instead of p.cmd.Flags()
	// to correctly detect changes when DisableFlagParsing is enabled.
	durationProvided := false
	if p.parser.parsedFlags != nil {
		if flag := p.parser.parsedFlags.Lookup("duration"); flag != nil {
			durationProvided = flag.Changed
		}
	}

	opts := AuthOptions{
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
		Verbose:          getBool(parsedConfig.Flags, "verbose"),
		Output:           getString(parsedConfig.Flags, "output"),
		Destination:      getString(parsedConfig.Flags, "destination"),
		Duration:         parseDuration(durationStr),
		DurationProvided: durationProvided,
		Issuer:           getString(parsedConfig.Flags, "issuer"),
		PrintOnly:        getBool(parsedConfig.Flags, "print-only"),
		NoOpen:           getBool(parsedConfig.Flags, "no-open"),
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
