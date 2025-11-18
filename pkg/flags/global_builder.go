package flags

import (
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
)

// GlobalOptionsBuilder provides a type-safe, fluent interface for building a parser
// with global flag definitions. This is used by RootCmd to register all global flags
// with proper defaults and precedence handling.
//
// Example:
//
//	parser := flags.NewGlobalOptionsBuilder().Build()
//	parser.RegisterFlags(RootCmd)
//	if err := parser.BindToViper(viper.GetViper()); err != nil {
//	    log.Fatal("Failed to bind flags", "error", err)
//	}
type GlobalOptionsBuilder struct {
	*StandardOptionsBuilder
}

// NewGlobalOptionsBuilder creates a new builder for global flags.
// Returns a builder with all global flags pre-configured.
func NewGlobalOptionsBuilder() *GlobalOptionsBuilder {
	defer perf.Track(nil, "flags.NewGlobalOptionsBuilder")()

	// Get defaults from global.NewFlags() to avoid duplication.
	defaults := global.NewFlags()

	// Start with a standard builder.
	builder := &GlobalOptionsBuilder{
		StandardOptionsBuilder: NewStandardOptionsBuilder(),
	}

	// Register all flag categories.
	builder.registerWorkingDirectoryFlags(&defaults)
	builder.registerLoggingFlags(&defaults)
	builder.registerTerminalFlags(&defaults)
	builder.registerAuthenticationFlags(&defaults)
	builder.registerProfilingFlags(&defaults)
	builder.registerPerformanceFlags(&defaults)
	builder.registerSystemFlags(&defaults)

	return builder
}

// registerWorkingDirectoryFlags registers working directory and path configuration flags.
func (b *GlobalOptionsBuilder) registerWorkingDirectoryFlags(defaults *global.Flags) {
	defer perf.Track(nil, "flags.GlobalOptionsBuilder.registerWorkingDirectoryFlags")()

	b.options = append(b.options, WithStringFlag("chdir", "C", defaults.Chdir, "Change working directory before executing the command (run as if Atmos started in this directory)"))
	b.options = append(b.options, WithEnvVars("chdir", "ATMOS_CHDIR"))

	b.options = append(b.options, WithStringFlag("base-path", "", defaults.BasePath, "Base path for Atmos project"))
	b.options = append(b.options, WithEnvVars("base-path", "ATMOS_BASE_PATH"))

	// String slice flags use registry pattern.
	b.options = append(b.options, func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        "config",
			Shorthand:   "",
			Default:     defaults.Config,
			Description: "Paths to configuration files (comma-separated or repeated flag)",
			EnvVars:     []string{"ATMOS_CONFIG"},
		})
	})

	b.options = append(b.options, func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        "config-path",
			Shorthand:   "",
			Default:     defaults.ConfigPath,
			Description: "Paths to search for Atmos configuration (comma-separated or repeated flag)",
			EnvVars:     []string{"ATMOS_CONFIG_PATH"},
		})
	})
}

// registerLoggingFlags registers logging configuration flags.
func (b *GlobalOptionsBuilder) registerLoggingFlags(defaults *global.Flags) {
	defer perf.Track(nil, "flags.GlobalOptionsBuilder.registerLoggingFlags")()

	b.options = append(b.options, WithStringFlag("logs-level", "", defaults.LogsLevel, "Logs level. Supported log levels are Trace, Debug, Info, Warning, Off. If the log level is set to Off, Atmos will not log any messages"))
	b.options = append(b.options, WithEnvVars("logs-level", "ATMOS_LOGS_LEVEL"))

	b.options = append(b.options, WithStringFlag("logs-file", "", defaults.LogsFile, "The file to write Atmos logs to. Logs can be written to any file or any standard file descriptor, including '/dev/stdout', '/dev/stderr' and '/dev/null'"))
	b.options = append(b.options, WithEnvVars("logs-file", "ATMOS_LOGS_FILE"))

	b.options = append(b.options, WithBoolFlag("no-color", "", defaults.NoColor, "Disable color output"))
	b.options = append(b.options, WithEnvVars("no-color", "ATMOS_NO_COLOR", "NO_COLOR", "CLICOLOR"))
}

// registerTerminalFlags registers terminal and I/O configuration flags.
func (b *GlobalOptionsBuilder) registerTerminalFlags(defaults *global.Flags) {
	defer perf.Track(nil, "flags.GlobalOptionsBuilder.registerTerminalFlags")()

	// Terminal and I/O configuration - use existing builder methods!
	b.WithForceColor()
	b.WithForceTTY()
	b.WithMask()

	// Interactive prompts configuration.
	b.options = append(b.options, WithBoolFlag("interactive", "", defaults.Interactive, "Enable interactive prompts for missing required flags (requires TTY, disabled in CI)"))
	b.options = append(b.options, WithEnvVars("interactive", "ATMOS_INTERACTIVE"))

	// Output configuration - pager with NoOptDefVal.
	b.options = append(b.options, WithStringFlag("pager", "", defaults.Pager.Value(), "Enable pager for output (--pager or --pager=true to enable, --pager=false to disable, --pager=less to use specific pager)"))
	b.options = append(b.options, WithEnvVars("pager", "ATMOS_PAGER", "PAGER"))
	b.options = append(b.options, WithNoOptDefVal("pager", "true"))
}

// registerAuthenticationFlags registers authentication flags.
func (b *GlobalOptionsBuilder) registerAuthenticationFlags(defaults *global.Flags) {
	defer perf.Track(nil, "flags.GlobalOptionsBuilder.registerAuthenticationFlags")()

	// Authentication - identity with NoOptDefVal.
	b.options = append(b.options, WithStringFlag("identity", "", defaults.Identity.Value(), "Identity to use for authentication. Use --identity to select interactively, --identity=NAME to specify"))
	b.options = append(b.options, WithEnvVars("identity", "ATMOS_IDENTITY"))
	b.options = append(b.options, WithNoOptDefVal("identity", "__SELECT__"))

	// Profiles - configuration profiles.
	b.options = append(b.options, func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        "profile",
			Shorthand:   "",
			Default:     defaults.Profile,
			Description: "Activate configuration profiles (comma-separated or repeated flag)",
			EnvVars:     []string{"ATMOS_PROFILE"},
		})
	})
}

// registerProfilingFlags registers profiling configuration flags.
func (b *GlobalOptionsBuilder) registerProfilingFlags(defaults *global.Flags) {
	defer perf.Track(nil, "flags.GlobalOptionsBuilder.registerProfilingFlags")()

	b.options = append(b.options, WithBoolFlag("profiler-enabled", "", defaults.ProfilerEnabled, "Enable pprof profiling server"))
	b.options = append(b.options, WithEnvVars("profiler-enabled", "ATMOS_PROFILER_ENABLED"))

	b.options = append(b.options, WithIntFlag("profiler-port", "", defaults.ProfilerPort, "Port for pprof profiling server"))
	b.options = append(b.options, WithEnvVars("profiler-port", "ATMOS_PROFILER_PORT"))

	b.options = append(b.options, WithStringFlag("profiler-host", "", defaults.ProfilerHost, "Host for pprof profiling server"))
	b.options = append(b.options, WithEnvVars("profiler-host", "ATMOS_PROFILER_HOST"))

	b.options = append(b.options, WithStringFlag("profile-file", "", defaults.ProfileFile, "Write profiling data to file instead of starting server"))
	b.options = append(b.options, WithEnvVars("profile-file", "ATMOS_PROFILE_FILE"))

	b.options = append(b.options, WithStringFlag("profile-type", "", defaults.ProfileType,
		"Type of profile to collect when using --profile-file. "+
			"Options: cpu, heap, allocs, goroutine, block, mutex, threadcreate, trace"))
	b.options = append(b.options, WithEnvVars("profile-type", "ATMOS_PROFILE_TYPE"))
}

// registerPerformanceFlags registers performance visualization flags.
func (b *GlobalOptionsBuilder) registerPerformanceFlags(defaults *global.Flags) {
	defer perf.Track(nil, "flags.GlobalOptionsBuilder.registerPerformanceFlags")()

	b.options = append(b.options, WithBoolFlag("heatmap", "", defaults.Heatmap, "Show performance heatmap visualization after command execution (includes P95 latency)"))
	b.options = append(b.options, WithEnvVars("heatmap", "ATMOS_HEATMAP"))

	b.options = append(b.options, WithStringFlag("heatmap-mode", "", defaults.HeatmapMode, "Heatmap visualization mode: bar, sparkline, table (press 1-3 to switch in TUI)"))
	b.options = append(b.options, WithEnvVars("heatmap-mode", "ATMOS_HEATMAP_MODE"))
}

// registerSystemFlags registers system configuration flags.
func (b *GlobalOptionsBuilder) registerSystemFlags(defaults *global.Flags) {
	defer perf.Track(nil, "flags.GlobalOptionsBuilder.registerSystemFlags")()

	b.options = append(b.options, WithStringFlag("redirect-stderr", "", defaults.RedirectStderr, "File descriptor to redirect stderr to. Errors can be redirected to any file or any standard file descriptor (including '/dev/null')"))
	b.options = append(b.options, WithEnvVars("redirect-stderr", "ATMOS_REDIRECT_STDERR"))

	// Verbose flag for error formatting.
	b.WithVerbose()

	b.options = append(b.options, WithBoolFlag("version", "", defaults.Version, "Display the Atmos CLI version"))
	b.options = append(b.options, WithEnvVars("version", "ATMOS_VERSION"))
}

// Build creates a StandardParser with all global flags configured.
// This parser can be used to register flags with RootCmd and parse them with proper precedence.
func (b *GlobalOptionsBuilder) Build() *StandardParser {
	defer perf.Track(nil, "flags.GlobalOptionsBuilder.Build")()

	return b.StandardOptionsBuilder.Build()
}
