package flags

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// IdentityFlagName is the flag name for the identity selector.
	identityFlagName = "identity"

	// PagerFlagName is the flag name for the pager selector.
	pagerFlagName = "pager"
)

// ParseGlobalFlags extracts all global flags from Viper with proper precedence.
// This should be called by parsers to populate the Flags struct in interpreters.
//
// Precedence order (handled automatically by Viper):
//  1. CLI flag value
//  2. Environment variable
//  3. Config file value
//  4. Flag default
//  5. Go zero value
//
// Special handling:
//   - identity and pager flags use NoOptDefVal pattern (require cmd.Flags().Changed check)
//   - All other flags use standard Viper resolution
func ParseGlobalFlags(cmd *cobra.Command, v *viper.Viper) global.Flags {
	defer perf.Track(nil, "flags.ParseGlobalFlags")()

	return global.Flags{
		// Working directory and path configuration.
		Chdir:      v.GetString("chdir"),
		BasePath:   v.GetString("base-path"),
		Config:     v.GetStringSlice("config"),
		ConfigPath: v.GetStringSlice("config-path"),

		// Logging configuration.
		LogsLevel: v.GetString("logs-level"),
		LogsFile:  v.GetString("logs-file"),
		NoColor:   v.GetBool("no-color"),

		// Terminal and I/O configuration.
		ForceColor: v.GetBool("force-color"),
		ForceTTY:   v.GetBool("force-tty"),
		Mask:       v.GetBool("mask"),

		// Output configuration.
		Pager: parsePagerFlag(cmd, v),

		// Authentication.
		Identity: parseIdentityFlag(cmd, v),

		// Profiles.
		Profile: v.GetStringSlice("profile"),

		// Profiling configuration.
		ProfilerEnabled: v.GetBool("profiler-enabled"),
		ProfilerPort:    v.GetInt("profiler-port"),
		ProfilerHost:    v.GetString("profiler-host"),
		ProfileFile:     v.GetString("profile-file"),
		ProfileType:     v.GetString("profile-type"),

		// Performance visualization.
		Heatmap:     v.GetBool("heatmap"),
		HeatmapMode: v.GetString("heatmap-mode"),

		// System configuration.
		RedirectStderr: v.GetString("redirect-stderr"),
		Version:        v.GetBool("version"),
	}
}

// BuildConfigAndStacksInfo parses global flags and builds ConfigAndStacksInfo.
// This ensures commands honor global flags like --base-path, --config, --config-path, and --profile.
// This is a convenience wrapper that extracts global flags and populates ConfigAndStacksInfo in one step.
func BuildConfigAndStacksInfo(cmd *cobra.Command, v *viper.Viper) schema.ConfigAndStacksInfo {
	defer perf.Track(nil, "flags.BuildConfigAndStacksInfo")()

	globalFlags := ParseGlobalFlags(cmd, v)
	return schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}
}

// parseIdentityFlag handles the identity flag's NoOptDefVal pattern.
// The identity flag has three states:
//  1. Not provided → IdentitySelector{provided: false}
//  2. --identity (alone) → IdentitySelector{value: "__SELECT__", provided: true}
//  3. --identity=value → IdentitySelector{value: "value", provided: true}
//
// Values like "false", "0", "no", "off" are normalized to the disabled sentinel
// value to allow users to disable authentication via --identity=false or ATMOS_IDENTITY=false.
func parseIdentityFlag(cmd *cobra.Command, v *viper.Viper) global.IdentitySelector {
	defer perf.Track(nil, "flags.parseIdentityFlag")()

	// Check local flags, inherited flags, and persistent flags.
	// The identity flag is registered as a persistent flag on RootCmd.
	// - On RootCmd: appears in PersistentFlags()
	// - On subcommands: appears in InheritedFlags() (inherited from RootCmd)
	flag := cmd.Flags().Lookup(identityFlagName)
	if flag == nil {
		flag = cmd.InheritedFlags().Lookup(identityFlagName)
	}
	if flag == nil {
		flag = cmd.PersistentFlags().Lookup(identityFlagName)
	}
	if flag == nil {
		// Identity flag not registered on this command or its parents.
		return global.NewIdentitySelector("", false)
	}

	// Check if flag was explicitly set on command line.
	// Check all flag sets because cmd.Flags().Changed() doesn't check persistent flags on root.
	changed := cmd.Flags().Changed(identityFlagName) ||
		cmd.InheritedFlags().Changed(identityFlagName) ||
		cmd.PersistentFlags().Changed(identityFlagName)

	if changed {
		value := v.GetString(identityFlagName)
		return global.NewIdentitySelector(normalizeIdentityValue(value), true)
	}

	// Fall back to env/config via Viper.
	if v.IsSet(identityFlagName) {
		value := v.GetString(identityFlagName)
		return global.NewIdentitySelector(normalizeIdentityValue(value), true)
	}

	return global.NewIdentitySelector("", false)
}

// normalizeIdentityValue converts boolean false representations to the disabled sentinel value.
// Recognizes: false, False, FALSE, 0, no, No, NO, off, Off, OFF.
// All other values are returned unchanged.
// This allows users to disable authentication via --identity=false or ATMOS_IDENTITY=false.
func normalizeIdentityValue(value string) string {
	if value == "" {
		return ""
	}

	switch strings.ToLower(value) {
	case "false", "0", "no", "off":
		return cfg.IdentityFlagDisabledValue
	default:
		return value
	}
}

// parsePagerFlag handles the pager flag's NoOptDefVal pattern.
// The pager flag has three states:
//  1. Not provided → PagerSelector{provided: false}
//  2. --pager (alone) → PagerSelector{value: "true", provided: true}
//  3. --pager=value → PagerSelector{value: "value", provided: true}
func parsePagerFlag(cmd *cobra.Command, v *viper.Viper) global.PagerSelector {
	defer perf.Track(nil, "flags.parsePagerFlag")()

	// Check local flags, inherited flags, and persistent flags.
	// The pager flag is registered as a persistent flag on RootCmd.
	// - On RootCmd: appears in PersistentFlags()
	// - On subcommands: appears in InheritedFlags() (inherited from RootCmd)
	flag := cmd.Flags().Lookup(pagerFlagName)
	if flag == nil {
		flag = cmd.InheritedFlags().Lookup(pagerFlagName)
	}
	if flag == nil {
		flag = cmd.PersistentFlags().Lookup(pagerFlagName)
	}
	if flag == nil {
		// Pager flag not registered on this command or its parents.
		return global.NewPagerSelector("", false)
	}

	// Check if flag was explicitly set on command line.
	// Check all flag sets because cmd.Flags().Changed() doesn't check persistent flags on root.
	changed := cmd.Flags().Changed(pagerFlagName) ||
		cmd.InheritedFlags().Changed(pagerFlagName) ||
		cmd.PersistentFlags().Changed(pagerFlagName)

	if changed {
		value := v.GetString(pagerFlagName)
		return global.NewPagerSelector(value, true)
	}

	// Check if value is set via environment variable or other Viper source.
	// We check v.IsSet() to catch env vars, but config values in atmos.yaml
	// are handled separately by the pager package.
	if v.IsSet(pagerFlagName) {
		value := v.GetString(pagerFlagName)
		if value != "" {
			return global.NewPagerSelector(value, true)
		}
	}

	// Pager flag not explicitly set - return as not provided.
	return global.NewPagerSelector("", false)
}

// GlobalFlagsRegistry returns a FlagRegistry with all global flags pre-configured.
// This can be used to register global flags on commands that don't inherit from RootCmd.
func GlobalFlagsRegistry() *FlagRegistry {
	defer perf.Track(nil, "flags.FlagsRegistry")()

	registry := NewFlagRegistry()

	// Register all flag categories.
	registerWorkingDirectoryFlags(registry)
	registerLoggingFlags(registry)
	registerTerminalFlags(registry)
	registerAuthenticationFlags(registry)
	registerProfilingFlags(registry)
	registerPerformanceFlags(registry)

	return registry
}

// registerWorkingDirectoryFlags registers working directory and path flags.
func registerWorkingDirectoryFlags(registry *FlagRegistry) {
	defer perf.Track(nil, "flags.registerWorkingDirectoryFlags")()

	registry.Register(&StringFlag{
		Name:        "chdir",
		Shorthand:   "C",
		Default:     "",
		Description: "Change working directory before processing",
		EnvVars:     []string{"ATMOS_CHDIR"},
	})

	registry.Register(&StringFlag{
		Name:        "base-path",
		Shorthand:   "",
		Default:     "",
		Description: "Base path for Atmos project",
		EnvVars:     []string{"ATMOS_BASE_PATH"},
	})

	registry.Register(&StringSliceFlag{
		Name:        "config",
		Shorthand:   "",
		Default:     []string{},
		Description: "Paths to configuration files (comma-separated or repeated flag)",
		EnvVars:     []string{"ATMOS_CONFIG"},
	})

	registry.Register(&StringSliceFlag{
		Name:        "config-path",
		Shorthand:   "",
		Default:     []string{},
		Description: "Paths to configuration directories (comma-separated or repeated flag)",
		EnvVars:     []string{"ATMOS_CONFIG_PATH"},
	})
}

// registerLoggingFlags registers logging configuration flags.
func registerLoggingFlags(registry *FlagRegistry) {
	defer perf.Track(nil, "flags.registerLoggingFlags")()

	registry.Register(&StringFlag{
		Name:        "logs-level",
		Shorthand:   "",
		Default:     "Info",
		Description: "Logs level (Trace, Debug, Info, Warning, Off)",
		EnvVars:     []string{"ATMOS_LOGS_LEVEL"},
	})

	registry.Register(&StringFlag{
		Name:        "logs-file",
		Shorthand:   "",
		Default:     "/dev/stderr",
		Description: "File to write logs to",
		EnvVars:     []string{"ATMOS_LOGS_FILE"},
	})

	registry.Register(&BoolFlag{
		Name:        "no-color",
		Shorthand:   "",
		Default:     false,
		Description: "Disable color output",
		EnvVars:     []string{"ATMOS_NO_COLOR", "NO_COLOR"},
	})
}

// registerAuthenticationFlags registers authentication and output flags.
func registerAuthenticationFlags(registry *FlagRegistry) {
	defer perf.Track(nil, "flags.registerAuthenticationFlags")()

	// Identity flag with NoOptDefVal for interactive selection.
	// NoOptDefVal enables the pattern: --identity (interactive), --identity value (explicit).
	registry.Register(&StringFlag{
		Name:        identityFlagName,
		Shorthand:   "i",
		Default:     "",
		Description: "Identity to use for authentication (use without value to select interactively)",
		EnvVars:     []string{"ATMOS_IDENTITY", "IDENTITY"},
		NoOptDefVal: cfg.IdentityFlagSelectValue, // "__SELECT__"
	})

	// Pager flag (special NoOptDefVal handling).
	registry.Register(&StringFlag{
		Name:        pagerFlagName,
		Shorthand:   "",
		Default:     "",
		Description: "Enable pager for output",
		NoOptDefVal: "true",
		EnvVars:     []string{"ATMOS_PAGER"},
	})
}

// registerProfilingFlags registers profiling configuration flags.
func registerProfilingFlags(registry *FlagRegistry) {
	defer perf.Track(nil, "flags.registerProfilingFlags")()

	registry.Register(&BoolFlag{
		Name:        "profiler-enabled",
		Shorthand:   "",
		Default:     false,
		Description: "Enable pprof profiling server",
		EnvVars:     []string{"ATMOS_PROFILER_ENABLED"},
	})

	registry.Register(&IntFlag{
		Name:        "profiler-port",
		Shorthand:   "",
		Default:     6060,
		Description: "Port for pprof profiling server",
		EnvVars:     []string{"ATMOS_PROFILER_PORT"},
	})

	registry.Register(&StringFlag{
		Name:        "profiler-host",
		Shorthand:   "",
		Default:     "localhost",
		Description: "Host for pprof profiling server",
		EnvVars:     []string{"ATMOS_PROFILER_HOST"},
	})

	registry.Register(&StringFlag{
		Name:        "profile-file",
		Shorthand:   "",
		Default:     "",
		Description: "Write profiling data to file",
		EnvVars:     []string{"ATMOS_PROFILE_FILE"},
	})

	registry.Register(&StringFlag{
		Name:        "profile-type",
		Shorthand:   "",
		Default:     "cpu",
		Description: "Type of profile to collect (cpu, heap, allocs, goroutine, block, mutex, threadcreate, trace)",
		EnvVars:     []string{"ATMOS_PROFILE_TYPE"},
	})
}

// registerTerminalFlags registers terminal and I/O configuration flags.
func registerTerminalFlags(registry *FlagRegistry) {
	defer perf.Track(nil, "flags.registerTerminalFlags")()

	registry.Register(&BoolFlag{
		Name:        "force-color",
		Shorthand:   "",
		Default:     false,
		Description: "Force color output even when not a TTY (useful for screenshots)",
		EnvVars:     []string{"ATMOS_FORCE_COLOR", "CLICOLOR_FORCE"},
	})

	registry.Register(&BoolFlag{
		Name:        "force-tty",
		Shorthand:   "",
		Default:     false,
		Description: "Force TTY mode with sane defaults (width=120, height=40) when terminal detection fails",
		EnvVars:     []string{"ATMOS_FORCE_TTY"},
	})

	registry.Register(&BoolFlag{
		Name:        "mask",
		Shorthand:   "",
		Default:     true,
		Description: "Enable automatic masking of secrets in output",
		EnvVars:     []string{"ATMOS_MASK"},
	})

	registry.Register(&StringFlag{
		Name:        "redirect-stderr",
		Shorthand:   "",
		Default:     "",
		Description: "Redirect stderr to file",
		EnvVars:     []string{"ATMOS_REDIRECT_STDERR"},
	})
}

// registerPerformanceFlags registers performance visualization flags.
func registerPerformanceFlags(registry *FlagRegistry) {
	defer perf.Track(nil, "flags.registerPerformanceFlags")()

	registry.Register(&BoolFlag{
		Name:        "heatmap",
		Shorthand:   "",
		Default:     false,
		Description: "Show performance heatmap visualization",
		EnvVars:     []string{"ATMOS_HEATMAP"},
	})

	registry.Register(&StringFlag{
		Name:        "heatmap-mode",
		Shorthand:   "",
		Default:     "bar",
		Description: "Heatmap visualization mode (bar, sparkline, table)",
		EnvVars:     []string{"ATMOS_HEATMAP_MODE"},
	})
}
