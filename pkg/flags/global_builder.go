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

	// Working directory and path configuration.
	builder.options = append(builder.options, WithStringFlag("chdir", "C", defaults.Chdir, "Change working directory before executing the command"))
	builder.options = append(builder.options, WithEnvVars("chdir", "ATMOS_CHDIR"))

	builder.options = append(builder.options, WithStringFlag("base-path", "", defaults.BasePath, "Base path for Atmos project"))
	builder.options = append(builder.options, WithEnvVars("base-path", "ATMOS_BASE_PATH"))

	// String slice flags use registry pattern.
	builder.options = append(builder.options, func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        "config",
			Shorthand:   "",
			Default:     defaults.Config,
			Description: "Paths to configuration files (comma-separated or repeated flag)",
			EnvVars:     []string{"ATMOS_CONFIG"},
		})
	})

	builder.options = append(builder.options, func(cfg *parserConfig) {
		cfg.registry.Register(&StringSliceFlag{
			Name:        "config-path",
			Shorthand:   "",
			Default:     defaults.ConfigPath,
			Description: "Paths to search for Atmos configuration (comma-separated or repeated flag)",
			EnvVars:     []string{"ATMOS_CONFIG_PATH"},
		})
	})

	// Logging configuration.
	builder.options = append(builder.options, WithStringFlag("logs-level", "", defaults.LogsLevel, "Logs level. Supported log levels are Trace, Debug, Info, Warning, Off. If the log level is set to Off, Atmos will not log any messages"))
	builder.options = append(builder.options, WithEnvVars("logs-level", "ATMOS_LOGS_LEVEL"))

	builder.options = append(builder.options, WithStringFlag("logs-file", "", defaults.LogsFile, "The file to write Atmos logs to. Logs can be written to any file or any standard file descriptor, including '/dev/stdout', '/dev/stderr' and '/dev/null'"))
	builder.options = append(builder.options, WithEnvVars("logs-file", "ATMOS_LOGS_FILE"))

	builder.options = append(builder.options, WithBoolFlag("no-color", "", defaults.NoColor, "Disable color output"))
	builder.options = append(builder.options, WithEnvVars("no-color", "ATMOS_NO_COLOR", "NO_COLOR", "CLICOLOR"))

	// Terminal and I/O configuration - use existing builder methods!
	builder.WithForceColor()
	builder.WithForceTTY()
	builder.WithMask()

	// Output configuration - pager with NoOptDefVal.
	builder.options = append(builder.options, WithStringFlag("pager", "", defaults.Pager.Value(), "Enable pager for output (--pager or --pager=true to enable, --pager=false to disable, --pager=less to use specific pager)"))
	builder.options = append(builder.options, WithEnvVars("pager", "ATMOS_PAGER", "PAGER"))
	builder.options = append(builder.options, WithNoOptDefVal("pager", "true"))

	// Authentication - identity with NoOptDefVal.
	builder.options = append(builder.options, WithStringFlag("identity", "", defaults.Identity.Value(), "Identity to use for authentication. Use --identity to select interactively, --identity=NAME to specify"))
	builder.options = append(builder.options, WithEnvVars("identity", "ATMOS_IDENTITY"))
	builder.options = append(builder.options, WithNoOptDefVal("identity", "__SELECT__"))

	// Profiling configuration.
	builder.options = append(builder.options, WithBoolFlag("profiler-enabled", "", defaults.ProfilerEnabled, "Enable pprof profiling server"))
	builder.options = append(builder.options, WithEnvVars("profiler-enabled", "ATMOS_PROFILER_ENABLED"))

	builder.options = append(builder.options, WithIntFlag("profiler-port", "", defaults.ProfilerPort, "Port for pprof profiling server"))
	builder.options = append(builder.options, WithEnvVars("profiler-port", "ATMOS_PROFILER_PORT"))

	builder.options = append(builder.options, WithStringFlag("profiler-host", "", defaults.ProfilerHost, "Host for pprof profiling server"))
	builder.options = append(builder.options, WithEnvVars("profiler-host", "ATMOS_PROFILER_HOST"))

	builder.options = append(builder.options, WithStringFlag("profile-file", "", defaults.ProfileFile, "Write profiling data to file instead of starting server"))
	builder.options = append(builder.options, WithEnvVars("profile-file", "ATMOS_PROFILE_FILE"))

	builder.options = append(builder.options, WithStringFlag("profile-type", "", defaults.ProfileType,
		"Type of profile to collect when using --profile-file. "+
			"Options: cpu, heap, allocs, goroutine, block, mutex, threadcreate, trace"))
	builder.options = append(builder.options, WithEnvVars("profile-type", "ATMOS_PROFILE_TYPE"))

	// Performance visualization.
	builder.options = append(builder.options, WithBoolFlag("heatmap", "", defaults.Heatmap, "Show performance heatmap visualization after command execution (includes P95 latency)"))
	builder.options = append(builder.options, WithEnvVars("heatmap", "ATMOS_HEATMAP"))

	builder.options = append(builder.options, WithStringFlag("heatmap-mode", "", defaults.HeatmapMode, "Heatmap visualization mode: bar, sparkline, table (press 1-3 to switch in TUI)"))
	builder.options = append(builder.options, WithEnvVars("heatmap-mode", "ATMOS_HEATMAP_MODE"))

	// System configuration.
	builder.options = append(builder.options, WithStringFlag("redirect-stderr", "", defaults.RedirectStderr, "File descriptor to redirect stderr to. Errors can be redirected to any file or any standard file descriptor (including '/dev/null')"))
	builder.options = append(builder.options, WithEnvVars("redirect-stderr", "ATMOS_REDIRECT_STDERR"))

	builder.options = append(builder.options, WithBoolFlag("version", "", defaults.Version, "Display the Atmos CLI version"))
	builder.options = append(builder.options, WithEnvVars("version", "ATMOS_VERSION"))

	return builder
}

// Build creates a StandardParser with all global flags configured.
// This parser can be used to register flags with RootCmd and parse them with proper precedence.
func (b *GlobalOptionsBuilder) Build() *StandardParser {
	defer perf.Track(nil, "flags.GlobalOptionsBuilder.Build")()

	return b.StandardOptionsBuilder.Build()
}
