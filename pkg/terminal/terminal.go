package terminal

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
	"golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// ANSI escape sequences for terminal control.
	escBEL = "\007"  // Bell/Alert character
	escOSC = "\033]" // Operating System Command
	escST  = "\007"  // String Terminator (can also be "\033\\")

	// ANSI cursor control sequences.
	EscCarriageReturn = "\r"      // Return cursor to start of line
	EscClearLine      = "\x1b[K"  // Clear from cursor to end of line
	EscResetLine      = "\r\x1b[K" // Return to start and clear entire line
)

// IOWriter is the interface for writing to I/O streams.
// This avoids circular dependency with pkg/io.
// The stream parameter uses int to allow different packages to define their own stream types.
type IOWriter interface {
	// Write outputs content to the specified stream with automatic masking.
	// stream values: 0=Data (stdout), 1=UI (stderr)
	Write(stream int, content string) error
}

// IOStream represents an I/O stream type for terminal operations.
// This mirrors io.Stream but avoids circular dependency.
type IOStream int

const (
	// IOStreamData represents stdout (data channel) - value 0.
	IOStreamData IOStream = 0
	// IOStreamUI represents stderr (UI channel) - value 1.
	IOStreamUI IOStream = 1
)

// Terminal provides terminal capability detection and operations.
// Terminal writes UI output through the I/O layer for automatic masking.
type Terminal interface {
	// Write outputs UI content to the terminal.
	// Content flows: terminal.Write() → io.Write(UIStream) → masking → stderr
	Write(content string) error

	// IsTTY returns whether the given stream is a TTY.
	IsTTY(stream Stream) bool

	// ColorProfile returns the terminal's color capabilities.
	ColorProfile() ColorProfile

	// Width returns the terminal width for the given stream.
	// Returns 0 if width cannot be determined.
	Width(stream Stream) int

	// Height returns the terminal height for the given stream.
	// Returns 0 if height cannot be determined.
	Height(stream Stream) int

	// SetTitle sets the terminal window title (if supported).
	// Does nothing if terminal doesn't support titles or if disabled in config.
	SetTitle(title string)

	// RestoreTitle restores the original terminal title.
	RestoreTitle()

	// Alert emits a terminal bell/alert (if enabled).
	Alert()
}

// Stream represents a terminal stream.
type Stream int

const (
	Stdin Stream = iota
	Stdout
	Stderr
)

// ColorProfile represents terminal color capabilities.
type ColorProfile int

const (
	ColorNone ColorProfile = iota // No color support
	Color16                       // 16 colors (basic ANSI)
	Color256                      // 256 colors
	ColorTrue                     // Truecolor (16 million colors)
)

// String returns the string representation of ColorProfile.
func (c ColorProfile) String() string {
	switch c {
	case ColorNone:
		return "None"
	case Color16:
		return "16"
	case Color256:
		return "256"
	case ColorTrue:
		return "TrueColor"
	default:
		return "Unknown"
	}
}

// Config holds terminal configuration from various sources.
type Config struct {
	// From CLI flags
	NoColor    bool
	Color      bool
	ForceColor bool
	ForceTTY   bool

	// From environment variables
	EnvNoColor       bool   // NO_COLOR
	EnvCLIColor      string // CLICOLOR
	EnvCLIColorForce bool   // CLICOLOR_FORCE
	EnvTerm          string // TERM
	EnvColorTerm     string // COLORTERM

	// From atmos.yaml
	AtmosConfig schema.AtmosConfiguration
}

// terminal implements the Terminal interface.
type terminal struct {
	io            IOWriter
	config        *Config
	colorProfile  ColorProfile
	originalTitle string
	forceTTY      bool
	forceColor    bool
}

const (
	// Sane defaults for forced TTY mode (used for screenshots).
	defaultForcedWidth  = 120
	defaultForcedHeight = 40
)

// New creates a new Terminal with configuration.
func New(opts ...Option) Terminal {
	defer perf.Track(nil, "terminal.New")()

	cfg := buildConfig()

	t := &terminal{
		config:     cfg,
		forceTTY:   cfg.ForceTTY,
		forceColor: cfg.ForceColor,
	}

	// Apply options
	for _, opt := range opts {
		opt(t)
	}

	// Detect color profile once at initialization.
	// Priority order (highest to lowest):
	// 1. NO_COLOR env var - always disables color (overrides --force-color)
	// 2. --force-color flag - forces TrueColor
	// 3. Standard detection via DetectColorProfile
	// Check Stderr first (where UI is written), fall back to Stdout.
	isTTYOut := t.IsTTY(Stderr)
	if !isTTYOut {
		isTTYOut = t.IsTTY(Stdout)
	}

	// Determine color profile based on precedence
	switch {
	case cfg.EnvNoColor:
		// NO_COLOR always wins, even over --force-color
		t.colorProfile = ColorNone
	case t.forceColor:
		// Force color profile if --force-color is set (but NO_COLOR takes precedence)
		t.colorProfile = ColorTrue
	default:
		t.colorProfile = cfg.DetectColorProfile(isTTYOut)
	}

	return t
}

// Option configures Terminal.
type Option func(*terminal)

// WithIO sets the I/O writer for output.
// If not set, terminal will write directly to os.Stderr (no masking).
func WithIO(io IOWriter) Option {
	return func(t *terminal) {
		t.io = io
	}
}

// WithConfig sets a custom config (for testing).
func WithConfig(cfg *Config) Option {
	return func(t *terminal) {
		t.config = cfg
		t.forceTTY = cfg.ForceTTY
		t.forceColor = cfg.ForceColor
	}
}

// Write outputs UI content to the terminal through the I/O layer.
// This ensures all terminal output flows through io.Write() for automatic masking.
func (t *terminal) Write(content string) error {
	defer perf.Track(nil, "terminal.Write")()

	if t.io != nil {
		// Write through I/O layer for masking
		// IOStreamUI has value 1, matching io.UIStream
		return t.io.Write(int(IOStreamUI), content)
	}

	// Fallback: write directly to stderr (no masking)
	// This should only happen in tests or when terminal is created without I/O
	_, err := fmt.Fprint(os.Stderr, content)
	return err
}

func (t *terminal) IsTTY(stream Stream) bool {
	defer perf.Track(nil, "terminal.IsTTY")()

	// If --force-tty is set, always return true (for screenshot generation).
	if t.forceTTY {
		return true
	}

	fd := streamToFd(stream)
	if fd < 0 {
		return false
	}
	return term.IsTerminal(fd)
}

func (t *terminal) ColorProfile() ColorProfile {
	defer perf.Track(nil, "terminal.ColorProfile")()

	return t.colorProfile
}

func (t *terminal) Width(stream Stream) int {
	defer perf.Track(nil, "terminal.Width")()

	fd := streamToFd(stream)
	if fd < 0 {
		// If --force-tty is set, return sane default width.
		if t.forceTTY {
			return defaultForcedWidth
		}
		return 0
	}

	width, _, err := term.GetSize(fd)
	if err != nil {
		// If --force-tty is set and detection fails, return sane default width.
		if t.forceTTY {
			return defaultForcedWidth
		}
		return 0
	}

	return width
}

func (t *terminal) Height(stream Stream) int {
	defer perf.Track(nil, "terminal.Height")()

	fd := streamToFd(stream)
	if fd < 0 {
		// If --force-tty is set, return sane default height.
		if t.forceTTY {
			return defaultForcedHeight
		}
		return 0
	}

	_, height, err := term.GetSize(fd)
	if err != nil {
		// If --force-tty is set and detection fails, return sane default height.
		if t.forceTTY {
			return defaultForcedHeight
		}
		return 0
	}

	return height
}

func (t *terminal) SetTitle(title string) {
	defer perf.Track(nil, "terminal.SetTitle")()

	// Check if title setting is enabled
	if !t.config.AtmosConfig.Settings.Terminal.Title {
		return
	}

	// Only set title if stderr is a TTY (we write control sequences to stderr)
	if !t.IsTTY(Stderr) {
		return
	}

	// Capture original title on first call (best-effort - can't query current title)
	if t.originalTitle == "" {
		t.originalTitle = title
	}

	// Use OSC sequence to set terminal title
	// OSC 0 ; <title> ST
	// Works in most modern terminals
	titleSeq := fmt.Sprintf("%s0;%s%s", escOSC, title, escST)

	if t.io != nil {
		// Write through I/O layer (no masking needed for terminal control sequences)
		_ = t.io.Write(int(IOStreamUI), titleSeq)
	} else {
		// Fallback for tests
		fmt.Fprint(os.Stderr, titleSeq)
	}
}

func (t *terminal) RestoreTitle() {
	defer perf.Track(nil, "terminal.RestoreTitle")()

	// Best-effort restore: Set to captured original title
	// Note: We can't query the actual terminal title, so we use the first title we set
	if t.originalTitle != "" {
		titleSeq := fmt.Sprintf("%s0;%s%s", escOSC, t.originalTitle, escST)
		if t.io != nil {
			_ = t.io.Write(int(IOStreamUI), titleSeq)
		} else {
			fmt.Fprint(os.Stderr, titleSeq)
		}
	}
}

func (t *terminal) Alert() {
	defer perf.Track(nil, "terminal.Alert")()

	// Check if alerts are enabled
	if !t.config.AtmosConfig.Settings.Terminal.Alerts {
		return
	}

	// Only alert if stderr is a TTY
	if !t.IsTTY(Stderr) {
		return
	}

	if t.io != nil {
		// Write through I/O layer
		_ = t.io.Write(int(IOStreamUI), escBEL)
	} else {
		// Fallback for tests
		fmt.Fprint(os.Stderr, escBEL)
	}
}

// streamToFd converts Stream to file descriptor.
// Returns -1 if the stream type is invalid.
func streamToFd(stream Stream) int {
	switch stream {
	case Stdin:
		return int(os.Stdin.Fd())
	case Stdout:
		return int(os.Stdout.Fd())
	case Stderr:
		return int(os.Stderr.Fd())
	default:
		return -1
	}
}

// buildConfig constructs Config from all sources.
func buildConfig() *Config {
	cfg := &Config{
		// From flags (bound via viper in cmd/root.go)
		NoColor:    viper.GetBool("no-color"),
		Color:      viper.GetBool("color"),
		ForceColor: viper.GetBool("force-color"),
		ForceTTY:   viper.GetBool("force-tty"),

		// From environment variables (standard terminal env vars, not Atmos-specific)
		EnvNoColor:       os.Getenv("NO_COLOR") != "",       //nolint:forbidigo // Standard terminal env var
		EnvCLIColor:      os.Getenv("CLICOLOR"),             //nolint:forbidigo // Standard terminal env var
		EnvCLIColorForce: os.Getenv("CLICOLOR_FORCE") != "", //nolint:forbidigo // Standard terminal env var
		EnvTerm:          os.Getenv("TERM"),                 //nolint:forbidigo // Standard terminal env var
		EnvColorTerm:     os.Getenv("COLORTERM"),            //nolint:forbidigo // Standard terminal env var
	}

	// Load atmos.yaml config (if available)
	if viper.IsSet("settings") {
		var atmosConfig schema.AtmosConfiguration
		if err := viper.Unmarshal(&atmosConfig); err == nil {
			cfg.AtmosConfig = atmosConfig
		}
	}

	return cfg
}

// ShouldUseColor determines if color should be used based on config priority.
// Priority (highest to lowest):
// 1. NO_COLOR env var - disables all color
// 2. CLICOLOR=0 - disables color (unless CLICOLOR_FORCE or --force-color is set)
// 3. CLICOLOR_FORCE - forces color even for non-TTY
// 4. --force-color flag - forces color even for non-TTY
// 5. --no-color flag - disables color
// 6. --color flag - enables color (only if TTY)
// 7. Atmos.yaml terminal.no_color (deprecated) - disables color
// 8. Atmos.yaml terminal.color - enables color (only if TTY)
// 9. Default (true for TTY, false for non-TTY).
//
//nolint:revive // Cyclomatic complexity acceptable for priority-based configuration logic.
func (c *Config) ShouldUseColor(isTTY bool) bool {
	// 1. NO_COLOR always wins
	if c.EnvNoColor {
		return false
	}

	// 2. CLICOLOR=0 (unless forced)
	if c.EnvCLIColor == "0" && !c.EnvCLIColorForce && !c.ForceColor {
		return false
	}

	// 3. CLICOLOR_FORCE overrides TTY detection
	if c.EnvCLIColorForce {
		return true
	}

	// 4. --force-color flag overrides TTY detection
	if c.ForceColor {
		return true
	}

	// 5. --no-color flag
	if c.NoColor {
		return false
	}

	// 6. --color flag (respects TTY - only enables color if TTY)
	if c.Color && isTTY {
		return true
	}

	// 7-8. atmos.yaml config (respects TTY)
	if c.AtmosConfig.Settings.Terminal.NoColor {
		return false
	}
	if c.AtmosConfig.Settings.Terminal.Color && isTTY {
		return true
	}

	// 9. Default based on TTY
	return isTTY
}

// DetectColorProfile determines the terminal's color capabilities.
//
//nolint:revive // Cyclomatic complexity acceptable for priority-based capability detection.
func (c *Config) DetectColorProfile(isTTY bool) ColorProfile {
	// If color is disabled, return ColorNone
	if !c.ShouldUseColor(isTTY) {
		return ColorNone
	}

	// If --force-color is set, return TrueColor
	if c.ForceColor {
		return ColorTrue
	}

	// If CLICOLOR_FORCE is set, return TrueColor
	if c.EnvCLIColorForce {
		return ColorTrue
	}

	// Check for truecolor support
	colorTerm := strings.ToLower(c.EnvColorTerm)
	if colorTerm == "truecolor" || colorTerm == "24bit" {
		return ColorTrue
	}

	// Check TERM for 256 color support
	termVar := strings.ToLower(c.EnvTerm)
	if strings.Contains(termVar, "256color") || strings.Contains(termVar, "256") {
		return Color256
	}

	// Check for any color support
	if strings.Contains(termVar, "color") || termVar == "xterm" || termVar == "screen" {
		return Color16
	}

	// Default to 16 colors if TTY and color enabled
	if isTTY {
		return Color16
	}

	return ColorNone
}
