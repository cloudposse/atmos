package global

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// DefaultProfilerPort is the default port for the Go pprof profiler.
	DefaultProfilerPort = 6060
)

// Flags contains all persistent flags available to every command.
// These flags are inherited from RootCmd.PersistentFlags() and should be embedded
// in all command interpreters using Go struct embedding.
//
// This provides a DRY way to handle global flags - define once, use everywhere.
//
// Example usage:
//
//	type TerraformOptions struct {
//	    global.Flags  // Embedded - provides all global flags
//	    Stack   string
//	    DryRun  bool
//	}
//
//	// Access global flags naturally:
//	interpreter.LogsLevel    // From global.Flags
//	interpreter.NoColor      // From global.Flags
//	interpreter.Stack        // From TerraformOptions
type Flags struct {
	// Working directory and path configuration.
	Chdir      string
	BasePath   string
	Config     []string // Config file paths.
	ConfigPath []string // Config directory paths.

	// Logging configuration.
	LogsLevel string
	LogsFile  string
	NoColor   bool

	// Terminal and I/O configuration.
	ForceColor  bool // Force color output even when not a TTY (--force-color).
	ForceTTY    bool // Force TTY mode with sane defaults (--force-tty).
	Mask        bool // Enable automatic masking of sensitive data (--mask).
	Interactive bool // Enable interactive prompts for missing required flags, optional value flags using the sentinel pattern, and missing positional arguments (--interactive).

	// Output configuration.
	Pager PagerSelector

	// Authentication.
	Identity IdentitySelector
	// Note: GitHubToken is NOT a global flag. It is only used by toolchain commands
	// and is registered locally on the toolchain command in cmd/toolchain/toolchain.go.

	// Profiles.
	Profile []string // Profile selects which configuration profiles to activate.

	// Profiling configuration.
	ProfilerEnabled bool
	ProfilerPort    int
	ProfilerHost    string
	ProfileFile     string
	ProfileType     string

	// Performance visualization.
	Heatmap     bool
	HeatmapMode string

	// System configuration.
	RedirectStderr string
	Version        bool
}

// NewFlags creates a Flags with default values.
// This is primarily used for testing.
func NewFlags() Flags {
	defer perf.Track(nil, "global.NewFlags")()

	return Flags{
		LogsLevel:    "Warning",
		LogsFile:     "/dev/stderr",
		NoColor:      false,
		ForceColor:   false,
		ForceTTY:     false,
		Mask:         true,       // Enabled by default for security.
		Interactive:  true,       // Enabled by default for better UX.
		Profile:      []string{}, // No profiles active by default.
		ProfilerPort: DefaultProfilerPort,
		ProfilerHost: "localhost",
		ProfileType:  "cpu",
		Heatmap:      false,
		HeatmapMode:  "bar",
	}
}

// GetGlobalFlags returns a pointer to the Flags.
// This implements part of the CommandOptions interface.
func (g *Flags) GetGlobalFlags() *Flags {
	defer perf.Track(nil, "global.Flags.GetGlobalFlags")()

	return g
}
