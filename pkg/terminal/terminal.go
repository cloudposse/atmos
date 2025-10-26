package terminal

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
	"golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Terminal provides terminal capability detection and operations.
type Terminal interface {
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
	NoColor bool
	Color   bool

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
	config        *Config
	colorProfile  ColorProfile
	originalTitle string
}

// New creates a new Terminal with configuration.
func New(opts ...Option) Terminal {
	cfg := buildConfig()

	t := &terminal{
		config: cfg,
	}

	// Apply options
	for _, opt := range opts {
		opt(t)
	}

	// Detect color profile once at initialization
	isTTYOut := t.IsTTY(Stdout)
	t.colorProfile = cfg.DetectColorProfile(isTTYOut)

	return t
}

// Option configures Terminal.
type Option func(*terminal)

// WithConfig sets a custom config (for testing).
func WithConfig(cfg *Config) Option {
	return func(t *terminal) {
		t.config = cfg
	}
}

func (t *terminal) IsTTY(stream Stream) bool {
	fd := streamToFd(stream)
	if fd < 0 {
		return false
	}
	return term.IsTerminal(fd)
}

func (t *terminal) ColorProfile() ColorProfile {
	return t.colorProfile
}

func (t *terminal) Width(stream Stream) int {
	fd := streamToFd(stream)
	if fd < 0 {
		return 0
	}

	width, _, err := term.GetSize(fd)
	if err != nil {
		return 0
	}

	return width
}

func (t *terminal) Height(stream Stream) int {
	fd := streamToFd(stream)
	if fd < 0 {
		return 0
	}

	_, height, err := term.GetSize(fd)
	if err != nil {
		return 0
	}

	return height
}

func (t *terminal) SetTitle(title string) {
	// Check if title setting is enabled
	if !t.config.AtmosConfig.Settings.Terminal.Title {
		return
	}

	// Only set title if stdout is a TTY
	if !t.IsTTY(Stdout) {
		return
	}

	// Use OSC sequence to set terminal title
	// OSC 0 ; <title> BEL
	// Works in most modern terminals
	fmt.Fprintf(os.Stderr, "\033]0;%s\007", title)
}

func (t *terminal) RestoreTitle() {
	if t.originalTitle != "" {
		t.SetTitle(t.originalTitle)
	}
}

func (t *terminal) Alert() {
	// Check if alerts are enabled
	if !t.config.AtmosConfig.Settings.Terminal.Alerts {
		return
	}

	// Only alert if stderr is a TTY
	if !t.IsTTY(Stderr) {
		return
	}

	// Emit BEL character
	fmt.Fprint(os.Stderr, "\007")
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
		NoColor: viper.GetBool("no-color"),
		Color:   viper.GetBool("color"),

		// From environment variables
		EnvNoColor:       os.Getenv("NO_COLOR") != "",
		EnvCLIColor:      os.Getenv("CLICOLOR"),
		EnvCLIColorForce: os.Getenv("CLICOLOR_FORCE") != "",
		EnvTerm:          os.Getenv("TERM"),
		EnvColorTerm:     os.Getenv("COLORTERM"),
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
// 2. CLICOLOR=0 - disables color (unless CLICOLOR_FORCE is set)
// 3. CLICOLOR_FORCE - forces color even for non-TTY
// 4. --no-color flag
// 5. --color flag
// 6. atmos.yaml terminal.no_color (deprecated)
// 7. atmos.yaml terminal.color
// 8. Default (true for TTY, false for non-TTY)
func (c *Config) ShouldUseColor(isTTY bool) bool {
	// 1. NO_COLOR always wins
	if c.EnvNoColor {
		return false
	}

	// 2. CLICOLOR=0 (unless forced)
	if c.EnvCLIColor == "0" && !c.EnvCLIColorForce {
		return false
	}

	// 3. CLICOLOR_FORCE overrides TTY detection
	if c.EnvCLIColorForce {
		return true
	}

	// 4. --no-color flag
	if c.NoColor {
		return false
	}

	// 5. --color flag
	if c.Color {
		return true
	}

	// 6-7. atmos.yaml config
	if c.AtmosConfig.Settings.Terminal.NoColor {
		return false
	}
	if !c.AtmosConfig.Settings.Terminal.Color {
		return false
	}

	// 8. Default based on TTY
	return isTTY
}

// DetectColorProfile determines the terminal's color capabilities.
func (c *Config) DetectColorProfile(isTTY bool) ColorProfile {
	// If color is disabled, return ColorNone
	if !c.ShouldUseColor(isTTY) {
		return ColorNone
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
