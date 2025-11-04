package flags

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// GlobalFlags contains all persistent flags available to every command.
// These flags are inherited from RootCmd.PersistentFlags() and should be embedded
// in all command interpreters using Go struct embedding.
//
// This provides a DRY way to handle global flags - define once, use everywhere.
//
// Example usage:
//
//	type TerraformOptions struct {
//	    GlobalFlags  // Embedded - provides all global flags
//	    Stack   string
//	    DryRun  bool
//	}
//
//	// Access global flags naturally:
//	interpreter.LogsLevel    // From GlobalFlags
//	interpreter.NoColor      // From GlobalFlags
//	interpreter.Stack        // From TerraformOptions
type GlobalFlags struct {
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
	ForceColor bool // Force color output even when not a TTY (--force-color).
	ForceTTY   bool // Force TTY mode with sane defaults (--force-tty).
	Mask       bool // Enable automatic masking of sensitive data (--mask).

	// Output configuration.
	Pager PagerSelector

	// Authentication.
	Identity IdentitySelector

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

// NewGlobalFlags creates a GlobalFlags with default values.
// This is primarily used for testing.
func NewGlobalFlags() GlobalFlags {
	defer perf.Track(nil, "flagparser.NewGlobalFlags")()

	return GlobalFlags{
		LogsLevel:    "Info",
		LogsFile:     "/dev/stderr",
		NoColor:      false,
		ForceColor:   false,
		ForceTTY:     false,
		Mask:         true, // Enabled by default for security.
		ProfilerPort: 6060,
		ProfilerHost: "localhost",
		ProfileType:  "cpu",
		Heatmap:      false,
		HeatmapMode:  "bar",
	}
}

// GetGlobalFlags returns a pointer to the GlobalFlags.
// This implements part of the CommandOptions interface.
func (g *GlobalFlags) GetGlobalFlags() *GlobalFlags {
	defer perf.Track(nil, "flagparser.GlobalFlags.GetGlobalFlags")()

	return g
}
